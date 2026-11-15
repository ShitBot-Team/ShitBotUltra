package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/adapter/onebotv11"
	"github.com/ShitBot-Team/ShitBotUltra/internal/adapter/registry"
	"github.com/ShitBot-Team/ShitBotUltra/internal/binding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/command"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/events"
	"github.com/ShitBot-Team/ShitBotUltra/internal/extension"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/forwarding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/gateway/minecraft"
	"github.com/ShitBot-Team/ShitBotUltra/internal/importer/shitbot"
	"github.com/ShitBot-Team/ShitBotUltra/internal/inventory"
	"github.com/ShitBot-Team/ShitBotUltra/internal/player"
	"github.com/ShitBot-Team/ShitBotUltra/internal/rendering"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage/sqlite"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
	sdkextension "github.com/ShitBot-Team/ShitBotUltra/sdk/extension"
)

type Engine struct {
	Config      config.Config
	ConfigStore *config.Store
	DB          *sqlite.Store
	Bindings    *binding.Service
	Players     *player.Service
	Inventory   *inventory.Service
	Renderer    *rendering.Service
	Plugins     *extension.Manager
	Imports     *shitbot.Service
	Events      *events.Bus
	Hub         *minecraft.Hub
	Adapters    map[string]adapter.Adapter
	AdminToken  string
	Dir         string
	logger      *slog.Logger
	commands    *command.Service
	router      *forwarding.Router
	ctx         context.Context
	cancel      context.CancelFunc
	queues      [4]chan adapter.Event
	wg          sync.WaitGroup
	closeOnce   sync.Once
}

func New(ctx context.Context, dir string, cfg config.Config, cs *config.Store, db *sqlite.Store, logger *slog.Logger, factories map[string]adapter.Factory) (*Engine, error) {
	ctx, cancel := context.WithCancel(ctx)
	e := &Engine{Config: cfg, ConfigStore: cs, DB: db, Dir: dir, logger: logger, ctx: ctx, cancel: cancel, Events: events.New(), Adapters: map[string]adapter.Adapter{}, commands: command.New(), router: forwarding.New(cfg.Routes)}
	success := false
	defer func() {
		if !success {
			e.Close()
		}
	}()
	var err error
	e.AdminToken, err = config.Secret(dir, cfg.HTTP.AdminToken)
	if err != nil {
		return nil, err
	}
	tokens := map[string]string{}
	used := map[string]bool{e.AdminToken: true}
	networks := map[string]string{}
	for _, node := range cfg.Nodes {
		token, err := config.Secret(dir, node.Token)
		if err != nil {
			return nil, err
		}
		if used[token] {
			return nil, errors.New("admin and node credentials must be distinct")
		}
		used[token] = true
		tokens[node.ID] = token
		networks[node.NetworkID] = node.IdentityMode
	}
	e.Bindings = binding.New(db, cfg.Binding)
	e.Players = player.New(db)
	e.Inventory = inventory.New(db, cfg.InventoryRetentionDays)
	e.Imports = shitbot.New(db, dir, cfg.Binding.MaxPlayersPerIdentity, networks)
	e.Hub = minecraft.New(ctx, db, cfg.Nodes, tokens, e)
	browserPath := cfg.Rendering.BrowserPath
	if browserPath != "" {
		browserPath = config.ResolvePath(dir, browserPath)
	}
	e.Renderer, err = rendering.New(ctx, dir, rendering.Options{BrowserPath: browserPath, BrowserVersion: cfg.Rendering.BrowserVersion, Concurrency: cfg.Rendering.Concurrency, QueueSize: cfg.Rendering.QueueSize, Timeout: time.Duration(cfg.Rendering.TimeoutSeconds) * time.Second})
	if err != nil {
		return nil, err
	}
	e.Plugins, err = extension.New(ctx, dir, e)
	if err != nil {
		return nil, err
	}
	if err := e.registerDataProviders(); err != nil {
		return nil, err
	}
	r := registry.New()
	if err := r.Register("onebot.v11", onebotv11.New); err != nil {
		return nil, err
	}
	for kind, factory := range factories {
		if err := r.Register(kind, factory); err != nil {
			return nil, err
		}
	}
	for _, conn := range cfg.Connections {
		if !conn.Enabled {
			continue
		}
		token, err := config.Secret(dir, conn.Token)
		if err != nil {
			return nil, err
		}
		if used[token] {
			return nil, errors.New("bot credentials must not reuse admin or node credentials")
		}
		instance, err := r.Create(conn.Type, adapter.Config{ID: conn.ID, Endpoint: conn.Endpoint, Token: token, AccountID: conn.AccountID, QueueSize: 64, RequestTimeout: 15 * time.Second})
		if err != nil {
			return nil, err
		}
		e.Adapters[conn.ID] = instance
	}
	for i := range e.queues {
		e.queues[i] = make(chan adapter.Event, 64)
	}
	success = true
	return e, nil
}

func (e *Engine) Start() error {
	if err := e.Players.Recover(e.ctx); err != nil {
		return err
	}
	e.Plugins.Start()
	for _, queue := range e.queues {
		e.wg.Add(1)
		go func(queue <-chan adapter.Event) {
			defer e.wg.Done()
			for {
				select {
				case <-e.ctx.Done():
					return
				case event := <-queue:
					ctx, cancel := context.WithTimeout(e.ctx, 30*time.Second)
					e.handleBot(ctx, event)
					cancel()
				}
			}
		}(queue)
	}
	for id, instance := range e.Adapters {
		e.wg.Add(1)
		go func(id string, instance adapter.Adapter) {
			defer e.wg.Done()
			err := instance.Run(e.ctx, func(ctx context.Context, event adapter.Event) error {
				if event.ConnectionID != id || !adapter.ValidID(event.ID) || event.Sender.Validate() != nil || event.Conversation.Validate() != nil {
					return errors.New("invalid adapter event")
				}
				index := crc32.ChecksumIEEE([]byte(id+"\x1f"+event.Conversation.Key())) % uint32(len(e.queues))
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-e.ctx.Done():
					return e.ctx.Err()
				case e.queues[index] <- event:
					return nil
				default:
					return adapter.ErrBusy
				}
			})
			if err != nil && e.ctx.Err() == nil {
				e.logger.Warn("机器人适配器停止", "connection", id)
				e.Events.Publish("adapter.error", map[string]string{"connection_id": id})
			}
		}(id, instance)
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-e.ctx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(e.ctx, 15*time.Second)
				err := storage.Cleanup(ctx, e.DB, e.Config.InventoryRetentionDays)
				cancel()
				if err != nil && e.ctx.Err() == nil {
					e.logger.Warn("数据保留清理未完成")
				}
				e.Events.Publish("connections", map[string]any{"nodes": e.Hub.Status(), "adapters": e.AdapterStatus()})
			}
		}
	}()
	return nil
}

func (e *Engine) Close() {
	e.closeOnce.Do(func() {
		e.cancel()
		if e.Hub != nil {
			e.Hub.Close()
		}
		e.wg.Wait()
		if e.Plugins != nil {
			e.Plugins.Close()
		}
		if e.Renderer != nil {
			e.Renderer.Close()
		}
		e.Events.Close()
	})
}
func (e *Engine) AdapterStatus() []adapter.Status {
	result := make([]adapter.Status, 0, len(e.Adapters))
	for _, a := range e.Adapters {
		result = append(result, a.Status())
	}
	slices.SortFunc(result, func(a, b adapter.Status) int { return strings.Compare(a.ID, b.ID) })
	return result
}

func (e *Engine) GameConnected(ctx context.Context, node config.Node, boot string) {
	e.Events.Publish("node.connected", map[string]string{"node_id": node.ID})
	e.Plugins.Emit(sdkextension.Event{Type: "node.connected", NetworkID: node.NetworkID, NodeID: node.ID, Data: json.RawMessage(`{}`)})
}
func (e *Engine) GameDisconnected(ctx context.Context, node config.Node, boot string) {
	_ = e.Players.Disconnect(ctx, node.ID, boot)
	e.Events.Publish("node.disconnected", map[string]string{"node_id": node.ID})
	e.Plugins.Emit(sdkextension.Event{Type: "node.disconnected", NetworkID: node.NetworkID, NodeID: node.ID, Data: json.RawMessage(`{}`)})
}
func (e *Engine) GameHeartbeat(ctx context.Context, node config.Node, boot string) {
	_ = e.Players.Heartbeat(ctx, node.ID, boot)
}

func (e *Engine) HandleGame(ctx context.Context, node config.Node, envelope bridge.Envelope) (any, error) {
	if envelope.Kind == "request" {
		switch envelope.Type {
		case "login.authorize":
			var request bridge.LoginRequest
			if json.Unmarshal(envelope.Payload, &request) != nil {
				return nil, fault.New("INVALID_INPUT", "登录请求格式无效")
			}
			return e.Bindings.Authorize(ctx, node, request.Player)
		case "binding.get":
			var request struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(envelope.Payload, &request) != nil {
				return nil, fault.New("INVALID_INPUT", "玩家名格式无效")
			}
			record, err := e.Bindings.Find(ctx, node.NetworkID, request.Name)
			if err != nil {
				return nil, err
			}
			return map[string]any{"player": record.Player, "whitelisted": true, "bound": record.Owner != nil}, nil
		default:
			return nil, fault.New("UNSUPPORTED_CAPABILITY", "Core 不支持该请求")
		}
	}
	switch envelope.Type {
	case "player.joined", "player.left", "players.snapshot":
		if err := e.Players.Apply(ctx, node, envelope); err != nil {
			return nil, err
		}
		if envelope.Type != "players.snapshot" {
			claimed, err := storage.Claim(ctx, e.DB, "notification:"+node.ID+":"+envelope.BootID, envelope.MessageID)
			if err != nil {
				return nil, err
			}
			if claimed {
				var event bridge.PlayerEvent
				_ = json.Unmarshal(envelope.Payload, &event)
				verb := "加入了游戏"
				if envelope.Type == "player.left" {
					verb = "离开了游戏"
				}
				e.notify(ctx, node, event.Player.Name+" "+verb)
			}
		}
		e.Events.Publish("players.changed", map[string]string{"network_id": node.NetworkID})
		e.Plugins.Emit(sdkextension.Event{Type: envelope.Type, NetworkID: node.NetworkID, NodeID: node.ID, Data: append(json.RawMessage(nil), envelope.Payload...)})
		return nil, nil
	case "inventory.snapshot":
		var snapshot bridge.InventorySnapshot
		if json.Unmarshal(envelope.Payload, &snapshot) != nil {
			return nil, fault.New("INVALID_INPUT", "背包快照格式无效")
		}
		return nil, e.Inventory.Save(ctx, node, snapshot)
	case "chat.message":
		var chat bridge.Chat
		if json.Unmarshal(envelope.Payload, &chat) != nil || len(chat.Text) > 16000 {
			return nil, fault.New("INVALID_INPUT", "聊天事件无效")
		}
		if err := binding.ValidatePlayer(chat.Player); err != nil {
			return nil, err
		}
		if chat.Player.IdentityMode != node.IdentityMode {
			return nil, fault.New("IDENTITY_CONFLICT", "玩家身份模式不一致")
		}
		if chat.OriginID != "" || time.Since(envelope.OccurredAt) > 30*time.Second {
			return nil, nil
		}
		if envelope.OccurredAt.After(time.Now().Add(30 * time.Second)) {
			return nil, fault.New("INVALID_INPUT", "聊天时间超出允许偏差")
		}
		claimed, err := storage.Claim(ctx, e.DB, "chat:"+node.ID+":"+envelope.BootID, envelope.MessageID)
		if err != nil || !claimed {
			return nil, err
		}
		e.forwardGame(ctx, node, chat)
		return nil, nil
	case "server.ready":
		claimed, err := storage.Claim(ctx, e.DB, "ready:"+node.ID, envelope.BootID)
		if err != nil {
			return nil, err
		}
		if claimed {
			e.notify(ctx, node, "服务器 "+node.ServerID+" 已就绪")
		}
		return nil, nil
	default:
		return nil, fault.New("UNSUPPORTED_EVENT", "Core 不支持该事件")
	}
}

func (e *Engine) send(ctx context.Context, route config.Route, message adapter.Message) {
	instance := e.Adapters[route.ConnectionID]
	if instance == nil {
		return
	}
	if _, err := instance.Send(ctx, route.Conversation, message); err != nil {
		e.logger.Warn("机器人消息未确认送达", "connection", route.ConnectionID)
		e.Events.Publish("delivery.failed", map[string]string{"connection_id": route.ConnectionID})
	}
}

func (e *Engine) notify(ctx context.Context, node config.Node, text string) {
	seen := map[string]bool{}
	for _, r := range e.router.FromGame(node.ID) {
		key := r.ConnectionID + r.Conversation.Key()
		if r.Notifications && !seen[key] {
			seen[key] = true
			e.send(ctx, r, adapter.Text(forwarding.Clean(text, 2000)))
		}
	}
}
func (e *Engine) forwardGame(ctx context.Context, node config.Node, chat bridge.Chat) {
	seen := map[string]bool{}
	for _, r := range e.router.FromGame(node.ID) {
		key := r.ConnectionID + r.Conversation.Key()
		if r.ToBot && !seen[key] {
			seen[key] = true
			e.send(ctx, r, adapter.Text(forwarding.Clean(r.GamePrefix+"["+node.ServerID+"] "+chat.Player.Name+": "+chat.Text, 3000)))
		}
	}
}

func (e *Engine) handleBot(ctx context.Context, event adapter.Event) {
	routes := e.router.FromBot(event)
	if len(routes) == 0 {
		return
	}
	if event.OccurredAt.IsZero() || time.Since(event.OccurredAt) > 2*time.Minute || event.OccurredAt.After(time.Now().Add(30*time.Second)) {
		return
	}
	claimed, err := storage.Claim(ctx, e.DB, "bot:"+event.ConnectionID+":"+event.Conversation.Key(), event.ID)
	if err != nil || !claimed {
		return
	}
	if event.Type == "member.left" {
		e.memberLeft(ctx, event, routes)
		return
	}
	if event.Type == "member.joined" {
		for _, route := range routes {
			if route.Notifications {
				e.send(ctx, route, adapter.Text("欢迎 "+forwarding.Clean(event.Sender.DisplayName, 64)+"，发送 /help 查看绑定指令。"))
				break
			}
		}
		return
	}
	if event.Type != "message" {
		return
	}
	for _, route := range routes {
		node, _ := e.Config.Node(route.NodeIDs[0])
		data, _ := json.Marshal(event)
		e.Plugins.Emit(sdkextension.Event{Type: "message", NetworkID: node.NetworkID, RouteID: route.ID, Data: data})
	}
	text := strings.TrimSpace(event.Message.PlainText())
	if strings.HasPrefix(text, "/") {
		for _, route := range routes {
			if route.Commands {
				e.botCommand(ctx, event, route, text)
				return
			}
		}
		return
	}
	seen := map[string]bool{}
	for _, route := range routes {
		if !route.ToGame {
			continue
		}
		for _, nodeID := range route.NodeIDs {
			if seen[nodeID] {
				continue
			}
			seen[nodeID] = true
			payload := bridge.Broadcast{Text: forwarding.Clean(route.BotPrefix+event.Sender.DisplayName+": "+forwarding.Text(event.Message), 3000), OriginID: event.ConnectionID + ":" + event.ID}
			if _, err := e.Hub.Request(ctx, nodeID, "chat.send", payload); err != nil {
				e.Events.Publish("delivery.failed", map[string]string{"node_id": nodeID})
			}
		}
	}
}

func (e *Engine) botCommand(ctx context.Context, event adapter.Event, route config.Route, text string) {
	reply := func(text string) { e.send(ctx, route, adapter.Text(text)) }
	args, err := command.Split(strings.TrimPrefix(text, "/"))
	if err != nil || len(args) == 0 {
		reply("命令格式无效；包含空格的玩家名请用双引号包围。")
		return
	}
	node, _ := e.Config.Node(route.NodeIDs[0])
	network := node.NetworkID
	fail := func(err error) { reply(fault.Public(err).Message) }
	if e.imageCommand(ctx, event, route, network, args) {
		return
	}
	switch args[0] {
	case "help":
		reply("/bind \"玩家名\" 验证码\n/unbind \"玩家名\"\n/bindings\n/online\n/profile [\"玩家名\"]\n/inventory [\"玩家名\"]\n/tps（Minecraft 服务器 TPS）")
	case "bind":
		if len(args) != 3 {
			reply("用法：/bind \"玩家名\" 验证码")
			return
		}
		record, err := e.Bindings.Redeem(ctx, network, args[1], args[2], event.Sender.Identity)
		if err != nil {
			fail(err)
			return
		}
		reply("已绑定 " + record.Player.Name + "，现在可以重新进入游戏。")
		e.Events.Publish("bindings.changed", map[string]string{"network_id": network})
	case "unbind":
		if len(args) != 2 {
			reply("用法：/unbind \"玩家名\"")
			return
		}
		record, err := e.Bindings.Remove(ctx, network, args[1], &event.Sender.Identity)
		if err != nil {
			fail(err)
			return
		}
		e.AfterUnbind(ctx, record)
		reply("已解绑 " + record.Player.Name)
	case "bindings":
		records, err := e.Bindings.List(ctx, network, &event.Sender.Identity, 100, 0)
		if err != nil {
			fail(err)
			return
		}
		names := []string{}
		for _, record := range records {
			names = append(names, record.Player.Name)
		}
		if len(names) == 0 {
			reply("尚未绑定玩家")
		} else {
			reply("已绑定：" + strings.Join(names, "、"))
		}
	case "online":
		players, err := e.Players.List(ctx, network, true, 1000, 0)
		if err != nil {
			fail(err)
			return
		}
		names := []string{}
		for _, p := range players {
			names = append(names, p.Player.Name)
		}
		if e.sendImage(ctx, route, e.Config.Rendering.OnlineTemplate, map[string]any{"network_id": network, "players": players, "updated_at": time.Now().UTC()}) {
			return
		}
		reply(forwarding.Clean(fmt.Sprintf("在线玩家 %d：%s", len(names), strings.Join(names, "、")), 3000))
	default:
		for _, c := range e.Config.Commands {
			if c.Name != args[0] || !slices.Contains(route.NodeIDs, c.NodeID) {
				continue
			}
			playerName := ""
			if c.RequireBinding {
				records, err := e.Bindings.List(ctx, network, &event.Sender.Identity, 2, 0)
				if err != nil {
					fail(err)
					return
				}
				if len(records) != 1 {
					reply("此快捷命令要求当前账号恰好绑定一个玩家。")
					return
				}
				playerName = records[0].Player.Name
			}
			line, err := e.commands.Prepare(c, event.Sender, playerName, args[1:])
			if err != nil {
				fail(err)
				return
			}
			_, err = e.Hub.Request(ctx, c.NodeID, "command.execute", bridge.Command{Command: line, ActorID: event.Sender.Key()})
			if err != nil {
				fail(err)
				return
			}
			reply("游戏节点已确认执行。")
			return
		}
		if !e.pluginCommand(ctx, event, route, network, args) {
			reply("未知命令，发送 /help 查看指令。")
		}
	}
}

func (e *Engine) AfterUnbind(ctx context.Context, record binding.Record) {
	e.Events.Publish("bindings.changed", map[string]string{"network_id": record.NetworkID})
	if !e.Config.Binding.KickOnUnbind {
		return
	}
	for _, node := range e.Config.Nodes {
		if node.NetworkID == record.NetworkID && slices.Contains(node.Actions, "player.kick") {
			if _, err := e.Hub.Request(ctx, node.ID, "player.kick", bridge.Kick{Player: record.Player, Reason: "白名单或绑定已撤销"}); err != nil {
				e.Events.Publish("kick.unconfirmed", map[string]string{"node_id": node.ID})
			}
		}
	}
}

func (e *Engine) memberLeft(ctx context.Context, event adapter.Event, routes []config.Route) {
	seen := map[string]bool{}
	for _, route := range routes {
		if !route.UnbindOnLeave {
			continue
		}
		node, _ := e.Config.Node(route.NodeIDs[0])
		if seen[node.NetworkID] {
			continue
		}
		seen[node.NetworkID] = true
		records, err := e.Bindings.List(ctx, node.NetworkID, &event.Sender.Identity, 1000, 0)
		if err != nil {
			continue
		}
		for _, record := range records {
			removed, err := e.Bindings.Remove(ctx, node.NetworkID, record.Player.Name, &event.Sender.Identity)
			if err == nil {
				e.AfterUnbind(ctx, removed)
			}
		}
	}
}

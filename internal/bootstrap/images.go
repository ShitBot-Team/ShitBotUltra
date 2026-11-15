package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/forwarding"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/render"
)

func (e *Engine) registerDataProviders() error {
	for _, kind := range []string{"online", "profile", "inventory", "performance"} {
		kind := kind
		if err := e.Renderer.RegisterProvider("core."+kind, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
			var request struct {
				NetworkID  string   `json:"network_id"`
				PlayerName string   `json:"player_name"`
				ServerID   string   `json:"server_id"`
				NodeIDs    []string `json:"node_ids"`
			}
			if json.Unmarshal(raw, &request) != nil {
				return nil, fault.New("INVALID_INPUT", "图片数据请求无效")
			}
			found := false
			for _, node := range e.Config.Nodes {
				if node.NetworkID == request.NetworkID {
					found = true
					break
				}
			}
			if !found {
				return nil, fault.New("INVALID_INPUT", "未知游戏网络")
			}
			var value any
			var err error
			switch kind {
			case "online":
				players, listErr := e.Players.List(ctx, request.NetworkID, true, 1000, 0)
				err = listErr
				value = map[string]any{"network_id": request.NetworkID, "players": players, "updated_at": time.Now().UTC()}
			case "profile":
				value, err = e.Players.Get(ctx, request.NetworkID, request.PlayerName)
			case "inventory":
				value, err = e.Inventory.Get(ctx, request.NetworkID, request.ServerID, request.PlayerName, nil)
			case "performance":
				if len(request.NodeIDs) == 0 {
					for _, node := range e.Config.Nodes {
						if node.NetworkID == request.NetworkID {
							request.NodeIDs = append(request.NodeIDs, node.ID)
						}
					}
				}
				if len(request.NodeIDs) > 256 {
					return nil, fault.New("INVALID_INPUT", "游戏节点数量过多")
				}
				for _, id := range request.NodeIDs {
					node, ok := e.Config.Node(id)
					if !ok || node.NetworkID != request.NetworkID {
						return nil, fault.New("FORBIDDEN", "请求包含其他网络的游戏节点")
					}
				}
				value = map[string]any{"servers": e.Performance(ctx, request.NodeIDs), "updated_at": time.Now().UTC()}
			}
			if err != nil {
				return nil, err
			}
			data, err := json.Marshal(value)
			return json.RawMessage(data), err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) sendImage(ctx context.Context, route config.Route, template string, value any) bool {
	if !e.Renderer.Enabled() {
		return false
	}
	data, err := json.Marshal(value)
	if err != nil {
		return false
	}
	imageCtx := ctx
	cancel := func() {}
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) < 5*time.Second {
			return false
		}
		imageCtx, cancel = context.WithDeadline(ctx, deadline.Add(-3*time.Second))
	}
	defer cancel()
	result, err := e.Renderer.Render(imageCtx, render.Request{TemplateID: template, Data: data, Locale: "zh-CN"})
	if err != nil {
		e.Events.Publish("render.failed", map[string]string{"template_id": template})
		return false
	}
	_, image, err := e.Renderer.Resource(result.ResourceID)
	if err != nil {
		return false
	}
	instance := e.Adapters[route.ConnectionID]
	if instance == nil {
		return false
	}
	_, err = instance.Send(imageCtx, route.Conversation, adapter.Message{Segments: []adapter.Segment{{Type: "image", Data: image}}})
	if err != nil {
		e.Events.Publish("delivery.failed", map[string]string{"connection_id": route.ConnectionID})
		return false
	}
	return true
}

func (e *Engine) imageCommand(ctx context.Context, event adapter.Event, route config.Route, network string, args []string) bool {
	reply := func(text string) { e.send(ctx, route, adapter.Text(text)) }
	switch args[0] {
	case "tps":
		if len(args) != 1 {
			reply("用法：/tps，查询本会话对应 MC 服务器的 TPS。")
			return true
		}
		servers := e.Performance(ctx, route.NodeIDs)
		if e.sendImage(ctx, route, e.Config.Rendering.PerformanceTemplate, map[string]any{"servers": servers, "updated_at": time.Now().UTC()}) {
			return true
		}
		lines := []string{"Minecraft 服务器 TPS："}
		for _, server := range servers {
			if !server.Available || server.Metrics == nil || server.Metrics.TPS == nil {
				lines = append(lines, server.ServerID+"：不可用（"+server.ErrorCode+"）")
				continue
			}
			line := fmt.Sprintf("%s：%.2f TPS", server.ServerID, *server.Metrics.TPS)
			if server.Metrics.MSPT != nil {
				line += fmt.Sprintf(" / %.2f MSPT", *server.Metrics.MSPT)
			}
			lines = append(lines, line)
		}
		reply(forwarding.Clean(strings.Join(lines, "\n"), 3000))
		return true
	case "profile", "inventory":
		if len(args) > 2 {
			reply("用法：/" + args[0] + " [\"玩家名\"]")
			return true
		}
		name := ""
		if len(args) == 2 {
			name = args[1]
		} else {
			records, err := e.Bindings.List(ctx, network, &event.Sender.Identity, 2, 0)
			if err != nil {
				reply(fault.Public(err).Message)
				return true
			}
			if len(records) != 1 {
				reply("请在命令中指定玩家名。")
				return true
			}
			name = records[0].Player.Name
		}
		if args[0] == "profile" {
			profile, err := e.Players.Get(ctx, network, name)
			if err != nil {
				reply(fault.Public(err).Message)
				return true
			}
			if !e.sendImage(ctx, route, e.Config.Rendering.ProfileTemplate, profile) {
				state := "离线"
				if profile.Online {
					state = "在线"
				}
				reply(fmt.Sprintf("%s：%s，累计在线 %d 分钟", name, state, profile.TotalSeconds/60))
			}
			return true
		}
		binding, err := e.Bindings.Find(ctx, network, name)
		if err != nil {
			reply(fault.Public(err).Message)
			return true
		}
		if binding.Owner == nil || *binding.Owner != event.Sender.Identity {
			reply("背包只允许绑定账号查看。")
			return true
		}
		var saved *bridge.InventorySnapshot
		queryCtx, queryCancel := context.WithTimeout(ctx, 6*time.Second)
		defer queryCancel()
		for _, id := range route.NodeIDs {
			node, _ := e.Config.Node(id)
			if slices.Contains(node.Actions, "inventory.read") {
				data, err := e.Hub.Request(queryCtx, id, "inventory.read", map[string]any{"player": binding.Player})
				if err == nil {
					var fresh bridge.InventorySnapshot
					if json.Unmarshal(data, &fresh) == nil && fresh.Player.Name == name {
						_ = e.Inventory.Save(ctx, node, fresh)
					}
				}
			}
			snapshot, err := e.Inventory.Get(ctx, network, node.ServerID, name, &event.Sender.Identity)
			if err == nil && (saved == nil || saved.CapturedAt.Before(snapshot.CapturedAt)) {
				copy := snapshot
				saved = &copy
			}
		}
		if saved == nil {
			reply("没有可用的背包快照。")
			return true
		}
		if !e.sendImage(ctx, route, e.Config.Rendering.InventoryTemplate, *saved) {
			reply("已保存 " + name + " 的背包快照，采集时间：" + saved.CapturedAt.Format(time.RFC3339) + "；当前图片渲染不可用。")
		}
		return true
	default:
		return false
	}
}

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/binding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/extension"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/render"
)

func (e *Engine) ValidateGrant(manifest extension.Manifest, grant extension.Grant) error {
	if len(grant.Networks) > 256 || len(grant.Nodes) > 256 || len(grant.Routes) > 1024 || len(grant.Capabilities) > 32 {
		return fault.New("INVALID_INPUT", "插件权限范围过大")
	}
	for _, network := range grant.Networks {
		found := false
		for _, node := range e.Config.Nodes {
			if node.NetworkID == network {
				found = true
				break
			}
		}
		if !found {
			return fault.New("INVALID_INPUT", "插件权限含未知网络")
		}
	}
	for _, id := range grant.Nodes {
		node, ok := e.Config.Node(id)
		if !ok || !slices.Contains(grant.Networks, node.NetworkID) {
			return fault.New("INVALID_INPUT", "节点权限必须包含对应网络")
		}
	}
	for _, id := range grant.Routes {
		index := slices.IndexFunc(e.Config.Routes, func(route config.Route) bool { return route.ID == id })
		if index < 0 {
			return fault.New("INVALID_INPUT", "插件权限含未知路由")
		}
		for _, nodeID := range e.Config.Routes[index].NodeIDs {
			node, _ := e.Config.Node(nodeID)
			if !slices.Contains(grant.Networks, node.NetworkID) {
				return fault.New("INVALID_INPUT", "路由权限必须包含对应网络")
			}
		}
	}
	for _, command := range manifest.Commands {
		for _, configured := range e.Config.Commands {
			if command.Name == configured.Name {
				return fault.New("REVISION_CONFLICT", "插件命令与配置中的快捷命令冲突")
			}
		}
	}
	return nil
}

func (e *Engine) PluginChanged(id, state string) {
	e.Events.Publish("plugin.state", map[string]string{"plugin_id": id, "state": state})
}

func (e *Engine) Call(ctx context.Context, manifest extension.Manifest, grant extension.Grant, method string, raw json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.ctx.Err() != nil {
		return nil, fault.New("UNAVAILABLE", "Core 正在停止")
	}
	require := func(capability string) error {
		if !slices.Contains(grant.Capabilities, capability) {
			return fault.New("FORBIDDEN", "插件没有请求该服务的权限")
		}
		return nil
	}
	networkAllowed := func(network string) error {
		if !slices.Contains(grant.Networks, network) {
			return fault.New("FORBIDDEN", "插件没有目标网络的访问权限")
		}
		return nil
	}
	encode := func(value any, err error) (json.RawMessage, error) {
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(value)
		return json.RawMessage(data), err
	}
	switch method {
	case "bindings.list":
		if err := require("bindings.read"); err != nil {
			return nil, err
		}
		var request struct {
			NetworkID string `json:"network_id"`
			Limit     int    `json:"limit"`
			Offset    int    `json:"offset"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return nil, fault.New("INVALID_INPUT", "插件请求无效")
		}
		if err := networkAllowed(request.NetworkID); err != nil {
			return nil, err
		}
		if request.Limit == 0 {
			request.Limit = 100
		}
		records, err := e.Bindings.List(ctx, request.NetworkID, nil, request.Limit, request.Offset)
		return encode(records, err)
	case "bindings.add":
		if err := require("bindings.write"); err != nil {
			return nil, err
		}
		var record binding.Record
		if json.Unmarshal(raw, &record) != nil {
			return nil, fault.New("INVALID_INPUT", "插件请求无效")
		}
		if err := networkAllowed(record.NetworkID); err != nil {
			return nil, err
		}
		mode := ""
		for _, node := range e.Config.Nodes {
			if node.NetworkID == record.NetworkID {
				mode = node.IdentityMode
				break
			}
		}
		if record.Player.IdentityMode != mode {
			return nil, fault.New("IDENTITY_CONFLICT", "玩家身份模式不一致")
		}
		record.CreatedAt = time.Time{}
		record.UpdatedAt = time.Time{}
		if err := e.Bindings.Add(ctx, record); err != nil {
			return nil, err
		}
		e.Events.Publish("bindings.changed", map[string]string{"network_id": record.NetworkID})
		return json.RawMessage(`{"added":true}`), nil
	case "bindings.remove":
		if err := require("bindings.write"); err != nil {
			return nil, err
		}
		var request struct {
			NetworkID string `json:"network_id"`
			Name      string `json:"name"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return nil, fault.New("INVALID_INPUT", "插件请求无效")
		}
		if err := networkAllowed(request.NetworkID); err != nil {
			return nil, err
		}
		record, err := e.Bindings.Remove(ctx, request.NetworkID, request.Name, nil)
		if err != nil {
			return nil, err
		}
		e.AfterUnbind(ctx, record)
		return json.RawMessage(`{"removed":true}`), nil
	case "players.list":
		if err := require("players.read"); err != nil {
			return nil, err
		}
		var request struct {
			NetworkID string `json:"network_id"`
			Online    bool   `json:"online"`
			Limit     int    `json:"limit"`
			Offset    int    `json:"offset"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return nil, fault.New("INVALID_INPUT", "插件请求无效")
		}
		if err := networkAllowed(request.NetworkID); err != nil {
			return nil, err
		}
		if request.Limit == 0 {
			request.Limit = 100
		}
		players, err := e.Players.List(ctx, request.NetworkID, request.Online, request.Limit, request.Offset)
		return encode(players, err)
	case "inventory.get":
		if err := require("inventory.read"); err != nil {
			return nil, err
		}
		var request struct {
			NetworkID  string `json:"network_id"`
			ServerID   string `json:"server_id"`
			PlayerName string `json:"player_name"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return nil, fault.New("INVALID_INPUT", "插件请求无效")
		}
		if err := networkAllowed(request.NetworkID); err != nil {
			return nil, err
		}
		snapshot, err := e.Inventory.Get(ctx, request.NetworkID, request.ServerID, request.PlayerName, nil)
		return encode(snapshot, err)
	case "messages.send":
		if err := require("messages.send"); err != nil {
			return nil, err
		}
		var request struct {
			RouteID string          `json:"route_id"`
			Message adapter.Message `json:"message"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return nil, fault.New("INVALID_INPUT", "插件请求无效")
		}
		if !slices.Contains(grant.Routes, request.RouteID) {
			return nil, fault.New("FORBIDDEN", "插件没有目标会话的访问权限")
		}
		for i, segment := range request.Message.Segments {
			if segment.Type == "image" && segment.ResourceID != "" {
				if err := require("render.request"); err != nil {
					return nil, err
				}
				_, image, err := e.Renderer.Resource(segment.ResourceID)
				if err != nil {
					return nil, err
				}
				request.Message.Segments[i].Data = image
			}
		}
		for _, route := range e.Config.Routes {
			if route.ID == request.RouteID {
				connection := e.Adapters[route.ConnectionID]
				if connection == nil {
					return nil, fault.New("NODE_OFFLINE", "机器人连接不可用")
				}
				receipt, err := connection.Send(ctx, route.Conversation, request.Message)
				if errors.Is(err, adapter.ErrUnknownResult) {
					return nil, fault.New("RESULT_UNKNOWN", "消息发送结果未知")
				}
				return encode(receipt, err)
			}
		}
		return nil, fault.New("NOT_FOUND", "路由不存在")
	case "game.request":
		var request struct {
			NodeID  string          `json:"node_id"`
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if json.Unmarshal(raw, &request) != nil {
			return nil, fault.New("INVALID_INPUT", "插件请求无效")
		}
		if err := require("game." + request.Type); err != nil {
			return nil, err
		}
		if !slices.Contains(grant.Nodes, request.NodeID) {
			return nil, fault.New("FORBIDDEN", "插件没有目标节点的访问权限")
		}
		return e.Hub.Request(ctx, request.NodeID, request.Type, request.Payload)
	case "render.request":
		if err := require("render.request"); err != nil {
			return nil, err
		}
		var request render.Request
		if json.Unmarshal(raw, &request) != nil {
			return nil, fault.New("INVALID_INPUT", "图片请求无效")
		}
		result, err := e.Renderer.Render(ctx, request)
		return encode(result, err)
	default:
		return nil, fault.New("UNSUPPORTED_CAPABILITY", "插件宿主服务不存在")
	}
}

// Commands supplied by plugins go through the same route, role, binding,
// argument and cooldown checks as configured game commands.
func (e *Engine) pluginCommand(ctx context.Context, event adapter.Event, route config.Route, network string, args []string) bool {
	id, definition, found := e.Plugins.Command(args[0], route.ID, network)
	if !found {
		return false
	}
	reply := func(message string) { e.send(ctx, route, adapter.Text(message)) }
	playerName := ""
	if definition.RequireBinding {
		records, err := e.Bindings.List(ctx, network, &event.Sender.Identity, 2, 0)
		if err != nil {
			reply(fault.Public(err).Message)
			return true
		}
		if len(records) != 1 {
			reply("此插件命令要求当前账号恰好绑定一个玩家。")
			return true
		}
		playerName = records[0].Player.Name
	}
	policy := config.Command{Name: "plugin." + id + "." + definition.Name, Arguments: definition.Arguments, Roles: definition.Roles, RequireBinding: definition.RequireBinding, CooldownSeconds: definition.CooldownSeconds}
	if _, err := e.commands.Prepare(policy, event.Sender, playerName, args[1:]); err != nil {
		reply(fault.Public(err).Message)
		return true
	}
	input, _ := json.Marshal(map[string]any{"name": definition.Name, "arguments": args[1:], "actor": event.Sender, "player_name": playerName, "route_id": route.ID, "network_id": network})
	data, err := e.Plugins.RunCommand(ctx, id, input)
	if err != nil {
		reply(fault.Public(err).Message)
		return true
	}
	if string(data) == "null" {
		return true
	}
	var text string
	if json.Unmarshal(data, &text) == nil {
		reply(text)
		return true
	}
	var message adapter.Message
	if json.Unmarshal(data, &message) != nil {
		reply("插件返回了无效消息。")
		return true
	}
	e.send(ctx, route, message)
	return true
}

package bootstrap

import (
	"context"
	"errors"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

// executeTask is the only transport dependency of the built-in task service.
// It sends one action, never queues for reconnect and never retries a failure.
func (e *Engine) executeTask(ctx context.Context, task config.Task, runID string) error {
	a := task.Action
	switch a.Kind {
	case "bot.message":
		for _, route := range e.Config.Routes {
			if route.ID != a.RouteID {
				continue
			}
			instance := e.Adapters[route.ConnectionID]
			if instance == nil {
				return fault.New("UNAVAILABLE", "机器人连接未启用")
			}
			_, err := instance.Send(ctx, route.Conversation, adapter.Text(a.Text))
			switch {
			case errors.Is(err, adapter.ErrOffline):
				return fault.New("UNAVAILABLE", "机器人连接离线")
			case errors.Is(err, adapter.ErrBusy):
				return fault.New("BUSY", "机器人请求队列已满")
			case errors.Is(err, adapter.ErrUnsupported):
				return fault.New("UNSUPPORTED_CAPABILITY", "机器人不支持该会话发送")
			case errors.Is(err, adapter.ErrUnknownResult):
				return fault.New("RESULT_UNKNOWN", "机器人消息结果未知")
			default:
				return err
			}
		}
		return fault.New("NOT_FOUND", "任务路由不存在")
	case "chat.send":
		_, err := e.Hub.Request(ctx, a.NodeID, a.Kind, bridge.Broadcast{Text: a.Text, OriginID: "task:" + runID})
		return err
	case "command.execute":
		_, err := e.Hub.Request(ctx, a.NodeID, a.Kind, bridge.Command{Command: a.Text, ActorID: "task:" + task.ID})
		return err
	default:
		return fault.New("INVALID_INPUT", "任务动作无效")
	}
}

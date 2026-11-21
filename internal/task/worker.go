package task

import (
	"context"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

func (s *Service) worker() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case run := <-s.queue:
			s.work(run)
		}
	}
}

func (s *Service) work(run Run) {
	s.mu.Lock()
	item := s.byID[run.TaskID]
	paused := s.storageFailed || s.closed || s.ctx.Err() != nil
	s.mu.Unlock()
	if paused {
		s.finish(run, "canceled", "CANCELED", "调度器已暂停或关闭，尚未发送的任务已取消")
		return
	}
	if time.Since(run.ScheduledAt) > maxDelay {
		s.finish(run, "skipped", "DEADLINE_EXCEEDED", "排队超过 30 秒，未发送")
		return
	}
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	now := time.Now().UTC()
	run.StartedAt = &now
	s.mu.Lock()
	if s.storageFailed || s.closed || s.ctx.Err() != nil {
		s.mu.Unlock()
		s.finish(run, "canceled", "CANCELED", "任务在发送前已取消")
		return
	}
	err := s.repo.start(ctx, run)
	if err != nil {
		s.pauseLocked()
		item.running = false
		s.mu.Unlock()
		// No action was sent. Leave the durable record untouched if the write
		// itself was ambiguous; recovery will classify it conservatively.
		return
	}
	s.mu.Unlock()
	if ctx.Err() != nil {
		s.finish(run, "canceled", "CANCELED", "任务在发送前已取消")
		return
	}
	if time.Since(run.ScheduledAt) > maxDelay {
		s.finish(run, "skipped", "DEADLINE_EXCEEDED", "已超过发送期限，未发送")
		return
	}
	run.Status = "running"
	s.emit("tasks.changed", run)
	err = s.dispatch(ctx, item.config, run.ID)
	if err == nil {
		s.finish(run, "succeeded", "", "目标已返回成功响应")
		return
	}
	code := fault.Public(err).Code
	status, message := "failed", "目标拒绝或未能执行；请检查连接、权限和目标状态"
	switch code {
	case "INVALID_INPUT", "FORBIDDEN", "UNSUPPORTED_CAPABILITY", "NODE_OFFLINE", "UNAVAILABLE", "BUSY", "RATE_LIMITED", "NOT_FOUND", "CAPACITY_EXCEEDED":
		// Stable local/pre-dispatch failures; never persist remote error text.
	case "RESULT_UNKNOWN", "DEADLINE_EXCEEDED", "CANCELED", "INTERNAL", "INVALID_RESPONSE":
		status, message = "unknown", "无法确认目标执行结果；请核实目标状态，未自动重试"
	default:
		status, code, message = "unknown", "RESULT_UNKNOWN", "目标返回无法识别的错误；请核实目标状态，未自动重试"
	}
	s.finish(run, status, code, message)
}

func (s *Service) dispatch(ctx context.Context, task config.Task, runID string) (err error) {
	defer func() {
		if recover() != nil {
			err = fault.New("RESULT_UNKNOWN", "任务执行器异常，执行结果未知")
		}
	}()
	return s.execute(ctx, task, runID)
}

func (s *Service) finish(run Run, status, code, message string) {
	now := time.Now().UTC()
	run.Status, run.ErrorCode, run.Message, run.FinishedAt = status, code, message, &now
	// Shutdown cancellation must not prevent recording a completed side effect.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err := s.repo.finish(ctx, run)
	cancel()
	s.mu.Lock()
	s.byID[run.TaskID].running = false
	if err != nil {
		s.pauseLocked()
	}
	s.mu.Unlock()
	if err == nil {
		s.emit("tasks.changed", run)
	}
}

package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
)

const (
	workerCount = 4
	queueLimit  = 64
	maxDelay    = 30 * time.Second
)

type Execute func(context.Context, config.Task, string) error

type entry struct {
	config     config.Task
	schedule   schedule
	next       time.Time
	running    bool // Includes queued work: a task cannot overlap itself.
	lastManual time.Time
}

type Item struct {
	Config  config.Task `json:"config"`
	NextAt  *time.Time  `json:"next_at,omitempty"`
	Running bool        `json:"running"`
}

type Snapshot struct {
	Revision string `json:"revision"`
	Paused   bool   `json:"paused"`
	Reason   string `json:"reason"`
	Items    []Item `json:"items"`
}

type Service struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	repo          repository
	entries       []*entry
	byID          map[string]*entry
	revision      string
	execute       Execute
	emit          func(string, any)
	logger        *slog.Logger
	queue         chan Run
	wg            sync.WaitGroup
	started       bool
	closed        bool
	storageFailed bool
	closeOnce     sync.Once
}

func New(ctx context.Context, db storage.Database, cfg config.Config, execute Execute, emit func(string, any), logger *slog.Logger) (*Service, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{ctx: ctx, cancel: cancel, repo: repository{db: db}, byID: map[string]*entry{}, revision: hex.EncodeToString(digest[:]), execute: execute, emit: emit, logger: logger, queue: make(chan Run, queueLimit)}
	for _, task := range cfg.Tasks {
		plan, err := newSchedule(task.Schedule)
		if err != nil {
			cancel()
			return nil, err
		}
		item := &entry{config: task, schedule: plan}
		s.entries = append(s.entries, item)
		s.byID[task.ID] = item
	}
	return s, nil
}

func (s *Service) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed || s.ctx.Err() != nil {
		return fault.New("UNAVAILABLE", "定时任务不能重复启动或在关闭后启动")
	}
	if err := s.repo.recover(s.ctx); err != nil {
		return err
	}
	now := time.Now()
	for _, item := range s.entries {
		if item.config.Enabled {
			item.next = item.schedule.next(now)
		}
	}
	s.started = true
	s.wg.Add(workerCount + 1)
	for range workerCount {
		go s.worker()
	}
	go s.loop()
	return nil
}

func (s *Service) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		s.mu.Unlock()
		s.wg.Wait()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.repo.recover(ctx); err != nil {
			s.logger.Warn("定时任务关闭记录未完成；下次启动将恢复未完成状态")
		}
	})
}

func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Snapshot{Revision: s.revision, Items: []Item{}}
	if s.storageFailed {
		result.Paused, result.Reason = true, "任务记录写入失败，已停止接收新任务；检查存储并重启宿主"
	} else if !s.started || s.closed || s.ctx.Err() != nil {
		result.Paused, result.Reason = true, "调度器尚未启动或正在关闭"
	}
	for _, item := range s.entries {
		value := Item{Config: item.config, Running: item.running}
		if !result.Paused && !item.next.IsZero() {
			at := item.next
			value.NextAt = &at
		}
		result.Items = append(result.Items, value)
	}
	return result
}

var manualID = regexp.MustCompile(`^([0-9]{13})-[a-f0-9]{32}$`)

// Manual uses a caller-generated timestamp + random ID. Reusing it returns the
// same run. A missing, old ID cannot become a fresh action after log retention.
func (s *Service) Manual(ctx context.Context, taskID, requestID, revision string) (Run, error) {
	match := manualID.FindStringSubmatch(requestID)
	if match == nil {
		return Run{}, fault.New("INVALID_INPUT", "request_id 必须是 13 位 Unix 毫秒时间戳、连字符和 32 位随机小写十六进制")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := s.repo.get(ctx, requestID)
	if err == nil {
		if previous.TaskID != taskID {
			return Run{}, fault.New("REVISION_CONFLICT", "request_id 已用于其他任务")
		}
		return previous, nil
	}
	if !fault.Is(err, "NOT_FOUND") {
		return Run{}, err
	}
	if !s.started || s.closed || s.storageFailed || s.ctx.Err() != nil {
		return Run{}, fault.New("UNAVAILABLE", "调度器当前不接收新任务")
	}
	item := s.byID[taskID]
	if item == nil {
		return Run{}, fault.New("NOT_FOUND", "任务未加载；保存配置后需重启宿主")
	}
	if revision != s.revision {
		return Run{}, fault.New("REVISION_CONFLICT", "运行配置已变化，请重新读取正在运行的任务")
	}
	if !item.config.Enabled {
		return Run{}, fault.New("FORBIDDEN", "禁用的任务不能手动执行")
	}
	stamp, _ := strconv.ParseInt(match[1], 10, 64)
	now := time.Now().UTC()
	at := time.UnixMilli(stamp)
	if now.Sub(at) > 5*time.Minute || at.After(now.Add(30*time.Second)) {
		return Run{}, fault.New("INVALID_INPUT", "此次请求已过期或客户端时钟偏差过大；先确认执行记录，不要直接重发")
	}
	if item.running || len(s.queue) >= queueLimit {
		return Run{}, fault.New("BUSY", "任务仍在排队或执行，或全局任务队列已满")
	}
	if now.Sub(item.lastManual) < 10*time.Second {
		return Run{}, fault.New("RATE_LIMITED", "同一任务手动提交至少间隔 10 秒")
	}
	run := s.newRun(item, requestID, "manual", now, now)
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	// Acceptance is owned by Core once begun. Losing the browser connection
	// cannot cancel the commit between its reservation and the queue handoff.
	acceptCtx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	inserted, err := s.repo.insert(acceptCtx, run, "manual:"+requestID, at)
	cancel()
	if err != nil {
		// A storage timeout may race COMMIT. Do not execute an ambiguous claim.
		s.pauseLocked()
		return Run{}, err
	}
	if !inserted {
		return Run{}, fault.New("REVISION_CONFLICT", "请求不新于已受理的手动请求，请核对执行记录与客户端时间")
	}
	item.running, item.lastManual = true, now
	s.queue <- run // Only producers holding mu can fill the bounded channel.
	s.emit("tasks.changed", run)
	return run, nil
}

func (s *Service) Get(ctx context.Context, id string) (Run, error) {
	if len(id) < 1 || len(id) > 64 {
		return Run{}, fault.New("INVALID_INPUT", "任务记录 ID 无效")
	}
	return s.repo.get(ctx, id)
}

func (s *Service) List(ctx context.Context, taskID string, limit, offset int) ([]Run, error) {
	if len(taskID) > 64 || limit < 1 || limit > 1000 || offset < 0 {
		return nil, fault.New("INVALID_INPUT", "任务查询参数无效")
	}
	return s.repo.list(ctx, taskID, limit, offset)
}

func (s *Service) newRun(item *entry, id, trigger string, due, now time.Time) Run {
	target := item.config.Action.NodeID
	if item.config.Action.Kind == "bot.message" {
		target = item.config.Action.RouteID
	}
	return Run{ID: id, TaskID: item.config.ID, TaskName: item.config.Name, Revision: s.revision, ActionKind: item.config.Action.Kind, TargetID: target, Trigger: trigger, ScheduledAt: due.UTC(), CreatedAt: now.UTC(), Status: "queued"}
}

func (s *Service) pauseLocked() {
	if !s.storageFailed {
		s.storageFailed = true
		s.logger.Warn("任务记录写入失败，调度器已暂停接收新任务")
		s.emit("tasks.paused", map[string]string{"code": "STORAGE_UNAVAILABLE"})
	}
}

func (s *Service) loop() {
	defer s.wg.Done()
	tick := time.NewTicker(time.Second)
	cleanup := time.NewTicker(time.Minute)
	defer tick.Stop()
	defer cleanup.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-tick.C:
			s.tick()
		case <-cleanup.C:
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			err := s.repo.cleanup(ctx)
			cancel()
			if err != nil && s.ctx.Err() == nil {
				s.logger.Warn("定时任务记录清理未完成")
			}
		}
	}
}

func (s *Service) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.storageFailed || s.ctx.Err() != nil {
		return
	}
	now := time.Now().UTC()
	for _, item := range s.entries {
		if item.next.IsZero() || item.next.After(now) {
			continue
		}
		due := item.next
		item.next = item.schedule.next(now)
		run := s.newRun(item, securefs.ID(), "scheduled", due, now)
		if now.Sub(due) > maxDelay {
			run.Status, run.ErrorCode, run.Message = "skipped", "DEADLINE_EXCEEDED", "错过计划时点超过 30 秒，未补发"
		} else if item.running || len(s.queue) >= queueLimit {
			run.Status, run.ErrorCode, run.Message = "skipped", "BUSY", "同一任务尚未完成或任务队列已满，未补发"
		}
		if run.Status == "skipped" {
			run.FinishedAt = &now
		}
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		inserted, err := s.repo.insert(ctx, run, item.schedule.slot(due), due)
		cancel()
		if err != nil {
			s.pauseLocked()
			return
		}
		if !inserted {
			continue
		}
		if run.Status == "queued" {
			item.running = true
			s.queue <- run
		}
		s.emit("tasks.changed", run)
	}
}

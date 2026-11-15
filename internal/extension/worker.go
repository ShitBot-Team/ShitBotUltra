package extension

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	sdk "github.com/ShitBot-Team/ShitBotUltra/sdk/extension"
	"github.com/dop251/goja"
)

type Host interface {
	ValidateGrant(sdk.Manifest, sdk.Grant) error
	Call(context.Context, sdk.Manifest, sdk.Grant, string, json.RawMessage) (json.RawMessage, error)
	PluginChanged(string, string)
}

type task struct {
	ctx     context.Context
	kind    string
	name    string
	payload json.RawMessage
	reply   chan outcome
}
type outcome struct {
	data json.RawMessage
	err  error
}
type worker struct {
	ctx           context.Context
	cancel        context.CancelFunc
	manifest      sdk.Manifest
	grant         sdk.Grant
	configuration json.RawMessage
	source        string
	dataDir       string
	host          Host
	queue         chan task
	done          chan struct{}
	ready         chan error
	timeout       time.Duration
	mu            sync.Mutex
	lastError     string
	dropped       atomic.Uint64
}

func newWorker(ctx context.Context, host Host, manifest sdk.Manifest, grant sdk.Grant, configuration json.RawMessage, source, dataDir string) *worker {
	ctx, cancel := context.WithCancel(ctx)
	return &worker{ctx: ctx, cancel: cancel, host: host, manifest: manifest, grant: grant, configuration: append(json.RawMessage(nil), configuration...), source: source, dataDir: dataDir, queue: make(chan task, 64), done: make(chan struct{}), ready: make(chan error, 1), timeout: 5 * time.Second}
}

func (w *worker) fail(code string) {
	w.mu.Lock()
	w.lastError = code
	w.mu.Unlock()
	w.host.PluginChanged(w.manifest.ID, code)
}
func (w *worker) status() (string, uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastError, w.dropped.Load()
}
func (w *worker) stop() { w.cancel(); <-w.done }
func (w *worker) submit(ctx context.Context, kind, name string, data json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	reply := make(chan outcome, 1)
	t := task{ctx: ctx, kind: kind, name: name, payload: append(json.RawMessage(nil), data...), reply: reply}
	select {
	case <-w.ctx.Done():
		return nil, fault.New("PLUGIN_DISABLED", "插件已停止")
	case <-ctx.Done():
		return nil, ctx.Err()
	case w.queue <- t:
	default:
		return nil, fault.New("BUSY", "插件任务队列已满")
	}
	select {
	case result := <-reply:
		return result.data, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.done:
		return nil, fault.New("PLUGIN_FAILED", "插件已停止")
	}
}

func (w *worker) event(event sdk.Event) {
	if !slices.Contains(w.grant.Capabilities, "events") || !slices.Contains(w.manifest.Events, event.Type) {
		return
	}
	if event.NetworkID != "" && !slices.Contains(w.grant.Networks, event.NetworkID) {
		return
	}
	if event.RouteID != "" && !slices.Contains(w.grant.Routes, event.RouteID) {
		return
	}
	if event.NodeID != "" && !slices.Contains(w.grant.Nodes, event.NodeID) {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	select {
	case <-w.ctx.Done():
	case w.queue <- task{ctx: w.ctx, kind: "event", payload: data}:
	default:
		w.dropped.Add(1)
	}
}

func (w *worker) run() {
	defer close(w.done)
	defer w.cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			w.fail("PLUGIN_PANIC")
			select {
			case w.ready <- errors.New("plugin worker failed"):
			default:
			}
		}
	}()
	vm := goja.New()
	vm.SetMaxCallStackSize(256)
	var callContext context.Context = w.ctx
	jsonObject := vm.Get("JSON").ToObject(vm)
	parse, _ := goja.AssertFunction(jsonObject.Get("parse"))
	stringify, _ := goja.AssertFunction(jsonObject.Get("stringify"))
	toValue := func(raw json.RawMessage) goja.Value {
		if len(raw) == 0 {
			return goja.Undefined()
		}
		value, err := parse(goja.Undefined(), vm.ToValue(string(raw)))
		if err != nil {
			panic(vm.NewGoError(errors.New("invalid host JSON")))
		}
		return value
	}
	serialize := func(value goja.Value) (json.RawMessage, error) {
		encoded, err := stringify(goja.Undefined(), value)
		if err != nil {
			return nil, err
		}
		if goja.IsUndefined(encoded) {
			return json.RawMessage("null"), nil
		}
		data := json.RawMessage(encoded.String())
		if len(data) > 2<<20 {
			return nil, errors.New("plugin JSON exceeds limit")
		}
		return data, nil
	}
	hostCall := func(method string, value goja.Value) goja.Value {
		if callContext.Err() != nil || w.ctx.Err() != nil {
			panic(vm.NewGoError(errors.New("plugin call canceled")))
		}
		data, err := serialize(value)
		if err != nil {
			panic(vm.NewGoError(errors.New("invalid plugin call data")))
		}
		var result json.RawMessage
		if method == "storage.get" || method == "storage.set" {
			result, err = w.storage(method, data)
		} else {
			result, err = w.host.Call(callContext, w.manifest, w.grant, method, data)
		}
		if err != nil {
			public := fault.Public(err)
			panic(vm.NewGoError(fmt.Errorf("%s: %s", public.Code, public.Message)))
		}
		return toValue(result)
	}
	if err := vm.Set("ultra", map[string]any{"apiVersion": sdk.APIVersion, "call": hostCall}); err != nil {
		w.ready <- err
		return
	}
	exports := vm.NewObject()
	if err := vm.Set("exports", exports); err != nil {
		w.ready <- err
		return
	}
	// No require(), Node.js globals, filesystem, network or Go reflection object
	// is injected. Plugins are administrator-trusted code, not an OS sandbox.
	run := func(parent context.Context, fn func() (goja.Value, error)) (json.RawMessage, error) {
		ctx, cancel := context.WithTimeout(parent, w.timeout)
		defer cancel()
		callContext = ctx
		stopParent := context.AfterFunc(w.ctx, cancel)
		defer stopParent()
		interrupted := make(chan struct{})
		stopInterrupt := context.AfterFunc(ctx, func() { vm.Interrupt("deadline exceeded"); close(interrupted) })
		// JSON conversion can execute getters/toJSON. Keep the interrupt active
		// until conversion finishes, not just until the callback returns.
		defer func() {
			if !stopInterrupt() {
				<-interrupted
			}
			vm.ClearInterrupt()
		}()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := fn()
		if err != nil {
			return nil, fault.New("PLUGIN_FAILED", "插件执行异常或超时")
		}
		if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
			return json.RawMessage("null"), nil
		}
		// Promise-based modules must bundle a synchronous export; do not report
		// an unresolved asynchronous result as successful completion.
		if _, promise := value.Export().(*goja.Promise); promise {
			return nil, fault.New("PLUGIN_FAILED", "插件回调必须返回同步 JSON 结果")
		}
		data, err := serialize(value)
		if err != nil {
			return nil, fault.New("PLUGIN_FAILED", "插件返回值不是有界 JSON")
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return data, nil
	}
	invoke := func(parent context.Context, name string, payload json.RawMessage) (json.RawMessage, error) {
		return run(parent, func() (goja.Value, error) {
			value := exports.Get(name)
			if value == nil || goja.IsUndefined(value) {
				return goja.Undefined(), nil
			}
			callback, ok := goja.AssertFunction(value)
			if !ok {
				return nil, errors.New("export is not callable")
			}
			return callback(exports, toValue(payload))
		})
	}
	_, err := run(w.ctx, func() (goja.Value, error) { return vm.RunScript(w.manifest.ID+"/"+w.manifest.Entry, w.source) })
	if err == nil {
		_, err = invoke(w.ctx, "init", w.configuration)
	}
	if err == nil {
		_, err = invoke(w.ctx, "enable", json.RawMessage(`{}`))
	}
	w.ready <- err
	if err != nil {
		w.fail("PLUGIN_INIT_FAILED")
		return
	}
	var ticks <-chan time.Time
	var timer *time.Ticker
	if w.manifest.TickSeconds > 0 && slices.Contains(w.grant.Capabilities, "timers") {
		timer = time.NewTicker(time.Duration(w.manifest.TickSeconds) * time.Second)
		defer timer.Stop()
		ticks = timer.C
	}
	for {
		select {
		case <-w.ctx.Done():
			// Shutdown callbacks have a separate finite budget. Host calls still see
			// a canceled worker and cannot schedule new actions after disable.
			vm.ClearInterrupt()
			callContext = w.ctx
			shutdown, done := context.WithTimeout(context.Background(), time.Second)
			interrupted := make(chan struct{})
			stop := context.AfterFunc(shutdown, func() { vm.Interrupt("plugin disabled"); close(interrupted) })
			for _, name := range []string{"disable", "dispose"} {
				if callback, ok := goja.AssertFunction(exports.Get(name)); ok {
					_, _ = callback(exports)
				}
			}
			if !stop() {
				<-interrupted
			}
			done()
			return
		case <-ticks:
			if _, err := invoke(w.ctx, "tick", json.RawMessage(`{}`)); err != nil {
				w.fail("PLUGIN_TICK_FAILED")
			}
		case t := <-w.queue:
			if t.ctx.Err() != nil {
				if t.reply != nil {
					t.reply <- outcome{err: t.ctx.Err()}
				}
				continue
			}
			var data json.RawMessage
			var err error
			if t.kind == "provider" {
				data, err = run(t.ctx, func() (goja.Value, error) {
					providers := exports.Get("providers")
					if providers == nil || goja.IsUndefined(providers) {
						return nil, errors.New("provider missing")
					}
					callback, ok := goja.AssertFunction(providers.ToObject(vm).Get(t.name))
					if !ok {
						return nil, errors.New("provider missing")
					}
					return callback(goja.Undefined(), toValue(t.payload))
				})
			} else {
				name := "onEvent"
				if t.kind == "command" {
					name = "onCommand"
				}
				data, err = invoke(t.ctx, name, t.payload)
			}
			if err != nil {
				w.fail("PLUGIN_CALLBACK_FAILED")
			}
			if t.reply != nil {
				t.reply <- outcome{data: data, err: err}
			}
		}
	}
}

func (w *worker) storage(method string, data json.RawMessage) (json.RawMessage, error) {
	if w.ctx.Err() != nil {
		return nil, fault.New("PLUGIN_DISABLED", "插件已停用")
	}
	if !slices.Contains(w.grant.Capabilities, "storage") {
		return nil, fault.New("FORBIDDEN", "插件没有存储权限")
	}
	var request struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(data, &request) != nil || !packagefs.ValidID(request.Key) {
		return nil, fault.New("INVALID_INPUT", "插件存储键无效")
	}
	path := filepath.Join(w.dataDir, request.Key+".json")
	if method == "storage.get" {
		value, err := securefs.ReadFile(path, 64<<10)
		if errors.Is(err, os.ErrNotExist) {
			return json.RawMessage("null"), nil
		}
		return value, err
	}
	if len(request.Value) == 0 || len(request.Value) > 64<<10 || !json.Valid(request.Value) {
		return nil, fault.New("INVALID_INPUT", "插件存储值超过限制或 JSON 无效")
	}
	entries, err := os.ReadDir(w.dataDir)
	if err != nil {
		return nil, err
	}
	total := int64(len(request.Value))
	keys := 0
	for _, entry := range entries {
		if entry.Name() == request.Key+".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		total += info.Size()
		keys++
	}
	if total > 256<<10 || keys >= 64 {
		return nil, fault.New("CAPACITY_EXCEEDED", "插件存储配额已满")
	}
	if err := securefs.AtomicWrite(path, request.Value); err != nil {
		return nil, err
	}
	return json.RawMessage(`{"saved":true}`), nil
}

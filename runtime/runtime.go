package runtime

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/bootstrap"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/gateway/admin"
	"github.com/ShitBot-Team/ShitBotUltra/internal/instance"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage/sqlite"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

var ErrAlreadyRun = errors.New("Core runtime is single-use; create a new runtime to restart")

type Runtime struct {
	options   Options
	mu        sync.RWMutex
	status    Status
	engine    *bootstrap.Engine
	cancel    context.CancelFunc
	done      chan struct{}
	ready     chan struct{}
	readyOnce sync.Once
	startErr  error
}

func New(options Options) (*Runtime, error) {
	if options.ConfigDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		options.ConfigDir = filepath.Join(base, "ShitBotUltra")
	}
	dir, err := filepath.Abs(options.ConfigDir)
	if err != nil {
		return nil, err
	}
	options.ConfigDir = filepath.Clean(dir)
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Version == "" {
		options.Version = "dev"
	}
	factories := map[string]adapter.Factory{}
	for kind, factory := range options.AdapterFactories {
		if factory == nil || !adapter.ValidID(kind) || kind == "onebot.v11" {
			return nil, fmt.Errorf("invalid or reserved adapter type %q", kind)
		}
		factories[kind] = factory
	}
	options.AdapterFactories = factories
	if options.Updates != nil {
		updates := *options.Updates
		options.Updates = &updates
	}
	if options.WebUI != nil {
		enabled := *options.WebUI
		options.WebUI = &enabled
	}
	return &Runtime{options: options, done: make(chan struct{}), ready: make(chan struct{}), status: Status{Component: ComponentID, Version: options.Version, AdminAPIVersion: AdminAPIVersion, BridgeProtocolVersion: bridge.Version, State: "new", Nodes: []NodeStatus{}, Adapters: []adapter.Status{}}}, nil
}

// ValidateConfig performs only in-memory YAML decoding and validation. It never
// takes a lock, creates files, opens a database or connects an adapter.
func ValidateConfig(data []byte) Validation {
	_, err := config.Decode(data)
	if err != nil {
		return Validation{Errors: []string{err.Error()}}
	}
	return Validation{Valid: true, Errors: []string{}}
}

// Run blocks until cancellation or a fatal listener error. The host owns signal
// handling. Cancellation is a normal shutdown and returns nil.
func (r *Runtime) Run(parent context.Context) (runErr error) {
	if parent == nil {
		return errors.New("Run requires a context")
	}
	r.mu.Lock()
	if r.status.State != "new" {
		r.mu.Unlock()
		return ErrAlreadyRun
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.status.State = "starting"
	r.status.InstanceID = securefs.ID()
	r.mu.Unlock()
	defer func() {
		cancel()
		r.mu.Lock()
		r.engine = nil
		r.startErr = runErr
		if runErr != nil {
			r.status.State = "failed"
			r.status.LastError = "Core 启动或服务运行失败，请查看宿主返回的错误"
		} else {
			r.status.State = "stopped"
		}
		r.mu.Unlock()
		r.readyOnce.Do(func() { close(r.ready) })
		close(r.done)
	}()
	if err := ctx.Err(); err != nil {
		return nil
	}
	dir := r.options.ConfigDir
	if err := securefs.PrivateDir(dir); err != nil {
		return err
	}
	// Resolve symlinks before locking so aliases of one directory share ownership.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	dir = resolved
	lock, err := instance.Acquire(dir)
	if err != nil {
		return err
	}
	defer lock.Close()
	cs, cfg, err := config.Open(dir)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	var tlsConfig *tls.Config
	if cfg.HTTP.TLSCert != "" {
		cert, err := tls.LoadX509KeyPair(config.ResolvePath(dir, cfg.HTTP.TLSCert), config.ResolvePath(dir, cfg.HTTP.TLSKey))
		if err != nil {
			return fmt.Errorf("load listener TLS: %w", err)
		}
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
	}
	db, err := sqlite.Open(ctx, filepath.Join(dir, "data", "core.sqlite"))
	if err != nil {
		return fmt.Errorf("open Core database: %w", err)
	}
	defer db.Close()
	engine, err := bootstrap.New(ctx, dir, cfg, cs, db, r.options.Logger, r.options.AdapterFactories)
	if err != nil {
		return err
	}
	defer engine.Close()
	if err := engine.ConfigureUpdates(r.options.Updates); err != nil {
		return err
	}
	if r.options.WebUI != nil {
		engine.WebUI.SetEnabled(*r.options.WebUI)
	}
	var listen net.ListenConfig
	listener, err := listen.Listen(ctx, "tcp", cfg.HTTP.Listen)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	address := listener.Addr().String()
	scheme := "http"
	if tlsConfig != nil {
		scheme = "https"
		listener = tls.NewListener(listener, tlsConfig)
	}
	mux := http.NewServeMux()
	mux.Handle("/bridge/v1", engine.Hub)
	management := admin.New(engine, func() any { return r.Status() }, address)
	mux.Handle("/api/", management)
	mux.Handle("/", management.Pages())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }}
	defer server.Close()
	if err := engine.Start(); err != nil {
		return err
	}
	started := time.Now().UTC()
	endpoint := scheme + "://" + address
	if err := lock.Publish(instance.Info{InstanceID: r.Status().InstanceID, URL: endpoint, StartedAt: started}); err != nil {
		return err
	}
	r.mu.Lock()
	r.engine = engine
	r.status.State = "running"
	r.status.StartedAt = started
	r.status.URL = endpoint
	r.mu.Unlock()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	r.readyOnce.Do(func() { close(r.ready) })
	engine.Events.Publish("core.started", map[string]string{"version": r.options.Version})
	r.options.Logger.Info("ShitBotUltra Core 已启动", "address", endpoint)
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			runErr = err
		}
	}
	r.mu.Lock()
	r.status.State = "stopping"
	r.mu.Unlock()
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
	}
	return runErr
}

// WaitReady returns startup errors to hosts without requiring polling. A stopped
// runtime is not ready, including cancellation during initialization.
func (r *Runtime) WaitReady(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ready:
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.startErr != nil {
		return r.startErr
	}
	if r.status.State != "running" {
		return errors.New("Core is not running")
	}
	return nil
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	r.mu.Lock()
	if r.status.State == "new" {
		r.status.State = "stopped"
		r.readyOnce.Do(func() { close(r.ready) })
		close(r.done)
		r.mu.Unlock()
		return nil
	}
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Runtime) Status() Status {
	r.mu.RLock()
	status := r.status
	engine := r.engine
	r.mu.RUnlock()
	status.Nodes = []NodeStatus{}
	status.Adapters = []adapter.Status{}
	if engine != nil {
		status.WebUIEnabled = engine.WebUI.Status().Enabled
		status.Adapters = engine.AdapterStatus()
		for _, node := range engine.Hub.Status() {
			status.Nodes = append(status.Nodes, NodeStatus{ID: node.ID, NetworkID: node.NetworkID, ServerID: node.ServerID, Connected: node.Connected, BootID: node.BootID, Platform: node.Platform, Capabilities: node.Capabilities, AllowedActions: node.AllowedActions, Since: node.Since})
		}
	}
	return status
}

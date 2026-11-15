package extension

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	sdk "github.com/ShitBot-Team/ShitBotUltra/sdk/extension"
)

type Version struct {
	Revision   string       `json:"revision"`
	Manifest   sdk.Manifest `json:"manifest"`
	ImportedAt time.Time    `json:"imported_at"`
}
type Plugin struct {
	ID             string          `json:"id"`
	Revision       string          `json:"revision"`
	ActiveRevision string          `json:"active_revision"`
	Enabled        bool            `json:"enabled"`
	Grant          sdk.Grant       `json:"grant"`
	Configuration  json.RawMessage `json:"configuration"`
	Versions       []Version       `json:"versions"`
	State          string          `json:"state,omitempty"`
	LastError      string          `json:"last_error,omitempty"`
	Dropped        uint64          `json:"dropped"`
}

type Manager struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	ops     sync.Mutex
	dir     string
	dataDir string
	host    Host
	workers map[string]*worker
	closed  bool
}

func New(ctx context.Context, dir string, host Host) (*Manager, error) {
	packages := filepath.Join(dir, "plugins")
	data := filepath.Join(dir, "plugin-data")
	for _, path := range []string{packages, data} {
		if err := securefs.PrivateDir(path); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Manager{ctx: ctx, cancel: cancel, dir: packages, dataDir: data, host: host, workers: map[string]*worker{}}, nil
}

func validate(manifest sdk.Manifest) error {
	if manifest.APIVersion != sdk.APIVersion || !packagefs.ValidID(manifest.ID) || !packagefs.ValidVersion(manifest.Version) || len(manifest.Name) == 0 || len(manifest.Name) > 128 || len(manifest.Author) == 0 || len(manifest.Author) > 256 || len(manifest.License) == 0 || len(manifest.License) > 256 || !packagefs.SafePath(manifest.Entry) || !strings.HasSuffix(manifest.Entry, ".js") {
		return errors.New("invalid plugin manifest")
	}
	u, err := url.Parse(manifest.ProjectURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("plugin requires an HTTPS project URL")
	}
	allowed := []string{"events", "commands", "timers", "storage", "bindings.read", "bindings.write", "players.read", "inventory.read", "messages.send", "render.request", "render.provider", "game.chat.send", "game.player.kick", "game.command.execute", "game.inventory.read", "game.tps.read", "game.permission.check", "game.placeholder.resolve"}
	if len(manifest.Capabilities) > 32 || len(manifest.Events) > 64 || len(manifest.Commands) > 32 || len(manifest.Providers) > 32 || manifest.TickSeconds < 0 || manifest.TickSeconds > 86400 {
		return errors.New("plugin declaration limits exceeded")
	}
	for _, capability := range manifest.Capabilities {
		if !slices.Contains(allowed, capability) {
			return errors.New("unknown plugin capability")
		}
	}
	if len(manifest.Events) > 0 && !slices.Contains(manifest.Capabilities, "events") {
		return errors.New("plugin events require events capability")
	}
	if len(manifest.Commands) > 0 && !slices.Contains(manifest.Capabilities, "commands") {
		return errors.New("plugin commands require commands capability")
	}
	if len(manifest.Providers) > 0 && !slices.Contains(manifest.Capabilities, "render.provider") {
		return errors.New("plugin providers require render.provider capability")
	}
	if manifest.TickSeconds > 0 && !slices.Contains(manifest.Capabilities, "timers") {
		return errors.New("plugin timers require timers capability")
	}
	seen := map[string]bool{}
	for _, command := range manifest.Commands {
		if !packagefs.ValidID(command.Name) || seen[command.Name] || slices.Contains([]string{"help", "bind", "unbind", "bindings", "online", "profile", "inventory", "tps"}, command.Name) || len(command.Arguments) > 16 || command.CooldownSeconds < 1 || command.CooldownSeconds > 86400 {
			return errors.New("invalid plugin command")
		}
		seen[command.Name] = true
		for _, pattern := range command.Arguments {
			if len(pattern) > 512 {
				return errors.New("plugin argument pattern too long")
			}
			if _, err := regexp.Compile("^(?:" + pattern + ")$"); err != nil {
				return err
			}
		}
	}
	seen = map[string]bool{}
	for _, provider := range manifest.Providers {
		if !packagefs.ValidID(provider) || seen[provider] {
			return errors.New("invalid plugin provider")
		}
		seen[provider] = true
	}
	_, err = packagefs.Schema(manifest.ConfigSchema)
	return err
}

func (m *Manager) load(id string) (Plugin, error) {
	if !packagefs.ValidID(id) {
		return Plugin{}, fault.New("INVALID_INPUT", "插件标识无效")
	}
	var plugin Plugin
	err := packagefs.ReadJSON(filepath.Join(m.dir, id), "state.json", &plugin)
	if errors.Is(err, os.ErrNotExist) {
		return plugin, fault.New("NOT_FOUND", "插件不存在")
	}
	return plugin, err
}
func (m *Manager) save(plugin Plugin) error {
	plugin.Revision = securefs.ID()
	plugin.State = ""
	plugin.LastError = ""
	plugin.Dropped = 0
	data, err := json.Marshal(plugin)
	if err != nil {
		return err
	}
	return securefs.AtomicWrite(filepath.Join(m.dir, plugin.ID, "state.json"), data)
}

func (m *Manager) Import(ctx context.Context, reader io.Reader) (Plugin, error) {
	stage, err := packagefs.Extract(ctx, m.dir, reader)
	if err != nil {
		return Plugin{}, fault.New("INVALID_PACKAGE", err.Error())
	}
	defer stage.Discard()
	return m.install(stage)
}
func (m *Manager) ImportDirectory(ctx context.Context, dir string) (Plugin, error) {
	stage, err := packagefs.CopyDirectory(ctx, m.dir, dir)
	if err != nil {
		return Plugin{}, err
	}
	defer stage.Discard()
	return m.install(stage)
}
func (m *Manager) install(stage *packagefs.Stage) (Plugin, error) {
	var manifest sdk.Manifest
	if err := packagefs.ReadJSON(stage.Dir, "plugin.json", &manifest); err != nil {
		return Plugin{}, fault.New("INVALID_PACKAGE", "插件清单无效")
	}
	if err := validate(manifest); err != nil {
		return Plugin{}, fault.New("INVALID_PACKAGE", err.Error())
	}
	entry, err := packagefs.Entry(stage.Dir, manifest.Entry)
	if err != nil {
		return Plugin{}, fault.New("INVALID_PACKAGE", "插件入口不存在")
	}
	if _, err := securefs.ReadFile(entry, 2<<20); err != nil {
		return Plugin{}, fault.New("INVALID_PACKAGE", "插件入口过大或不可读取")
	}
	m.ops.Lock()
	defer m.ops.Unlock()
	if m.closed {
		return Plugin{}, fault.New("UNAVAILABLE", "插件宿主已停止")
	}
	plugin, err := m.load(manifest.ID)
	if fault.Is(err, "NOT_FOUND") {
		plugin = Plugin{ID: manifest.ID, Configuration: json.RawMessage(`{}`), Versions: []Version{}}
	} else if err != nil {
		return Plugin{}, err
	}
	for _, version := range plugin.Versions {
		if version.Revision == stage.Digest {
			return plugin, nil
		}
		if version.Manifest.Version == manifest.Version {
			return Plugin{}, fault.New("REVISION_CONFLICT", "同一插件版本含不同内容，请修改版本号")
		}
	}
	if len(plugin.Versions) >= 32 {
		return Plugin{}, fault.New("CAPACITY_EXCEEDED", "插件保留版本过多")
	}
	base := filepath.Join(m.dir, manifest.ID)
	if err := securefs.PrivateDir(base); err != nil {
		return Plugin{}, err
	}
	target := filepath.Join(base, stage.Digest)
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(stage.Dir, target); err != nil {
			return Plugin{}, err
		}
		stage.Dir = ""
	} else if err != nil {
		return Plugin{}, err
	} else {
		hash, err := packagefs.Hash(target)
		if err != nil || hash != stage.Digest {
			return Plugin{}, errors.New("plugin package digest mismatch")
		}
	}
	plugin.Versions = append(plugin.Versions, Version{Revision: stage.Digest, Manifest: manifest, ImportedAt: time.Now().UTC()})
	if plugin.ActiveRevision == "" {
		plugin.ActiveRevision = stage.Digest
	}
	if err := m.save(plugin); err != nil {
		return Plugin{}, err
	}
	return m.load(manifest.ID)
}

func (m *Manager) List() ([]Plugin, error) {
	m.ops.Lock()
	defer m.ops.Unlock()
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}
	plugins := []Plugin{}
	for _, entry := range entries {
		if !entry.IsDir() || !packagefs.ValidID(entry.Name()) {
			continue
		}
		plugin, err := m.load(entry.Name())
		if err != nil {
			return nil, err
		}
		plugin.State = "disabled"
		if worker := m.workers[plugin.ID]; worker != nil {
			plugin.State = "running"
			plugin.LastError, plugin.Dropped = worker.status()
			select {
			case <-worker.done:
				plugin.State = "failed"
			default:
			}
		} else if plugin.Enabled {
			plugin.State = "failed"
			plugin.LastError = "PLUGIN_NOT_RUNNING"
		}
		plugins = append(plugins, plugin)
	}
	slices.SortFunc(plugins, func(a, b Plugin) int { return strings.Compare(a.ID, b.ID) })
	return plugins, nil
}

func (m *Manager) Enable(ctx context.Context, id, version, revision string, grant sdk.Grant, configuration json.RawMessage) (Plugin, error) {
	m.ops.Lock()
	defer m.ops.Unlock()
	if m.closed {
		return Plugin{}, fault.New("UNAVAILABLE", "插件宿主已停止")
	}
	plugin, err := m.load(id)
	if err != nil {
		return plugin, err
	}
	if plugin.Revision != revision || revision == "" {
		return plugin, fault.New("REVISION_CONFLICT", "插件状态已变化")
	}
	if worker := m.workers[id]; worker != nil {
		select {
		case <-worker.done:
			m.mu.Lock()
			delete(m.workers, id)
			m.mu.Unlock()
		default:
			return plugin, fault.New("REVISION_CONFLICT", "请先停用插件再修改版本、配置或权限")
		}
	}
	index := slices.IndexFunc(plugin.Versions, func(v Version) bool { return v.Revision == version })
	if index < 0 {
		return plugin, fault.New("NOT_FOUND", "插件版本不存在")
	}
	manifest := plugin.Versions[index].Manifest
	if err := validate(manifest); err != nil {
		return plugin, fault.New("INVALID_PACKAGE", "插件清单无效")
	}
	if err := m.host.ValidateGrant(manifest, grant); err != nil {
		return plugin, err
	}
	for _, capability := range grant.Capabilities {
		if !slices.Contains(manifest.Capabilities, capability) {
			return plugin, fault.New("FORBIDDEN", "授予权限超出了插件声明")
		}
	}
	if len(m.workers) >= 32 {
		return plugin, fault.New("CAPACITY_EXCEEDED", "启用插件数量已达到上限")
	}
	if slices.Contains(grant.Capabilities, "commands") {
		for _, command := range manifest.Commands {
			for _, worker := range m.workers {
				if slices.Contains(worker.grant.Capabilities, "commands") {
					for _, existing := range worker.manifest.Commands {
						if existing.Name == command.Name {
							return plugin, fault.New("REVISION_CONFLICT", "命令名称与已启用插件冲突")
						}
					}
				}
			}
		}
	}
	if len(configuration) == 0 {
		configuration = json.RawMessage(`{}`)
	}
	schema, err := packagefs.Schema(manifest.ConfigSchema)
	if err != nil {
		return plugin, err
	}
	if err := packagefs.ValidateJSON(schema, configuration); err != nil {
		return plugin, fault.New("INVALID_INPUT", "插件配置不符合 schema")
	}
	root := filepath.Join(m.dir, id, version)
	hash, err := packagefs.Hash(root)
	if err != nil || hash != version {
		return plugin, fault.New("REVISION_CONFLICT", "插件文件已变化，请重新导入")
	}
	entry, err := packagefs.Entry(root, manifest.Entry)
	if err != nil {
		return plugin, err
	}
	source, err := securefs.ReadFile(entry, 2<<20)
	if err != nil {
		return plugin, err
	}
	dataDir := filepath.Join(m.dataDir, id)
	if err := securefs.PrivateDir(dataDir); err != nil {
		return plugin, err
	}
	worker := newWorker(m.ctx, m.host, manifest, grant, configuration, string(source), dataDir)
	go worker.run()
	select {
	case err := <-worker.ready:
		if err != nil {
			worker.stop()
			return plugin, err
		}
	case <-ctx.Done():
		worker.stop()
		return plugin, ctx.Err()
	case <-m.ctx.Done():
		worker.stop()
		return plugin, m.ctx.Err()
	}
	plugin.ActiveRevision = version
	plugin.Enabled = true
	plugin.Grant = grant
	plugin.Configuration = append(json.RawMessage(nil), configuration...)
	if err := m.save(plugin); err != nil {
		worker.stop()
		return plugin, err
	}
	m.mu.Lock()
	m.workers[id] = worker
	m.mu.Unlock()
	m.host.PluginChanged(id, "enabled")
	return m.load(id)
}

func (m *Manager) Disable(id, revision string) (Plugin, error) {
	m.ops.Lock()
	defer m.ops.Unlock()
	plugin, err := m.load(id)
	if err != nil {
		return plugin, err
	}
	if revision == "" || plugin.Revision != revision {
		return plugin, fault.New("REVISION_CONFLICT", "插件状态已变化")
	}
	plugin.Enabled = false
	if err := m.save(plugin); err != nil {
		return plugin, err
	}
	m.mu.Lock()
	worker := m.workers[id]
	delete(m.workers, id)
	m.mu.Unlock()
	if worker != nil {
		worker.stop()
	}
	m.host.PluginChanged(id, "disabled")
	return m.load(id)
}

func (m *Manager) Remove(id, revision string) error {
	m.ops.Lock()
	defer m.ops.Unlock()
	plugin, err := m.load(id)
	if err != nil {
		return err
	}
	if plugin.Revision != revision || revision == "" {
		return fault.New("REVISION_CONFLICT", "插件状态已变化")
	}
	if plugin.Enabled || m.workers[id] != nil {
		return fault.New("REVISION_CONFLICT", "请先停用插件")
	}
	dataDir := filepath.Join(m.dataDir, id)
	if err := securefs.PrivateDir(dataDir); err != nil {
		return err
	}
	if err := securefs.AtomicWrite(filepath.Join(dataDir, "last-configuration.json"), plugin.Configuration); err != nil {
		return err
	}
	return packagefs.Remove(m.dir, filepath.Join(m.dir, id))
}

func (m *Manager) Start() {
	plugins, err := m.List()
	if err != nil {
		m.host.PluginChanged("core", "PLUGIN_CATALOG_FAILED")
		return
	}
	for _, plugin := range plugins {
		if !plugin.Enabled {
			continue
		}
		ctx, cancel := context.WithTimeout(m.ctx, 15*time.Second)
		_, err := m.Enable(ctx, plugin.ID, plugin.ActiveRevision, plugin.Revision, plugin.Grant, plugin.Configuration)
		cancel()
		if err != nil {
			m.host.PluginChanged(plugin.ID, "PLUGIN_START_FAILED")
		}
	}
}

func (m *Manager) Emit(event sdk.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	for _, worker := range m.workers {
		worker.event(event)
	}
}
func (m *Manager) Command(name, route, network string) (string, sdk.Command, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, worker := range m.workers {
		if !slices.Contains(worker.grant.Capabilities, "commands") || !slices.Contains(worker.grant.Routes, route) || !slices.Contains(worker.grant.Networks, network) {
			continue
		}
		for _, command := range worker.manifest.Commands {
			if command.Name == name {
				return id, command, true
			}
		}
	}
	return "", sdk.Command{}, false
}
func (m *Manager) RunCommand(ctx context.Context, id string, data json.RawMessage) (json.RawMessage, error) {
	m.mu.Lock()
	worker := m.workers[id]
	m.mu.Unlock()
	if worker == nil {
		return nil, fault.New("PLUGIN_DISABLED", "插件已停用")
	}
	return worker.submit(ctx, "command", "", data)
}
func (m *Manager) Provider(ctx context.Context, id, name string, data json.RawMessage) (json.RawMessage, error) {
	m.mu.Lock()
	worker := m.workers[id]
	m.mu.Unlock()
	if worker == nil || !slices.Contains(worker.grant.Capabilities, "render.provider") || !slices.Contains(worker.manifest.Providers, name) {
		return nil, fault.New("NOT_FOUND", "插件数据提供器不可用")
	}
	return worker.submit(ctx, "provider", name, data)
}
func (m *Manager) Close() {
	m.ops.Lock()
	m.mu.Lock()
	m.closed = true
	m.cancel()
	workers := m.workers
	m.workers = map[string]*worker{}
	m.mu.Unlock()
	m.ops.Unlock()
	for _, worker := range workers {
		worker.stop()
	}
}

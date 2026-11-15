package rendering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/rendering/chromium"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/render"
)

type Options struct {
	BrowserPath    string
	BrowserVersion string
	Concurrency    int
	QueueSize      int
	Timeout        time.Duration
}
type Service struct {
	Catalog   *Catalog
	ctx       context.Context
	cancel    context.CancelFunc
	options   Options
	dir       string
	slots     chan struct{}
	queue     chan struct{}
	mu        sync.Mutex
	providers map[string]render.DataProvider
	wg        sync.WaitGroup
	closed    bool
}

func New(ctx context.Context, dir string, options Options) (*Service, error) {
	catalog, err := NewCatalog(filepath.Join(dir, "templates"))
	if err != nil {
		return nil, err
	}
	cache := filepath.Join(dir, "cache", "renders")
	if err := securefs.PrivateDir(cache); err != nil {
		return nil, err
	}
	if err := catalog.SeedBuiltins(ctx); err != nil {
		return nil, err
	}
	if options.Concurrency < 1 || options.Concurrency > 4 || options.QueueSize < 1 || options.QueueSize > 64 || options.Timeout < time.Second || options.Timeout > 2*time.Minute {
		return nil, errors.New("invalid render limits")
	}
	if options.BrowserPath != "" {
		info, err := os.Stat(options.BrowserPath)
		if err != nil || !info.Mode().IsRegular() || options.BrowserVersion == "" {
			return nil, errors.New("renderer requires an existing browser executable and exact browser_version")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	return &Service{Catalog: catalog, ctx: ctx, cancel: cancel, options: options, dir: cache, slots: make(chan struct{}, options.Concurrency), queue: make(chan struct{}, options.QueueSize+options.Concurrency), providers: map[string]render.DataProvider{}}, nil
}

func (s *Service) Enabled() bool { return s.options.BrowserPath != "" }
func (s *Service) RegisterProvider(id string, provider render.DataProvider) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !packagefs.ValidID(id) || provider == nil {
		return errors.New("invalid render provider")
	}
	if s.providers[id] != nil {
		return errors.New("render provider already registered")
	}
	s.providers[id] = provider
	return nil
}
func (s *Service) Provider(ctx context.Context, id string, request json.RawMessage) (json.RawMessage, error) {
	s.mu.Lock()
	provider := s.providers[id]
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, fault.New("UNAVAILABLE", "渲染服务已停止")
	}
	if provider == nil {
		return nil, fault.New("NOT_FOUND", "图片数据提供器不存在")
	}
	ctx, cancel := context.WithTimeout(ctx, s.options.Timeout)
	defer cancel()
	value, err := provider(ctx, append(json.RawMessage(nil), request...))
	if err != nil {
		return nil, err
	}
	if err := packagefs.ValidateJSON(nil, value); err != nil {
		return nil, err
	}
	return value, nil
}

func (s *Service) Render(ctx context.Context, request render.Request) (render.Result, error) {
	if !s.Enabled() {
		return render.Result{}, fault.New("RENDERER_UNAVAILABLE", "尚未配置 Chromium 路径和固定版本")
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return render.Result{}, fault.New("UNAVAILABLE", "渲染服务已停止")
	}
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	select {
	case s.queue <- struct{}{}:
		defer func() { <-s.queue }()
	default:
		return render.Result{}, fault.New("BUSY", "图片任务队列已满")
	}
	ctx, cancel := context.WithTimeout(ctx, s.options.Timeout)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return render.Result{}, ctx.Err()
	}
	dir, version, err := s.Catalog.Resolve(request.TemplateID, request.Revision)
	if err != nil {
		return render.Result{}, err
	}
	manifest := version.Manifest
	if !request.IgnoreDefaults {
		defaults, err := s.Catalog.Defaults(request.TemplateID)
		if err != nil {
			return render.Result{}, err
		}
		if len(request.Theme) == 0 {
			request.Theme = defaults.Theme
		}
		if request.CSS == "" {
			request.CSS = defaults.CSS
		}
		if request.Locale == "" {
			request.Locale = defaults.Locale
		}
	}
	schema, err := packagefs.Schema(manifest.DataSchema)
	if err != nil {
		return render.Result{}, err
	}
	if err := packagefs.ValidateJSON(schema, request.Data); err != nil {
		return render.Result{}, fault.New("INVALID_INPUT", "图片数据不符合模板 schema")
	}
	if len(request.Theme) > 0 {
		if err := packagefs.ValidateJSON(nil, request.Theme); err != nil {
			return render.Result{}, fault.New("INVALID_INPUT", "主题配置不是有效 JSON")
		}
	}
	if len(request.CSS) > 64<<10 || len(request.Locale) > 32 {
		return render.Result{}, fault.New("INVALID_INPUT", "图片样式或语言参数过长")
	}
	if request.Width == 0 {
		request.Width = manifest.Width
	}
	if request.Height == 0 {
		request.Height = manifest.Height
	}
	if request.Scale == 0 {
		request.Scale = manifest.Scale
	}
	if request.Format == "" {
		request.Format = manifest.Format
	}
	if request.Locale == "" {
		request.Locale = "zh-CN"
	}
	if math.IsNaN(request.Scale) || math.IsInf(request.Scale, 0) {
		return render.Result{}, fault.New("INVALID_INPUT", "图片倍率无效")
	}
	if err := Dimensions(request.Width, request.Height, request.Scale, request.Format); err != nil {
		return render.Result{}, fault.New("INVALID_INPUT", err.Error())
	}
	output, err := (chromium.Browser{Executable: s.options.BrowserPath, Version: s.options.BrowserVersion}).Capture(ctx, dir, manifest, request)
	if err != nil {
		return render.Result{}, fault.New("RENDER_FAILED", "浏览器渲染未完成，请检查版本、模板 ready 调用和任务时限")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.cleanupLocked(); err != nil {
		return render.Result{}, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return render.Result{}, err
	}
	var count int
	var size int64
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".image") {
			info, err := entry.Info()
			if err != nil {
				return render.Result{}, err
			}
			count++
			size += info.Size()
		}
	}
	if count >= 128 || size+int64(len(output)) > 256<<20 {
		return render.Result{}, fault.New("CAPACITY_EXCEEDED", "图片缓存数量已满")
	}
	id := securefs.ID()
	hash := sha256.Sum256(output)
	now := time.Now().UTC()
	mime := "image/png"
	if request.Format == "jpeg" {
		mime = "image/jpeg"
	}
	result := render.Result{ResourceID: id, MIME: mime, Width: int(float64(request.Width) * request.Scale), Height: int(float64(request.Height) * request.Scale), SHA256: hex.EncodeToString(hash[:]), Size: int64(len(output)), CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}
	if err := securefs.AtomicWrite(filepath.Join(s.dir, id+".image"), output); err != nil {
		return render.Result{}, err
	}
	metadata, _ := json.Marshal(result)
	if err := securefs.AtomicWrite(filepath.Join(s.dir, id+".json"), metadata); err != nil {
		_ = os.Remove(filepath.Join(s.dir, id+".image"))
		return render.Result{}, err
	}
	return result, nil
}

func (s *Service) Resource(id string) (render.Result, []byte, error) {
	if len(id) != 32 || !packagefs.ValidID(id) {
		return render.Result{}, nil, fault.New("NOT_FOUND", "图片资源不存在")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var metadata render.Result
	if err := packagefs.ReadJSON(s.dir, id+".json", &metadata); err != nil {
		return metadata, nil, fault.New("NOT_FOUND", "图片资源不存在")
	}
	if time.Now().After(metadata.ExpiresAt) {
		return metadata, nil, fault.New("NOT_FOUND", "图片资源已过期")
	}
	data, err := securefs.ReadFile(filepath.Join(s.dir, id+".image"), packagefs.MaxFileBytes)
	if err != nil {
		return metadata, nil, err
	}
	hash := sha256.Sum256(data)
	if hex.EncodeToString(hash[:]) != metadata.SHA256 {
		return metadata, nil, errors.New("render resource digest mismatch")
	}
	return metadata, data, nil
}

func (s *Service) cleanupLocked() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var result render.Result
		err := packagefs.ReadJSON(s.dir, entry.Name(), &result)
		if err != nil || now.After(result.ExpiresAt) {
			id := strings.TrimSuffix(entry.Name(), ".json")
			_ = os.Remove(filepath.Join(s.dir, id+".image"))
			_ = os.Remove(filepath.Join(s.dir, entry.Name()))
		}
	}
	// An interrupted metadata write must not reserve cache capacity forever.
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".image") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".image")
		if _, err := os.Stat(filepath.Join(s.dir, id+".json")); errors.Is(err, os.ErrNotExist) {
			if err := os.Remove(filepath.Join(s.dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Close() { s.mu.Lock(); s.closed = true; s.cancel(); s.mu.Unlock(); s.wg.Wait() }

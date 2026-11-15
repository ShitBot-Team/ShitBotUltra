package chromium

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/render"
	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type Browser struct {
	Executable string
	Version    string
}

func (b Browser) Capture(ctx context.Context, dir string, manifest render.Manifest, request render.Request) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	prefix := "/" + securefs.ID() + "/"
	origin := "http://" + listener.Addr().String()
	assets := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, prefix)
		if !packagefs.SafePath(name) {
			http.NotFound(w, r)
			return
		}
		file, err := root.Open(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox allow-scripts allow-same-origin; default-src 'self' data: blob:; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'none'; frame-src 'none'; worker-src 'none'; object-src 'none'; base-uri 'none'; form-action 'none'")
		http.ServeContent(w, r, name, info.ModTime(), file)
	})}
	defer assets.Close()
	go func() { _ = assets.Serve(listener) }()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(b.Executable), chromedp.Flag("allow-file-access-from-files", false), chromedp.Flag("no-sandbox", false), chromedp.Flag("disable-extensions", true), chromedp.ModifyCmdFunc(hideWindow))
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()
	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx)
	defer chromeCancel()
	// This context owns exactly one process and page. Cancellation releases both;
	// a failed browser is replaced naturally by the next job.
	chromedp.ListenTarget(chromeCtx, func(event any) {
		paused, ok := event.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		go func() {
			target := chromedp.FromContext(chromeCtx).Target
			if target == nil {
				return
			}
			execution := cdp.WithExecutor(chromeCtx, target)
			if strings.HasPrefix(paused.Request.URL, origin+prefix) || strings.HasPrefix(paused.Request.URL, "data:") {
				_ = fetch.ContinueRequest(paused.RequestID).Do(execution)
			} else {
				_ = fetch.FailRequest(paused.RequestID, network.ErrorReasonBlockedByClient).Do(execution)
			}
		}()
	})
	if len(request.Theme) == 0 {
		request.Theme = json.RawMessage(`{}`)
	}
	contextData, err := json.Marshal(map[string]any{"data": request.Data, "theme": request.Theme, "locale": request.Locale, "template": manifest.ID, "version": manifest.Version})
	if err != nil {
		return nil, err
	}
	initScript := `globalThis.__sbuReady=false;Object.defineProperty(globalThis,"ultra",{writable:false,value:Object.freeze({context:` + string(contextData) + `,ready:()=>{globalThis.__sbuReady=true;}})});`
	css, _ := json.Marshal(request.CSS)
	entry := origin + (&url.URL{Path: prefix + manifest.Entry}).String()
	var output []byte
	err = chromedp.Run(chromeCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, product, _, _, _, err := browser.GetVersion().Do(ctx)
			if err != nil {
				return err
			}
			if product != b.Version {
				return errors.New("Chromium version does not match configured browser_version")
			}
			return nil
		}),
		fetch.Enable().WithPatterns([]*fetch.RequestPattern{{URLPattern: "*"}}),
		emulation.SetDeviceMetricsOverride(int64(request.Width), int64(request.Height), request.Scale, false),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(initScript).Do(ctx)
			return err
		}),
		chromedp.Navigate(entry),
		chromedp.Evaluate(`(()=>{const style=document.createElement('style');style.textContent=`+string(css)+`;document.head.appendChild(style)})()`, nil),
		chromedp.Poll(`globalThis.__sbuReady===true`, nil, chromedp.WithPollingInterval(50*time.Millisecond)),
		chromedp.Evaluate(`(async()=>{await document.fonts.ready;await Promise.all(Array.from(document.images).map(image=>image.complete?Promise.resolve():new Promise(resolve=>{image.onload=resolve;image.onerror=resolve})));await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))})()`, nil, func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return p.WithAwaitPromise(true) }),
		chromedp.ActionFunc(func(ctx context.Context) error {
			format := page.CaptureScreenshotFormatPng
			if request.Format == "jpeg" {
				format = page.CaptureScreenshotFormatJpeg
			}
			capture := page.CaptureScreenshot().WithFormat(format).WithCaptureBeyondViewport(false).WithClip(&page.Viewport{X: 0, Y: 0, Width: float64(request.Width), Height: float64(request.Height), Scale: 1})
			if request.Format == "jpeg" {
				capture = capture.WithQuality(90)
			}
			var err error
			output, err = capture.Do(ctx)
			return err
		}),
	)
	if err != nil {
		return nil, err
	}
	if len(output) == 0 || len(output) > packagefs.MaxFileBytes {
		return nil, errors.New("render output exceeds size limit")
	}
	return output, nil
}

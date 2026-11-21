package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/bootstrap"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

type Handler struct {
	engine *bootstrap.Engine
	status func() any
	hosts  []string
	mux    *http.ServeMux
}

func New(engine *bootstrap.Engine, status func() any, listenAddress string) *Handler {
	h := &Handler{engine: engine, status: status, mux: http.NewServeMux(), hosts: append([]string{}, engine.Config.HTTP.AllowedHosts...)}
	host, port, _ := net.SplitHostPort(listenAddress)
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		h.hosts = append(h.hosts, net.JoinHostPort(host, port), net.JoinHostPort("localhost", port))
	}
	h.mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, r *http.Request) { respond(w, http.StatusOK, h.status()) })
	h.mux.HandleFunc("GET /api/v1/dashboard", h.dashboard)
	h.mux.HandleFunc("GET /api/v1/connections", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, map[string]any{"nodes": engine.Hub.Status(), "adapters": engine.AdapterStatus()})
	})
	h.mux.HandleFunc("GET /api/v1/events", h.stream)
	h.mux.HandleFunc("GET /api/v1/config", h.getConfig)
	h.mux.HandleFunc("PUT /api/v1/config", h.saveConfig)
	h.mux.HandleFunc("POST /api/v1/config/validate", h.validateConfig)
	h.mux.HandleFunc("POST /api/v1/secrets", h.createSecret)
	h.mux.HandleFunc("GET /api/v1/bindings", h.listBindings)
	h.mux.HandleFunc("POST /api/v1/bindings", h.addBinding)
	h.mux.HandleFunc("DELETE /api/v1/bindings/{network}/{name}", h.removeBinding)
	h.mux.HandleFunc("GET /api/v1/players", h.players)
	h.mux.HandleFunc("GET /api/v1/inventory", h.inventory)
	h.mux.HandleFunc("POST /api/v1/nodes/{id}/actions", h.action)
	h.mux.HandleFunc("GET /api/v1/imports/sources", h.sources)
	h.mux.HandleFunc("POST /api/v1/imports/sources", h.upload)
	h.mux.HandleFunc("DELETE /api/v1/imports/sources/{id}", h.deleteSource)
	h.mux.HandleFunc("POST /api/v1/imports/previews", h.preview)
	h.mux.HandleFunc("GET /api/v1/imports/{id}", h.report)
	h.mux.HandleFunc("POST /api/v1/imports/{id}/commit", h.commit)
	h.mux.HandleFunc("POST /api/v1/imports/{id}/restore", h.restore)
	h.registerRendering()
	h.registerPlugins()
	h.registerUpdates()
	h.registerWebUI()
	h.registerSettings()
	return h
}

func (h *Handler) checkOrigin(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	host := strings.ToLower(r.Host)
	allowed := false
	for _, entry := range h.hosts {
		if strings.EqualFold(entry, host) {
			allowed = true
			break
		}
	}
	if !allowed {
		writeError(w, fault.New("FORBIDDEN", "Host 不在允许列表中"))
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if err != nil || u.User != nil || u.Host != r.Host || u.Scheme != scheme || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			writeError(w, fault.New("FORBIDDEN", "跨源管理请求被拒绝"))
			return false
		}
	}
	return true
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.checkOrigin(w, r) {
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(auth, "Bearer ")), []byte(h.engine.AdminToken)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		respond(w, http.StatusUnauthorized, map[string]any{"error": map[string]string{"code": "UNAUTHORIZED", "message": "需要管理凭据"}})
		return
	}
	if r.URL.Path != "/api/v1/events" {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		r = r.WithContext(ctx)
	}
	h.mux.ServeHTTP(w, r)
}

func respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	e := fault.Public(err)
	status := http.StatusInternalServerError
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		e = &fault.Error{Code: "INVALID_INPUT", Message: "请求体超过允许大小"}
	}
	switch e.Code {
	case "INVALID_INPUT", "INVALID_PLAYER", "INVALID_CONFIG", "INVALID_SOURCE", "INVALID_PACKAGE", "INVALID_CODE":
		status = http.StatusBadRequest
	case "FORBIDDEN":
		status = http.StatusForbidden
	case "NOT_FOUND":
		status = http.StatusNotFound
	case "REVISION_CONFLICT", "ALREADY_BOUND", "IDENTITY_CONFLICT", "BINDING_LIMIT", "IMPORT_CONFLICT", "PREVIEW_EXPIRED":
		status = http.StatusConflict
	case "RATE_LIMITED", "BUSY", "CAPACITY_EXCEEDED":
		status = http.StatusTooManyRequests
	case "NODE_OFFLINE", "UNAVAILABLE", "RENDERER_UNAVAILABLE", "PLUGIN_DISABLED", "UPDATE_UNAVAILABLE":
		status = http.StatusServiceUnavailable
	case "UNSUPPORTED_CAPABILITY":
		status = http.StatusUnprocessableEntity
	case "RESULT_UNKNOWN", "DEADLINE_EXCEEDED":
		status = http.StatusGatewayTimeout
	case "RENDER_FAILED", "PLUGIN_FAILED", "INVALID_RESPONSE":
		status = http.StatusBadGateway
	case "CANCELED":
		status = http.StatusRequestTimeout
	}
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
		e = &fault.Error{Code: "DEADLINE_EXCEEDED", Message: "操作超时"}
	}
	respond(w, status, map[string]any{"error": e})
}

func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	if content := r.Header.Get("Content-Type"); !strings.HasPrefix(content, "application/json") {
		writeError(w, fault.New("INVALID_INPUT", "请求体必须为 application/json"))
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		writeError(w, fault.New("INVALID_INPUT", "JSON 请求体无效、过大或含未知字段"))
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		writeError(w, fault.New("INVALID_INPUT", "请求体必须只有一个 JSON 对象"))
		return false
	}
	return true
}

func (h *Handler) network(id string) (string, error) {
	for _, node := range h.engine.Config.Nodes {
		if node.NetworkID == id {
			return node.IdentityMode, nil
		}
	}
	return "", fault.New("INVALID_INPUT", "未知目标网络")
}
func page(r *http.Request) (int, int, error) {
	limit := 100
	offset := 0
	var err error
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, fault.New("INVALID_INPUT", "limit 无效")
		}
	}
	if value := r.URL.Query().Get("offset"); value != "" {
		offset, err = strconv.Atoi(value)
		if err != nil {
			return 0, 0, fault.New("INVALID_INPUT", "offset 无效")
		}
	}
	if limit < 1 || limit > 1000 || offset < 0 {
		return 0, 0, fault.New("INVALID_INPUT", "分页参数无效")
	}
	return limit, offset, nil
}

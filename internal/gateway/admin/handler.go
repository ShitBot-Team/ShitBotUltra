package admin

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/binding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/bootstrap"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/events"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/importer/shitbot"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
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

func (h *Handler) getConfig(w http.ResponseWriter, r *http.Request) {
	doc, err := h.engine.ConfigStore.Read()
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(doc.Revision))
	respond(w, http.StatusOK, doc)
}
func (h *Handler) saveConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	revision, err := strconv.Unquote(r.Header.Get("If-Match"))
	if err != nil {
		respond(w, http.StatusPreconditionRequired, map[string]any{"error": map[string]string{"code": "REVISION_REQUIRED", "message": "请在 If-Match 中提供读取时的 ETag"}})
		return
	}
	doc, err := h.engine.ConfigStore.Save(body.Text, revision)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(doc.Revision))
	h.engine.Events.Publish("config.saved", map[string]bool{"restart_required": true})
	respond(w, http.StatusOK, map[string]any{"document": doc, "restart_required": true})
}
func (h *Handler) validateConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	if _, err := config.Decode([]byte(body.Text)); err != nil {
		writeError(w, fault.New("INVALID_CONFIG", err.Error()))
		return
	}
	respond(w, http.StatusOK, map[string]bool{"valid": true})
}

func (h *Handler) createSecret(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Value string `json:"value,omitempty"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`).MatchString(body.Name) {
		writeError(w, fault.New("INVALID_INPUT", "凭据文件名无效"))
		return
	}
	value := body.Value
	if value == "" {
		value = securefs.ID() + securefs.ID()
	}
	if len(value) < 32 || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		writeError(w, fault.New("INVALID_INPUT", "凭据长度须为 32 至 4096 字符"))
		return
	}
	path := filepath.Join(h.engine.Dir, "secrets", body.Name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		writeError(w, fault.New("REVISION_CONFLICT", "凭据已存在，请使用新文件名以免覆盖活动凭据"))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	_, err = f.WriteString(value + "\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		writeError(w, err)
		return
	}
	respond(w, http.StatusCreated, map[string]string{"reference": "file:" + body.Name, "value": value})
}

func (h *Handler) listBindings(w http.ResponseWriter, r *http.Request) {
	network := r.URL.Query().Get("network_id")
	if _, err := h.network(network); err != nil {
		writeError(w, err)
		return
	}
	limit, offset, err := page(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.engine.Bindings.List(r.Context(), network, nil, limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) addBinding(w http.ResponseWriter, r *http.Request) {
	var record binding.Record
	if !decode(w, r, &record) {
		return
	}
	mode, err := h.network(record.NetworkID)
	if err != nil {
		writeError(w, err)
		return
	}
	if record.Player.IdentityMode != mode {
		writeError(w, fault.New("IDENTITY_CONFLICT", "玩家身份模式与网络配置不匹配"))
		return
	}
	record.CreatedAt = time.Now().UTC()
	record.UpdatedAt = record.CreatedAt
	if err := h.engine.Bindings.Add(r.Context(), record); err != nil {
		writeError(w, err)
		return
	}
	h.engine.Events.Publish("bindings.changed", map[string]string{"network_id": record.NetworkID})
	respond(w, http.StatusCreated, record)
}
func (h *Handler) removeBinding(w http.ResponseWriter, r *http.Request) {
	network := r.PathValue("network")
	if _, err := h.network(network); err != nil {
		writeError(w, err)
		return
	}
	record, err := h.engine.Bindings.Remove(r.Context(), network, r.PathValue("name"), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	h.engine.AfterUnbind(r.Context(), record)
	respond(w, http.StatusOK, map[string]bool{"removed": true})
}
func (h *Handler) players(w http.ResponseWriter, r *http.Request) {
	network := r.URL.Query().Get("network_id")
	if _, err := h.network(network); err != nil {
		writeError(w, err)
		return
	}
	limit, offset, err := page(r)
	if err != nil {
		writeError(w, err)
		return
	}
	items, err := h.engine.Players.List(r.Context(), network, r.URL.Query().Get("online") == "true", limit, offset)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) inventory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if _, err := h.network(q.Get("network_id")); err != nil {
		writeError(w, err)
		return
	}
	snapshot, err := h.engine.Inventory.Get(r.Context(), q.Get("network_id"), q.Get("server_id"), q.Get("player_name"), nil)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, snapshot)
}
func (h *Handler) action(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !slices.Contains([]string{"chat.send", "player.kick", "command.execute", "inventory.read", "tps.read", "permission.check", "placeholder.resolve"}, body.Type) || len(body.Payload) == 0 {
		writeError(w, fault.New("INVALID_INPUT", "动作类型或参数无效"))
		return
	}
	value, err := h.engine.Hub.Request(r.Context(), r.PathValue("id"), body.Type, body.Payload)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"result": value})
}

func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.Imports.Sources()
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"items": items})
}
func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, shitbot.MaxSourceBytes)
	value, err := h.engine.Imports.Upload(r.Context(), r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusCreated, value)
}
func (h *Handler) deleteSource(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.Imports.DeleteSource(r.PathValue("id")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	var request shitbot.Request
	if !decode(w, r, &request) {
		return
	}
	report, err := h.engine.Imports.Preview(r.Context(), request)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusCreated, report)
}
func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	report, err := h.engine.Imports.Report(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, report)
}
func (h *Handler) commit(w http.ResponseWriter, r *http.Request) {
	report, err := h.engine.Imports.Commit(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	h.engine.Events.Publish("import.completed", map[string]any{"id": report.ID, "added": report.Added})
	respond(w, http.StatusOK, report)
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	report, err := h.engine.Imports.Restore(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	h.engine.Events.Publish("import.restored", map[string]string{"id": report.ID})
	respond(w, http.StatusOK, report)
}

func (h *Handler) stream(w http.ResponseWriter, r *http.Request) {
	after := uint64(0)
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		value, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			writeError(w, fault.New("INVALID_INPUT", "Last-Event-ID 无效"))
			return
		}
		after = value
	}
	backlog, ch, cancel, ok := h.engine.Events.Subscribe(after)
	if !ok {
		writeError(w, fault.New("BUSY", "事件订阅数已达到上限"))
		return
	}
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(event events.Event) error {
		_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		data, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.ID, event.Type, data); err != nil {
			return err
		}
		return controller.Flush()
	}
	for _, event := range backlog {
		if send(event) != nil {
			return
		}
	}
	_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(w, ": connected\n\n"); err != nil {
		return
	}
	if controller.Flush() != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			if send(event) != nil {
				return
			}
		case <-ticker.C:
			_ = controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if controller.Flush() != nil {
				return
			}
		}
	}
}

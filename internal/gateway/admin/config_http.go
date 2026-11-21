package admin

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
)

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
	settings, err := h.settingsDocument(doc)
	if err != nil {
		writeError(w, err)
		return
	}
	h.engine.Events.Publish("config.saved", map[string]bool{"restart_required": settings.RestartRequired})
	respond(w, http.StatusOK, map[string]any{"document": doc, "restart_required": settings.RestartRequired})
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
	// Match the normalization used when the credential is read on startup, so
	// the value shown to the administrator is the value the runtime will use.
	value := strings.TrimSpace(body.Value)
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

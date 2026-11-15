package admin

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ShitBot-Team/ShitBotUltra/internal/extension"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	sdk "github.com/ShitBot-Team/ShitBotUltra/sdk/extension"
)

func (h *Handler) registerPlugins() {
	h.mux.HandleFunc("GET /api/v1/plugins", h.plugins)
	h.mux.HandleFunc("POST /api/v1/plugins", h.importPlugin)
	h.mux.HandleFunc("GET /api/v1/plugins/{id}", h.plugin)
	h.mux.HandleFunc("POST /api/v1/plugins/{id}/enable", h.enablePlugin)
	h.mux.HandleFunc("POST /api/v1/plugins/{id}/disable", h.disablePlugin)
	h.mux.HandleFunc("DELETE /api/v1/plugins/{id}", h.removePlugin)
	h.mux.HandleFunc("POST /api/v1/plugins/{id}/providers/{name}", h.pluginProvider)
}

func pluginResponse(w http.ResponseWriter, status int, value extension.Plugin, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(value.Revision))
	respond(w, status, value)
}

func (h *Handler) plugins(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.Plugins.List()
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) plugin(w http.ResponseWriter, r *http.Request) {
	value, err := h.engine.Plugins.Get(r.PathValue("id"))
	pluginResponse(w, http.StatusOK, value, err)
}

func (h *Handler) importPlugin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, packagefs.MaxArchiveBytes)
	value, err := h.engine.Plugins.Import(r.Context(), r.Body)
	pluginResponse(w, http.StatusCreated, value, err)
}

func (h *Handler) enablePlugin(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	var body struct {
		VersionRevision string          `json:"version_revision"`
		Grant           sdk.Grant       `json:"grant"`
		Configuration   json.RawMessage `json:"configuration"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := h.engine.Plugins.Enable(r.Context(), r.PathValue("id"), body.VersionRevision, revision, body.Grant, body.Configuration)
	if err == nil {
		value, err = h.engine.Plugins.Get(value.ID)
	}
	pluginResponse(w, http.StatusOK, value, err)
}

func (h *Handler) disablePlugin(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	value, err := h.engine.Plugins.Disable(r.PathValue("id"), revision)
	if err == nil {
		value, err = h.engine.Plugins.Get(value.ID)
	}
	pluginResponse(w, http.StatusOK, value, err)
}

func (h *Handler) removePlugin(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	if err := h.engine.Plugins.Remove(r.PathValue("id"), revision); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) pluginProvider(w http.ResponseWriter, r *http.Request) {
	var body json.RawMessage
	if !decode(w, r, &body) {
		return
	}
	if err := packagefs.ValidateJSON(nil, body); err != nil {
		writeError(w, err)
		return
	}
	value, err := h.engine.Plugins.Provider(r.Context(), r.PathValue("id"), r.PathValue("name"), body)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, value)
}

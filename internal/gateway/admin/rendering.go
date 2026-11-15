package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/rendering"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/render"
)

func (h *Handler) registerRendering() {
	h.mux.HandleFunc("GET /api/v1/rendering", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, map[string]any{"enabled": h.engine.Renderer.Enabled(), "api_version": render.APIVersion,
			"providers": []string{"core.online", "core.profile", "core.inventory", "core.performance"}})
	})
	h.mux.HandleFunc("GET /api/v1/templates", h.templates)
	h.mux.HandleFunc("POST /api/v1/templates", h.importTemplate)
	h.mux.HandleFunc("GET /api/v1/templates/{id}", h.template)
	h.mux.HandleFunc("GET /api/v1/templates/{id}/files", h.templateFiles)
	h.mux.HandleFunc("GET /api/v1/templates/{id}/file", h.templateFile)
	h.mux.HandleFunc("PUT /api/v1/templates/{id}/draft", h.editTemplate)
	h.mux.HandleFunc("PUT /api/v1/templates/{id}/defaults", h.templateDefaults)
	h.mux.HandleFunc("POST /api/v1/templates/{id}/publish", h.publishTemplate)
	h.mux.HandleFunc("POST /api/v1/templates/{id}/restore", h.restoreTemplate)
	h.mux.HandleFunc("POST /api/v1/rendering/providers/{id}", h.renderData)
	h.mux.HandleFunc("POST /api/v1/renders", h.render)
	h.mux.HandleFunc("GET /api/v1/renders/resources/{id}", h.renderResource)
}

func requiredRevision(w http.ResponseWriter, r *http.Request) (string, bool) {
	revision, err := strconv.Unquote(r.Header.Get("If-Match"))
	if err != nil || revision == "" {
		respond(w, http.StatusPreconditionRequired, map[string]any{"error": map[string]string{
			"code": "REVISION_REQUIRED", "message": "请在 If-Match 中提供读取时的 ETag"}})
		return "", false
	}
	return revision, true
}

func templateResponse(w http.ResponseWriter, status int, value rendering.Template, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(value.Revision))
	respond(w, status, value)
}

func (h *Handler) templates(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.Renderer.Catalog.List()
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) template(w http.ResponseWriter, r *http.Request) {
	value, err := h.engine.Renderer.Catalog.Get(r.PathValue("id"))
	templateResponse(w, http.StatusOK, value, err)
}

func (h *Handler) importTemplate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, packagefs.MaxArchiveBytes)
	value, err := h.engine.Renderer.Catalog.Import(r.Context(), r.Body)
	if err == nil {
		h.engine.Events.Publish("template.imported", map[string]string{"id": value.ID})
	}
	templateResponse(w, http.StatusCreated, value, err)
}

func (h *Handler) templateFiles(w http.ResponseWriter, r *http.Request) {
	items, err := h.engine.Renderer.Catalog.Files(r.PathValue("id"), r.URL.Query().Get("revision"))
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) templateFile(w http.ResponseWriter, r *http.Request) {
	text, err := h.engine.Renderer.Catalog.File(r.PathValue("id"), r.URL.Query().Get("revision"), r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, err)
		return
	}
	// HTML and JS are returned as JSON text, never executed on the admin origin.
	respond(w, http.StatusOK, map[string]string{"text": text})
}

func (h *Handler) editTemplate(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	var body struct {
		BaseRevision string             `json:"base_revision"`
		Files        map[string]*string `json:"files"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := h.engine.Renderer.Catalog.Edit(r.Context(), r.PathValue("id"), rendering.DraftRequest{
		Revision: revision, BaseRevision: body.BaseRevision, Files: body.Files})
	templateResponse(w, http.StatusOK, value, err)
}

func (h *Handler) templateDefaults(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	var body render.Style
	if !decode(w, r, &body) {
		return
	}
	value, err := h.engine.Renderer.Catalog.Configure(r.PathValue("id"), revision, body)
	templateResponse(w, http.StatusOK, value, err)
}

func (h *Handler) publishTemplate(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	var body struct {
		VersionRevision string `json:"version_revision"`
	}
	if !decode(w, r, &body) {
		return
	}
	value, err := h.engine.Renderer.Catalog.Publish(r.PathValue("id"), body.VersionRevision, revision)
	if err == nil {
		h.engine.Events.Publish("template.published", map[string]string{"id": value.ID})
	}
	templateResponse(w, http.StatusOK, value, err)
}

func (h *Handler) restoreTemplate(w http.ResponseWriter, r *http.Request) {
	revision, ok := requiredRevision(w, r)
	if !ok {
		return
	}
	value, err := h.engine.Renderer.Catalog.RestoreBuiltin(r.PathValue("id"), revision)
	templateResponse(w, http.StatusOK, value, err)
}

func (h *Handler) renderData(w http.ResponseWriter, r *http.Request) {
	var body json.RawMessage
	if !decode(w, r, &body) {
		return
	}
	value, err := h.engine.Renderer.Provider(r.Context(), r.PathValue("id"), body)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, value)
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request) {
	var body render.Request
	if !decode(w, r, &body) {
		return
	}
	value, err := h.engine.Renderer.Render(r.Context(), body)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusCreated, value)
}

func (h *Handler) renderResource(w http.ResponseWriter, r *http.Request) {
	metadata, data, err := h.engine.Renderer.Resource(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	if metadata.MIME != "image/png" && metadata.MIME != "image/jpeg" {
		writeError(w, fault.New("INVALID_RESPONSE", "图片资源类型无效"))
		return
	}
	w.Header().Set("Content-Type", metadata.MIME)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(w, r, "render", metadata.CreatedAt, bytes.NewReader(data))
}

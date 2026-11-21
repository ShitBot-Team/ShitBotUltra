package admin

import (
	"net/http"

	"github.com/ShitBot-Team/ShitBotUltra/internal/importer/shitbot"
)

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

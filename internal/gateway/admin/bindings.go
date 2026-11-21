package admin

import (
	"net/http"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/binding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

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

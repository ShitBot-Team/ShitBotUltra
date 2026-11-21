package admin

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

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

package admin

import (
	"net/http"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/update"
)

func (h *Handler) registerUpdates() {
	h.mux.HandleFunc("GET /api/v1/updates", func(w http.ResponseWriter, r *http.Request) {
		if h.engine.Updates == nil {
			respond(w, http.StatusOK, update.State{Phase: "disabled"})
			return
		}
		respond(w, http.StatusOK, h.engine.Updates.Status())
	})
	h.mux.HandleFunc("POST /api/v1/updates/check", h.checkUpdate)
	h.mux.HandleFunc("POST /api/v1/updates/install", h.installUpdate)
}

func (h *Handler) checkUpdate(w http.ResponseWriter, r *http.Request) {
	if h.engine.Updates == nil {
		writeError(w, fault.New("UNSUPPORTED_CAPABILITY", "宿主没有注册更新功能"))
		return
	}
	state, err := h.engine.Updates.Check(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusOK, state)
}

func (h *Handler) installUpdate(w http.ResponseWriter, r *http.Request) {
	if h.engine.Updates == nil {
		writeError(w, fault.New("UNSUPPORTED_CAPABILITY", "宿主没有注册更新功能"))
		return
	}
	var body struct {
		Version string `json:"version"`
		SHA256  string `json:"sha256"`
	}
	if !decode(w, r, &body) {
		return
	}
	state, err := h.engine.Updates.Install(body.Version, body.SHA256)
	if err != nil {
		writeError(w, err)
		return
	}
	respond(w, http.StatusAccepted, state)
}

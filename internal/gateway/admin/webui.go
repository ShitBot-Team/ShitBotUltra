package admin

import (
	"net/http"
	"slices"
	"strings"
)

// Pages applies the same Host/Origin policy as the management API. Only the
// login shell is public; every operation continues to require a Bearer token.
func (h *Handler) Pages() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.checkOrigin(w, r) {
			h.engine.WebUI.ServeHTTP(w, r)
		}
	})
}

func (h *Handler) registerWebUI() {
	h.mux.HandleFunc("GET /api/v1/webui", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, h.engine.WebUI.Status())
	})
	h.mux.HandleFunc("PUT /api/v1/webui", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Enabled == nil {
			respond(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"code": "INVALID_INPUT", "message": "需要 enabled 布尔字段"}})
			return
		}
		h.engine.WebUI.SetEnabled(*body.Enabled)
		h.engine.Events.Publish("webui.changed", h.engine.WebUI.Status())
		respond(w, http.StatusOK, h.engine.WebUI.Status())
	})
	h.mux.HandleFunc("GET /api/v1/networks", h.networks)
}

func (h *Handler) networks(w http.ResponseWriter, r *http.Request) {
	type node struct {
		ID       string `json:"id"`
		ServerID string `json:"server_id"`
	}
	type network struct {
		ID           string   `json:"id"`
		IdentityMode string   `json:"identity_mode"`
		Nodes        []node   `json:"nodes"`
		Routes       []string `json:"routes"`
	}
	items := []network{}
	for _, n := range h.engine.Config.Nodes {
		i := slices.IndexFunc(items, func(v network) bool { return v.ID == n.NetworkID })
		if i < 0 {
			items = append(items, network{ID: n.NetworkID, IdentityMode: n.IdentityMode, Nodes: []node{}, Routes: []string{}})
			i = len(items) - 1
		}
		items[i].Nodes = append(items[i].Nodes, node{ID: n.ID, ServerID: n.ServerID})
	}
	for _, route := range h.engine.Config.Routes {
		n, _ := h.engine.Config.Node(route.NodeIDs[0])
		for i := range items {
			if items[i].ID == n.NetworkID {
				items[i].Routes = append(items[i].Routes, route.ID)
			}
		}
	}
	slices.SortFunc(items, func(a, b network) int { return strings.Compare(a.ID, b.ID) })
	respond(w, http.StatusOK, map[string]any{"items": items})
}

package admin

import (
	"net/http"
	"sort"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/player"
)

type networkSummary struct {
	ID           string        `json:"id"`
	IdentityMode string        `json:"identity_mode"`
	Nodes        int           `json:"nodes"`
	Connected    int           `json:"connected"`
	Counts       player.Counts `json:"counts"`
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	counts, err := h.engine.Players.Summary(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	nodes := h.engine.Hub.Status()
	adapters := h.engine.AdapterStatus()
	networks := map[string]*networkSummary{}
	for _, node := range h.engine.Config.Nodes {
		if networks[node.NetworkID] == nil {
			networks[node.NetworkID] = &networkSummary{ID: node.NetworkID, IdentityMode: node.IdentityMode, Counts: counts[node.NetworkID]}
		}
		networks[node.NetworkID].Nodes++
	}
	connectedNodes, connectedAdapters := 0, 0
	for _, node := range nodes {
		if node.Connected {
			connectedNodes++
			if network := networks[node.NetworkID]; network != nil {
				network.Connected++
			}
		}
	}
	for _, adapter := range adapters {
		if adapter.State == "connected" {
			connectedAdapters++
		}
	}
	total := player.Counts{}
	items := make([]networkSummary, 0, len(networks))
	for _, network := range networks {
		items = append(items, *network)
		total.Players += network.Counts.Players
		total.Online += network.Counts.Online
		total.Bound += network.Counts.Bound
		total.WhitelistOnly += network.Counts.WhitelistOnly
		total.Unlisted += network.Counts.Unlisted
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	respond(w, http.StatusOK, map[string]any{
		"generated_at": time.Now().UTC(), "counts": total, "networks": items,
		"nodes": len(nodes), "connected_nodes": connectedNodes,
		"adapters": len(h.engine.Config.Connections), "enabled_adapters": len(adapters), "connected_adapters": connectedAdapters,
		"routes": len(h.engine.Config.Routes), "commands": len(h.engine.Config.Commands),
	})
}

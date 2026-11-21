package admin

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"

	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
)

type settingsDocument struct {
	Revision        string        `json:"revision"`
	Config          config.Config `json:"config"`
	RestartRequired bool          `json:"restart_required"`
}

func (h *Handler) settingsDocument(doc config.Document) (settingsDocument, error) {
	cfg, err := config.Decode([]byte(doc.Text))
	if err != nil {
		return settingsDocument{}, fault.New("INVALID_CONFIG", err.Error())
	}
	cfg = normalizedSettings(cfg)
	return settingsDocument{Revision: doc.Revision, Config: cfg, RestartRequired: !reflect.DeepEqual(cfg, normalizedSettings(h.engine.Config))}, nil
}

// JSON forms represent empty collections as []; YAML may load them as nil.
// Normalize copies, including nested slices, so a round trip does not invent a
// restart requirement or mutate the immutable running configuration.
func normalizedSettings(cfg config.Config) config.Config {
	cfg.HTTP.AllowedHosts = append([]string{}, cfg.HTTP.AllowedHosts...)
	cfg.Connections = append([]config.Connection{}, cfg.Connections...)
	cfg.Nodes = append([]config.Node{}, cfg.Nodes...)
	for i := range cfg.Nodes {
		cfg.Nodes[i].Actions = append([]string{}, cfg.Nodes[i].Actions...)
	}
	cfg.Routes = append([]config.Route{}, cfg.Routes...)
	for i := range cfg.Routes {
		cfg.Routes[i].NodeIDs = append([]string{}, cfg.Routes[i].NodeIDs...)
	}
	cfg.Commands = append([]config.Command{}, cfg.Commands...)
	for i := range cfg.Commands {
		cfg.Commands[i].Arguments = append([]string{}, cfg.Commands[i].Arguments...)
		cfg.Commands[i].Roles = append([]string{}, cfg.Commands[i].Roles...)
	}
	return cfg
}

func (h *Handler) registerSettings() {
	h.mux.HandleFunc("GET /api/v1/config/sections", h.getSettings)
	h.mux.HandleFunc("PATCH /api/v1/config/sections", h.saveSettings)
}

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	doc, err := h.engine.ConfigStore.Read()
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := h.settingsDocument(doc)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", strconv.Quote(doc.Revision))
	respond(w, http.StatusOK, result)
}

func (h *Handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	var sections map[string]json.RawMessage
	if !decode(w, r, &sections) {
		return
	}
	revision, err := strconv.Unquote(r.Header.Get("If-Match"))
	if err != nil || revision == "" {
		respond(w, http.StatusPreconditionRequired, map[string]any{"error": map[string]string{"code": "REVISION_REQUIRED", "message": "请提供读取配置时的 ETag"}})
		return
	}
	doc, err := h.engine.ConfigStore.Read()
	if err != nil {
		writeError(w, err)
		return
	}
	if doc.Revision != revision {
		writeError(w, fault.New("REVISION_CONFLICT", "配置已变更，请保留草稿并重新读取后合并"))
		return
	}
	text, err := config.SectionText(doc.Text, sections)
	if err != nil {
		writeError(w, err)
		return
	}
	// Save compares the revision again under the store lock, closing the window
	// between reading/merging above and writing alongside a YAML editor.
	doc, err = h.engine.ConfigStore.Save(text, revision)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := h.settingsDocument(doc)
	if err != nil {
		writeError(w, err)
		return
	}
	h.engine.Events.Publish("config.saved", map[string]bool{"restart_required": result.RestartRequired})
	w.Header().Set("ETag", strconv.Quote(doc.Revision))
	respond(w, http.StatusOK, result)
}

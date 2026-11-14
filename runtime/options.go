// Package runtime embeds the ShitBotUltra Core in a CLI, service or other Go host.
// It does not parse flags, install signal handlers, exit the process or start a UI.
package runtime

import (
	"log/slog"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

const ComponentID = "shitbotteam.ultra.core"
const AdminAPIVersion = 1

type Options struct {
	// ConfigDir defaults to the user's application configuration directory.
	// New is side-effect free; Run takes exclusive ownership of this directory.
	ConfigDir string
	Logger    *slog.Logger
	Version   string
	// AdapterFactories adds compiled-in protocols without importing internals.
	// Built-in protocol names cannot be replaced.
	AdapterFactories map[string]adapter.Factory
}

type NodeStatus struct {
	ID           string    `json:"id"`
	NetworkID    string    `json:"network_id"`
	ServerID     string    `json:"server_id"`
	Connected    bool      `json:"connected"`
	BootID       string    `json:"boot_id,omitempty"`
	Platform     string    `json:"platform,omitempty"`
	Capabilities []string  `json:"capabilities"`
	Since        time.Time `json:"since,omitempty"`
}

type Status struct {
	Component             string           `json:"component"`
	Version               string           `json:"version"`
	AdminAPIVersion       int              `json:"admin_api_version"`
	BridgeProtocolVersion int              `json:"bridge_protocol_version"`
	InstanceID            string           `json:"instance_id"`
	State                 string           `json:"state"`
	URL                   string           `json:"url,omitempty"`
	StartedAt             time.Time        `json:"started_at,omitempty"`
	Nodes                 []NodeStatus     `json:"nodes"`
	Adapters              []adapter.Status `json:"adapters"`
	LastError             string           `json:"last_error,omitempty"`
}

type Validation struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

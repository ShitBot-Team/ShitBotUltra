// Package extension describes locally imported JavaScript workers and the
// permissions granted by an administrator when enabling them.
package extension

import "encoding/json"

const APIVersion = 1

type Command struct {
	Name            string   `json:"name"`
	Arguments       []string `json:"arguments"`
	Roles           []string `json:"roles"`
	RequireBinding  bool     `json:"require_binding"`
	CooldownSeconds int      `json:"cooldown_seconds"`
}

type Manifest struct {
	APIVersion   int             `json:"api_version"`
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Version      string          `json:"version"`
	Author       string          `json:"author"`
	ProjectURL   string          `json:"project_url"`
	License      string          `json:"license"`
	Entry        string          `json:"entry"`
	Capabilities []string        `json:"capabilities"`
	Events       []string        `json:"events,omitempty"`
	Commands     []Command       `json:"commands,omitempty"`
	Providers    []string        `json:"providers,omitempty"`
	TickSeconds  int             `json:"tick_seconds,omitempty"`
	ConfigSchema json.RawMessage `json:"config_schema,omitempty"`
}

type Grant struct {
	Capabilities []string `json:"capabilities"`
	Networks     []string `json:"networks"`
	Routes       []string `json:"routes"`
	Nodes        []string `json:"nodes"`
}

type Event struct {
	Type      string          `json:"type"`
	NetworkID string          `json:"network_id,omitempty"`
	RouteID   string          `json:"route_id,omitempty"`
	NodeID    string          `json:"node_id,omitempty"`
	Data      json.RawMessage `json:"data"`
}

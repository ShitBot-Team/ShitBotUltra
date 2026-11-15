package render

import (
	"context"
	"encoding/json"
	"time"
)

const APIVersion = 1

// Style is persisted per template and can be overridden by a render request.
type Style struct {
	Theme  json.RawMessage `json:"theme,omitempty"`
	CSS    string          `json:"css,omitempty"`
	Locale string          `json:"locale,omitempty"`
}

type Attribution struct {
	Project    string `json:"project"`
	Author     string `json:"author"`
	URL        string `json:"url"`
	Revision   string `json:"revision,omitempty"`
	ModifiedAt string `json:"modified_at,omitempty"`
	Changes    string `json:"changes,omitempty"`
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
	Width        int             `json:"width"`
	Height       int             `json:"height"`
	Scale        float64         `json:"scale"`
	Format       string          `json:"format"`
	DataSchema   json.RawMessage `json:"data_schema,omitempty"`
	Attributions []Attribution   `json:"attributions,omitempty"`
}

type Request struct {
	TemplateID string `json:"template_id"`
	// IgnoreDefaults allows a preview with deliberately empty CSS/theme/locale.
	IgnoreDefaults bool `json:"ignore_defaults,omitempty"`
	// Revision is optional for published templates; drafts must name it explicitly.
	Revision string          `json:"revision,omitempty"`
	Data     json.RawMessage `json:"data"`
	Theme    json.RawMessage `json:"theme,omitempty"`
	CSS      string          `json:"css,omitempty"`
	Locale   string          `json:"locale,omitempty"`
	Width    int             `json:"width,omitempty"`
	Height   int             `json:"height,omitempty"`
	Scale    float64         `json:"scale,omitempty"`
	Format   string          `json:"format,omitempty"`
}

type Result struct {
	ResourceID string    `json:"resource_id"`
	MIME       string    `json:"mime"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type Renderer interface {
	Render(context.Context, Request) (Result, error)
}

// DataProvider receives a caller-scoped request; the host validates access before
// invoking it and injects the returned immutable JSON into a template context.
type DataProvider func(context.Context, json.RawMessage) (json.RawMessage, error)

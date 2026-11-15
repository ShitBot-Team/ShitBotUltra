// Package update defines the boundary between Core and the process-owning host.
package update

import (
	"context"
	"time"
)

type Options struct {
	Repository     string // owner/repository, normally ShitBot-Team/ShitBotUltra-CLI
	CurrentVersion string
	// AssetName must identify one exact asset for this host's OS, architecture
	// and installation format. {version} expands to the release tag without v.
	AssetName        string
	Prereleases      bool
	CheckInterval    time.Duration // zero disables automatic checks; never auto-installs
	MaxDownloadBytes int64         // zero defaults to 512 MiB
	Installer        Installer
}

type Release struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
	AssetName string `json:"asset_name"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	Available bool   `json:"available"`
}

type State struct {
	Enabled        bool      `json:"enabled"`
	CanInstall     bool      `json:"can_install"`
	Repository     string    `json:"repository"`
	CurrentVersion string    `json:"current_version"`
	Phase          string    `json:"phase"`
	TaskID         string    `json:"task_id,omitempty"`
	CheckedAt      time.Time `json:"checked_at,omitempty"`
	Release        *Release  `json:"release,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	Backup         string    `json:"backup,omitempty"`
}

type Staged struct {
	Release        Release
	Path           string
	DatabaseBackup string
	ConfigBackup   string
}

// Installer prepares a verified asset for the owning CLI/service/desktop. It
// must validate package contents and API compatibility and arrange rollback.
// Return only once the helper/installer has accepted ownership of Path. Remove
// that file after consumption. Never synchronously wait for Core.Shutdown here:
// shutdown joins this operation. Schedule shutdown after returning instead.
// A nil result means handed off, not that the new version is running.
type Installer func(context.Context, Staged) error

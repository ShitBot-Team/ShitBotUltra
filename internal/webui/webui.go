// Package webui serves the embedded management shell. API authentication is
// independent of the page switch; the shell never embeds credentials.
package webui

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// assets is authored browser-module source, requiring no frontend build.
// A maintainer-created Vite bundle is selected when optimized/dist exists.
//
//go:embed assets optimized
var embedded embed.FS

type Service struct {
	enabled atomic.Bool
	files   fs.FS
}

type State struct {
	Enabled bool `json:"enabled"`
}

func New(enabled bool) *Service {
	files, _ := fs.Sub(embedded, "assets")
	if recorded, err := embedded.ReadFile("optimized/dist/source.sha256"); err == nil && strings.TrimSpace(string(recorded)) == digest(files) {
		if _, err := embedded.ReadFile("optimized/dist/index.html"); err == nil {
			files, _ = fs.Sub(embedded, "optimized/dist")
		}
	}
	s := &Service{files: files}
	s.enabled.Store(enabled)
	return s
}

func digest(files fs.FS) string {
	hash := sha256.New()
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(hash, name+"\x00")
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return ""
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (s *Service) SetEnabled(enabled bool) { s.enabled.Store(enabled) }
func (s *Service) Status() State           { return State{Enabled: s.enabled.Load()} }

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' blob:; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
	if !s.enabled.Load() {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) {
		http.NotFound(w, r)
		return
	}
	info, err := fs.Stat(s.files, name)
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
}

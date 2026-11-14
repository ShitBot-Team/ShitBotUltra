package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"gopkg.in/yaml.v3"
)

type Store struct {
	mu  sync.Mutex
	dir string
}
type Document struct {
	Text     string `json:"text"`
	Revision string `json:"revision"`
}

func Open(dir string) (*Store, Config, error) {
	s := &Store{dir: dir}
	for _, part := range []string{"secrets", "data", "backups", "imports", "cache"} {
		if err := securefs.PrivateDir(filepath.Join(dir, part)); err != nil {
			return nil, Config{}, err
		}
	}
	path := filepath.Join(dir, "config.yaml")
	data, err := securefs.ReadFile(path, MaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		c := Defaults()
		secretPath := filepath.Join(dir, "secrets", "admin.token")
		if _, secretErr := os.Stat(secretPath); errors.Is(secretErr, os.ErrNotExist) {
			if err = securefs.AtomicWrite(secretPath, []byte(securefs.ID()+securefs.ID()+"\n")); err != nil {
				return nil, c, err
			}
		} else if secretErr != nil {
			return nil, c, secretErr
		}
		data, err = yaml.Marshal(c)
		if err == nil {
			err = securefs.AtomicWrite(path, data)
		}
	}
	if err != nil {
		return nil, Config{}, err
	}
	c, err := Decode(data)
	return s, c, err
}

func Revision(data []byte) string { hash := sha256.Sum256(data); return hex.EncodeToString(hash[:]) }

func (s *Store) Read() (Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := securefs.ReadFile(filepath.Join(s.dir, "config.yaml"), MaxBytes)
	return Document{Text: string(data), Revision: Revision(data)}, err
}

// Save only persists a validated configuration. The active snapshot is immutable;
// callers must report restart_required, never claim connections have hot-reloaded.
func (s *Store) Save(text, revision string) (Document, error) {
	if _, err := Decode([]byte(text)); err != nil {
		return Document{}, fault.New("INVALID_CONFIG", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.dir, "config.yaml")
	old, err := securefs.ReadFile(path, MaxBytes)
	if err != nil {
		return Document{}, err
	}
	if revision == "" || Revision(old) != revision {
		return Document{}, fault.New("REVISION_CONFLICT", "配置已被其他编辑者修改，请重新读取")
	}
	backup := filepath.Join(s.dir, "backups", "config-"+time.Now().UTC().Format("20060102T150405")+"-"+securefs.ID()+".yaml")
	if err := securefs.AtomicWrite(backup, old); err != nil {
		return Document{}, err
	}
	if err := securefs.AtomicWrite(path, []byte(text)); err != nil {
		return Document{}, err
	}
	return Document{Text: text, Revision: Revision([]byte(text))}, nil
}

func Secret(dir, ref string) (string, error) {
	if !validRef(ref) {
		return "", errors.New("invalid secret reference")
	}
	var value string
	if strings.HasPrefix(ref, "env:") {
		value = os.Getenv(strings.TrimPrefix(ref, "env:"))
	} else {
		path := filepath.Join(dir, "secrets", strings.TrimPrefix(ref, "file:"))
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("secret %s unavailable", ref)
		}
		if !info.Mode().IsRegular() {
			return "", errors.New("secret must be a regular file, not a link")
		}
		data, err := securefs.ReadFile(path, 4096)
		if err != nil {
			return "", fmt.Errorf("secret %s unavailable", ref)
		}
		value = string(data)
	}
	value = strings.TrimSpace(value)
	if len(value) < 32 || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
		return "", fmt.Errorf("secret %s must contain 32 to 4096 characters", ref)
	}
	return value, nil
}

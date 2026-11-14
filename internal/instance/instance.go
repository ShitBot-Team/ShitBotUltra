package instance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/gofrs/flock"
)

type Lock struct {
	file *flock.Flock
	dir  string
}

type Info struct {
	InstanceID string    `json:"instance_id"`
	PID        int       `json:"pid"`
	URL        string    `json:"url"`
	StartedAt  time.Time `json:"started_at"`
}

func Acquire(dir string) (*Lock, error) {
	run := filepath.Join(dir, "run")
	if err := securefs.PrivateDir(run); err != nil {
		return nil, err
	}
	f := flock.New(filepath.Join(run, "core.lock"))
	ok, err := f.TryLock()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !ok {
		_ = f.Close()
		return nil, fmt.Errorf("configuration directory already owned by another Core: %s", dir)
	}
	return &Lock{file: f, dir: run}, nil
}

func (l *Lock) Publish(info Info) error {
	info.PID = os.Getpid()
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return securefs.AtomicWrite(filepath.Join(l.dir, "instance.json"), data)
}

func (l *Lock) Close() error {
	// Never unlink the lock file: a second process could otherwise lock a new inode.
	_ = os.Remove(filepath.Join(l.dir, "instance.json"))
	return l.file.Close()
}

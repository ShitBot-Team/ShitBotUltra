//go:build !windows

package securefs

import (
	"os"
	"path/filepath"
)

func restrict(path string, dir bool) error {
	if dir {
		return os.Chmod(path, 0700)
	}
	return os.Chmod(path, 0600)
}

func replace(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(dst))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

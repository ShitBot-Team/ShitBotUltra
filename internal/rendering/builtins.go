package rendering

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"io/fs"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/rendering/builtin"
)

func (c *Catalog) SeedBuiltins(ctx context.Context) error {
	root := builtin.Files()
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		files, err := fs.Sub(root, entry.Name())
		if err != nil {
			return err
		}
		var buffer bytes.Buffer
		archive := zip.NewWriter(&buffer)
		err = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			output, err := archive.Create(name)
			if err != nil {
				return err
			}
			input, err := files.Open(name)
			if err != nil {
				return err
			}
			_, err = io.Copy(output, input)
			input.Close()
			return err
		})
		closeErr := archive.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		stage, err := packagefs.Extract(ctx, c.dir, &buffer)
		if err != nil {
			return err
		}
		_, err = c.install(stage, true, "")
		stage.Discard()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Catalog) RestoreBuiltin(id, revision string) (Template, error) {
	c.mu.RLock()
	state, err := c.load(id)
	c.mu.RUnlock()
	if err != nil {
		return state, err
	}
	for index := len(state.Versions) - 1; index >= 0; index-- {
		if state.Versions[index].Builtin {
			return c.Publish(id, state.Versions[index].Revision, revision)
		}
	}
	return state, fault.New("NOT_FOUND", "模板没有内置版本")
}

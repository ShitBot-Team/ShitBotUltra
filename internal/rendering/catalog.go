package rendering

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/render"
)

type Version struct {
	Revision   string          `json:"revision"`
	Manifest   render.Manifest `json:"manifest"`
	ImportedAt time.Time       `json:"imported_at"`
	Builtin    bool            `json:"builtin"`
}
type Template struct {
	ID        string    `json:"id"`
	Published string    `json:"published"`
	Draft     string    `json:"draft"`
	Revision  string    `json:"revision"`
	Versions  []Version `json:"versions"`
	Defaults  render.Style `json:"defaults"`
}
type Catalog struct {
	mu  sync.RWMutex
	dir string
}

func NewCatalog(dir string) (*Catalog, error) {
	if err := securefs.PrivateDir(dir); err != nil {
		return nil, err
	}
	return &Catalog{dir: dir}, nil
}

func validateManifest(m render.Manifest) error {
	if m.APIVersion != render.APIVersion || !packagefs.ValidID(m.ID) || !packagefs.ValidVersion(m.Version) || len(m.Name) == 0 || len(m.Name) > 128 || len(m.Author) == 0 || len(m.Author) > 256 || len(m.License) == 0 || len(m.License) > 256 {
		return errors.New("invalid template manifest")
	}
	u, err := url.Parse(m.ProjectURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return errors.New("template requires an HTTPS project URL")
	}
	if !packagefs.SafePath(m.Entry) || !strings.EqualFold(filepath.Ext(m.Entry), ".html") {
		return errors.New("template entry must be a local HTML file")
	}
	if err := Dimensions(m.Width, m.Height, m.Scale, m.Format); err != nil {
		return err
	}
	if len(m.Attributions) > 32 {
		return errors.New("too many template attributions")
	}
	_, err = packagefs.Schema(m.DataSchema)
	return err
}

func Dimensions(width, height int, scale float64, format string) error {
	if width < 64 || width > 4096 || height < 64 || height > 8192 || scale < 0.5 || scale > 3 || float64(width)*float64(height)*scale*scale > 32e6 || (format != "png" && format != "jpeg") {
		return errors.New("invalid render size, scale, pixel count or format")
	}
	return nil
}

func (c *Catalog) load(id string) (Template, error) {
	if !packagefs.ValidID(id) {
		return Template{}, fault.New("INVALID_INPUT", "模板标识无效")
	}
	var state Template
	err := packagefs.ReadJSON(filepath.Join(c.dir, id), "state.json", &state)
	if errors.Is(err, os.ErrNotExist) {
		return Template{}, fault.New("NOT_FOUND", "模板不存在")
	}
	return state, err
}

func (c *Catalog) save(state Template) error {
	state.Revision = securefs.ID()
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return securefs.AtomicWrite(filepath.Join(c.dir, state.ID, "state.json"), data)
}

func (c *Catalog) Import(ctx context.Context, reader io.Reader) (Template, error) {
	stage, err := packagefs.Extract(ctx, c.dir, reader)
	if err != nil {
		return Template{}, fault.New("INVALID_PACKAGE", err.Error())
	}
	defer stage.Discard()
	return c.install(stage, false, "")
}
func (c *Catalog) ImportDirectory(ctx context.Context, dir string) (Template, error) {
	stage, err := packagefs.CopyDirectory(ctx, c.dir, dir)
	if err != nil {
		return Template{}, err
	}
	defer stage.Discard()
	return c.install(stage, false, "")
}

func (c *Catalog) install(stage *packagefs.Stage, builtin bool, expected string) (Template, error) {
	var manifest render.Manifest
	if err := packagefs.ReadJSON(stage.Dir, "template.json", &manifest); err != nil {
		return Template{}, fault.New("INVALID_PACKAGE", "模板清单无效")
	}
	if err := validateManifest(manifest); err != nil {
		return Template{}, fault.New("INVALID_PACKAGE", err.Error())
	}
	if _, err := packagefs.Entry(stage.Dir, manifest.Entry); err != nil {
		return Template{}, fault.New("INVALID_PACKAGE", "模板入口不存在")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, err := c.load(manifest.ID)
	if fault.Is(err, "NOT_FOUND") {
		state = Template{ID: manifest.ID, Versions: []Version{}}
	} else if err != nil {
		return Template{}, err
	}
	if expected != "" && expected != state.Revision { return Template{}, fault.New("REVISION_CONFLICT", "模板状态已变化") }
	for _, version := range state.Versions {
		if version.Revision == stage.Digest {
			if expected != "" { state.Draft = stage.Digest; if err := c.save(state); err != nil { return Template{}, err }; return c.load(state.ID) }
			return state, nil
		}
		if expected == "" && version.Manifest.Version == manifest.Version {
			return Template{}, fault.New("REVISION_CONFLICT", "相同模板版本包含不同内容，请先修改版本号")
		}
	}
	if len(state.Versions) >= 64 {
		return Template{}, fault.New("CAPACITY_EXCEEDED", "模板版本数量已达到上限")
	}
	base := filepath.Join(c.dir, manifest.ID)
	if err := securefs.PrivateDir(base); err != nil {
		return Template{}, err
	}
	target := filepath.Join(base, stage.Digest)
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(stage.Dir, target); err != nil {
			return Template{}, err
		}
		stage.Dir = ""
	} else if err != nil {
		return Template{}, err
	} else {
		hash, err := packagefs.Hash(target)
		if err != nil || hash != stage.Digest {
			return Template{}, errors.New("existing template package digest mismatch")
		}
	}
	state.Versions = append(state.Versions, Version{Revision: stage.Digest, Manifest: manifest, ImportedAt: time.Now().UTC(), Builtin: builtin})
	state.Draft = stage.Digest
	if builtin && state.Published == "" {
		state.Published = stage.Digest
	}
	if err := c.save(state); err != nil {
		return Template{}, err
	}
	return c.load(manifest.ID)
}

func (c *Catalog) List() ([]Template, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, err
	}
	result := []Template{}
	for _, entry := range entries {
		if !entry.IsDir() || !packagefs.ValidID(entry.Name()) {
			continue
		}
		state, err := c.load(entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, state)
	}
	slices.SortFunc(result, func(a, b Template) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
}

func (c *Catalog) Publish(id, digest, revision string) (Template, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state, err := c.load(id)
	if err != nil {
		return Template{}, err
	}
	if revision == "" || state.Revision != revision {
		return Template{}, fault.New("REVISION_CONFLICT", "模板状态已变化")
	}
	index := slices.IndexFunc(state.Versions, func(v Version) bool { return v.Revision == digest })
	if index < 0 {
		return Template{}, fault.New("NOT_FOUND", "模板版本不存在")
	}
	dir := filepath.Join(c.dir, id, digest)
	current, err := packagefs.Hash(dir)
	if err != nil || current != digest {
		return Template{}, fault.New("REVISION_CONFLICT", "模板文件已被外部修改，请重新导入")
	}
	state.Published = digest
	if err := c.save(state); err != nil {
		return Template{}, err
	}
	return c.load(id)
}

func (c *Catalog) Resolve(id, digest string) (string, Version, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state, err := c.load(id)
	if err != nil {
		return "", Version{}, err
	}
	if digest == "" {
		digest = state.Published
	}
	if !packagefs.ValidDigest(digest) {
		return "", Version{}, fault.New("NOT_FOUND", "模板尚未发布")
	}
	for _, version := range state.Versions {
		if version.Revision == digest {
			dir := filepath.Join(c.dir, id, digest)
			hash, err := packagefs.Hash(dir)
			if err != nil || hash != digest {
				return "", Version{}, fault.New("REVISION_CONFLICT", "模板文件与已导入摘要不一致")
			}
			return dir, version, nil
		}
	}
	return "", Version{}, fault.New("NOT_FOUND", "模板版本不存在")
}

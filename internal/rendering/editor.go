package rendering

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/packagefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/render"
)

type DraftRequest struct {
	Revision     string `json:"revision"`
	BaseRevision string `json:"base_revision"`
	// A null value removes a file. Metadata and license files are changed by importing a new version.
	Files map[string]*string `json:"files"`
}

func editable(name string) bool {
	return packagefs.SafePath(name) && name != "template.json" && slices.Contains([]string{".html", ".css", ".js", ".json"}, strings.ToLower(filepath.Ext(name)))
}

func (c *Catalog) Edit(ctx context.Context, id string, request DraftRequest) (Template, error) {
	if request.Revision == "" || request.BaseRevision == "" || len(request.Files) == 0 || len(request.Files) > 32 {
		return Template{}, fault.New("INVALID_INPUT", "草稿必须包含状态版本、源版本及 1 至 32 个文件")
	}
	dir, _, err := c.Resolve(id, request.BaseRevision)
	if err != nil {
		return Template{}, err
	}
	stage, err := packagefs.CopyDirectory(ctx, c.dir, dir)
	if err != nil {
		return Template{}, err
	}
	defer stage.Discard()
	for name, content := range request.Files {
		if !editable(name) {
			return Template{}, fault.New("INVALID_INPUT", "只允许编辑模板中的 HTML、CSS、JS 和数据 JSON 文件")
		}
		target := filepath.Join(stage.Dir, filepath.FromSlash(name))
		if content == nil {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return Template{}, err
			}
			continue
		}
		if len(*content) > 1<<20 || !utf8.ValidString(*content) {
			return Template{}, fault.New("INVALID_INPUT", "草稿文件须为不超过 1 MiB 的 UTF-8 文本")
		}
		if err := securefs.PrivateDir(filepath.Dir(target)); err != nil {
			return Template{}, err
		}
		if err := securefs.AtomicWrite(target, []byte(*content)); err != nil {
			return Template{}, err
		}
	}
	stage.Digest, err = packagefs.Hash(stage.Dir)
	if err != nil {
		return Template{}, err
	}
	return c.install(stage, false, request.Revision)
}

func (c *Catalog) Files(id, revision string) ([]string, error) {
	dir, _, err := c.Resolve(id, revision)
	if err != nil {
		return nil, err
	}
	result := []string{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		result = append(result, filepath.ToSlash(name))
		return nil
	})
	slices.Sort(result)
	return result, err
}

func (c *Catalog) File(id, revision, name string) (string, error) {
	if !packagefs.SafePath(name) {
		return "", fault.New("INVALID_INPUT", "模板文件路径无效")
	}
	dir, _, err := c.Resolve(id, revision)
	if err != nil {
		return "", err
	}
	path, err := packagefs.Entry(dir, name)
	if err != nil {
		return "", fault.New("NOT_FOUND", "模板文件不存在")
	}
	data, err := securefs.ReadFile(path, 1<<20)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fault.New("INVALID_INPUT", "该文件不是 UTF-8 文本")
	}
	return string(data), nil
}

func validateStyle(style render.Style) error {
	if len(style.CSS) > 64<<10 || len(style.Locale) > 32 {
		return fault.New("INVALID_INPUT", "模板样式或语言参数过长")
	}
	if len(style.Theme) > 0 {
		if err := packagefs.ValidateJSON(nil, style.Theme); err != nil {
			return fault.New("INVALID_INPUT", "主题须为有效 JSON")
		}
	}
	return nil
}

func (c *Catalog) Configure(id, revision string, style render.Style) (Template, error) {
	if err := validateStyle(style); err != nil {
		return Template{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state, err := c.load(id)
	if err != nil {
		return Template{}, err
	}
	if revision == "" || state.Revision != revision {
		return Template{}, fault.New("REVISION_CONFLICT", "模板状态已变化")
	}
	state.Defaults = style
	if err := c.save(state); err != nil {
		return Template{}, err
	}
	return c.load(id)
}

func (c *Catalog) Defaults(id string) (render.Style, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state, err := c.load(id)
	return state.Defaults, err
}

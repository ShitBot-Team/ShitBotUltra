// Package packagefs provides bounded local package IO. Template and plugin
// manifests and their execution lifecycles are intentionally separate.
package packagefs

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
)

const MaxArchiveBytes = 64 << 20
const MaxFileBytes = 16 << 20
const MaxEntries = 2048

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
var version = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-zA-Z0-9.-]+)?$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ValidID(id string) bool         { return identifier.MatchString(id) && SafePath(id) }
func ValidVersion(value string) bool { return len(value) <= 64 && version.MatchString(value) }
func ValidDigest(value string) bool  { return digestPattern.MatchString(value) }

func SafePath(name string) bool {
	if name == "" || len(name) > 240 || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if strings.TrimSpace(part) != part || strings.HasSuffix(part, ".") {
			return false
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if slices.Contains([]string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}, stem) {
			return false
		}
	}
	return true
}

type Stage struct {
	Dir    string
	Digest string
	base   string
}

func (s *Stage) Discard() {
	if s.Dir != "" {
		_ = Remove(s.base, s.Dir)
		s.Dir = ""
	}
}

func Remove(base, target string) error {
	absBase, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(absBase, absTarget)
	if err != nil || relative == "." || !filepath.IsLocal(relative) {
		return errors.New("refusing package removal outside its root")
	}
	return os.RemoveAll(absTarget)
}

func Extract(ctx context.Context, base string, reader io.Reader) (*Stage, error) {
	if err := securefs.PrivateDir(base); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(base, ".upload-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	n, err := io.Copy(f, io.LimitReader(reader, MaxArchiveBytes+1))
	if err != nil {
		return nil, err
	}
	if n > MaxArchiveBytes {
		return nil, errors.New("package archive exceeds 64 MiB")
	}
	archive, err := zip.NewReader(f, n)
	if err != nil {
		return nil, errors.New("invalid ZIP package")
	}
	if len(archive.File) == 0 || len(archive.File) > MaxEntries {
		return nil, errors.New("invalid package entry count")
	}
	dir, err := os.MkdirTemp(base, ".stage-*")
	if err != nil {
		return nil, err
	}
	stage := &Stage{Dir: dir, base: base}
	ok := false
	defer func() {
		if !ok {
			stage.Discard()
		}
	}()
	seen := map[string]bool{}
	var total int64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		key := strings.ToLower(name)
		if !SafePath(name) || seen[key] {
			return nil, errors.New("unsafe or duplicate package path")
		}
		seen[key] = true
		mode := entry.Mode()
		if !mode.IsDir() && !mode.IsRegular() {
			return nil, errors.New("package links and special files are not supported")
		}
		destination := filepath.Join(dir, filepath.FromSlash(name))
		if mode.IsDir() {
			if err := os.MkdirAll(destination, 0700); err != nil {
				return nil, err
			}
			continue
		}
		if entry.UncompressedSize64 > MaxFileBytes {
			return nil, errors.New("package file exceeds 16 MiB")
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return nil, err
		}
		input, err := entry.Open()
		if err != nil {
			return nil, err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			input.Close()
			return nil, err
		}
		copied, copyErr := io.Copy(output, io.LimitReader(input, MaxFileBytes+1))
		inputErr := input.Close()
		closeErr := output.Close()
		total += copied
		if copyErr != nil {
			return nil, copyErr
		}
		if inputErr != nil {
			return nil, inputErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if copied > MaxFileBytes || total > MaxArchiveBytes {
			return nil, errors.New("unpacked package exceeds size limit")
		}
	}
	stage.Digest, err = Hash(dir)
	if err != nil {
		return nil, err
	}
	ok = true
	return stage, nil
}

// CopyDirectory imports only the explicitly selected tree, never neighboring
// plugins, resources or server files. No symlinks or reparse point files are copied.
func CopyDirectory(ctx context.Context, base, source string) (*Stage, error) {
	if err := securefs.PrivateDir(base); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(base, ".stage-*")
	if err != nil {
		return nil, err
	}
	stage := &Stage{Dir: dir, base: base}
	ok := false
	defer func() {
		if !ok {
			stage.Discard()
		}
	}()
	root, err := os.OpenRoot(source)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var total int64
	count := 0
	seen := map[string]bool{}
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		count++
		if count > MaxEntries || !SafePath(name) || seen[strings.ToLower(name)] {
			return errors.New("invalid package paths or file count")
		}
		seen[strings.ToLower(name)] = true
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("package symlinks are not supported")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		destination := filepath.Join(dir, filepath.FromSlash(name))
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		if !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
			return errors.New("invalid package file")
		}
		total += info.Size()
		if total > MaxArchiveBytes {
			return errors.New("package exceeds size limit")
		}
		input, err := root.Open(name)
		if err != nil {
			return err
		}
		defer input.Close()
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		copied, err := io.Copy(output, io.LimitReader(input, MaxFileBytes+1))
		closeErr := output.Close()
		if err != nil {
			return err
		}
		if copied != info.Size() {
			return errors.New("package changed during import")
		}
		return closeErr
	})
	if err != nil {
		return nil, err
	}
	stage.Digest, err = Hash(dir)
	if err != nil {
		return nil, err
	}
	ok = true
	return stage, nil
}

func Hash(dir string) (string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer root.Close()
	hash := sha256.New()
	count := 0
	var total int64
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		count++
		if count > MaxEntries || !SafePath(name) || entry.Type()&os.ModeSymlink != 0 {
			return errors.New("invalid installed package")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > MaxFileBytes {
			return errors.New("invalid installed package file")
		}
		total += info.Size()
		if total > MaxArchiveBytes {
			return errors.New("installed package exceeds limit")
		}
		_, _ = io.WriteString(hash, name+"\x00")
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		_, err = io.Copy(hash, io.LimitReader(f, MaxFileBytes+1))
		f.Close()
		if err != nil {
			return err
		}
		_, _ = hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func ReadJSON(dir, name string, value any) error {
	if !SafePath(name) {
		return errors.New("invalid manifest path")
	}
	data, err := securefs.ReadFile(filepath.Join(dir, filepath.FromSlash(name)), 256<<10)
	if err != nil {
		return err
	}
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("manifest contains trailing data")
	}
	return nil
}

func Entry(dir, name string) (string, error) {
	if !SafePath(name) || path.Ext(name) == "" {
		return "", errors.New("invalid package entry")
	}
	full := filepath.Join(dir, filepath.FromSlash(name))
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("package entry is not a regular file")
	}
	return full, nil
}

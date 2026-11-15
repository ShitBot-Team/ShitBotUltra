package updating

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	sdk "github.com/ShitBot-Team/ShitBotUltra/sdk/update"
	"golang.org/x/mod/semver"
)

type Backup func(context.Context, string) (database, configuration string, err error)

type Service struct {
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	wg          sync.WaitGroup
	closed      bool
	started     bool
	options     sdk.Options
	state       sdk.State
	downloadURL string
	dir         string
	client      *http.Client
	backup      Backup
	publish     func(string, any)
}

var repository = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var assetName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.+-]{0,199}$`)
var checksumLine = regexp.MustCompile(`^([a-fA-F0-9]{64}) [ *](.+)$`)

func normalized(version string) string {
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func New(parent context.Context, dir string, options sdk.Options, backup Backup, publish func(string, any)) (*Service, error) {
	if backup == nil || publish == nil {
		return nil, errors.New("updates require backup and event callbacks")
	}
	if !repository.MatchString(options.Repository) || !assetName.MatchString(strings.ReplaceAll(options.AssetName, "{version}", "1.0.0")) {
		return nil, errors.New("updates require an explicit repository and exact platform asset name")
	}
	if !semver.IsValid(normalized(options.CurrentVersion)) {
		return nil, errors.New("updates require the host's semantic current version")
	}
	if options.CheckInterval != 0 && (options.CheckInterval < time.Hour || options.CheckInterval > 7*24*time.Hour) {
		return nil, errors.New("update check interval must be zero or between one hour and seven days")
	}
	if options.MaxDownloadBytes == 0 {
		options.MaxDownloadBytes = 512 << 20
	}
	if options.MaxDownloadBytes < 1 || options.MaxDownloadBytes > 2<<30 {
		return nil, errors.New("invalid update size limit")
	}
	dir = filepath.Join(dir, "cache", "updates")
	if err := securefs.PrivateDir(dir); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 4, MaxIdleConnsPerHost: 2,
		IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second}
	client := &http.Client{Transport: transport, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !downloadAddress(r.URL) {
			return errors.New("untrusted update redirect")
		}
		return nil
	}}
	return &Service{ctx: ctx, cancel: cancel, options: options, dir: dir, client: client, backup: backup, publish: publish,
		state: sdk.State{Enabled: true, CanInstall: options.Installer != nil, Repository: options.Repository, CurrentVersion: options.CurrentVersion, Phase: "idle"}}, nil
}

func downloadAddress(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		return true
	}
	return false
}

func (s *Service) get(ctx context.Context, address string, limit int64) ([]byte, error) {
	r, err := s.open(ctx, address)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fault.New("INVALID_RESPONSE", "更新服务响应过大")
	}
	return data, nil
}

func (s *Service) open(ctx context.Context, address string) (*http.Response, error) {
	u, err := url.Parse(address)
	if err != nil || !downloadAddress(u) {
		return nil, fault.New("INVALID_RESPONSE", "更新下载地址无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ShitBotUltra-update/1")
	if u.Hostname() == "api.github.com" {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		if response.StatusCode == 403 || response.StatusCode == 429 {
			return nil, fault.New("RATE_LIMITED", "GitHub 暂时限制了更新请求")
		}
		return nil, fault.New("UPDATE_UNAVAILABLE", "无法取得对应的 GitHub Release 或资产")
	}
	return response, nil
}

func (s *Service) Status() sdk.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	if state.Release != nil {
		release := *state.Release
		state.Release = &release
	}
	return state
}

func (s *Service) busy() bool {
	return s.state.Phase == "checking" || s.state.Phase == "downloading" || s.state.Phase == "handing_off" || s.state.Phase == "handed_off"
}

func (s *Service) Check(parent context.Context) (sdk.State, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return sdk.State{}, fault.New("UNAVAILABLE", "更新服务已停止")
	}
	if s.busy() {
		s.mu.Unlock()
		return sdk.State{}, fault.New("BUSY", "已有更新任务或待重启更新")
	}
	s.wg.Add(1)
	s.state.Phase, s.state.LastError, s.state.TaskID = "checking", "", ""
	s.mu.Unlock()
	defer s.wg.Done()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	release, address, err := s.check(ctx)
	s.mu.Lock()
	if err != nil {
		s.state.Phase, s.state.LastError = "failed", fault.Public(err).Code
		s.state.Release, s.downloadURL = nil, ""
		if release.Version != "" {
			s.state.Release = &release
		}
	} else {
		s.state.Phase, s.state.CheckedAt = "idle", time.Now().UTC()
		s.state.Release, s.downloadURL = &release, address
	}
	s.mu.Unlock()
	state := s.Status()
	s.publish("update.changed", state)
	return state, err
}

type remoteAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type remoteRelease struct {
	Tag        string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Body       string        `json:"body"`
	Assets     []remoteAsset `json:"assets"`
}

func (s *Service) check(ctx context.Context) (sdk.Release, string, error) {
	data, err := s.get(ctx, "https://api.github.com/repos/"+s.options.Repository+"/releases?per_page=100", 8<<20)
	if err != nil {
		return sdk.Release{}, "", err
	}
	var releases []remoteRelease
	if json.Unmarshal(data, &releases) != nil {
		return sdk.Release{}, "", fault.New("INVALID_RESPONSE", "Release 数据无效")
	}
	var selected *remoteRelease
	for i := range releases {
		candidate := &releases[i]
		v := normalized(candidate.Tag)
		if candidate.Draft || !semver.IsValid(v) || (!s.options.Prereleases && (candidate.Prerelease || semver.Prerelease(v) != "")) {
			continue
		}
		if selected == nil || semver.Compare(v, normalized(selected.Tag)) > 0 {
			selected = candidate
		}
	}
	if selected == nil {
		return sdk.Release{}, "", fault.New("UPDATE_UNAVAILABLE", "没有可用的语义化版本 Release")
	}
	notes := selected.Body
	if len(notes) > 64000 {
		notes = strings.ToValidUTF8(notes[:64000], "")
	}
	release := sdk.Release{Version: selected.Tag, URL: "https://github.com/" + s.options.Repository + "/releases/tag/" + url.PathEscape(selected.Tag), Notes: notes,
		Available: semver.Compare(normalized(selected.Tag), normalized(s.options.CurrentVersion)) > 0}
	wanted := strings.ReplaceAll(s.options.AssetName, "{version}", strings.TrimPrefix(selected.Tag, "v"))
	release.AssetName = wanted
	if !assetName.MatchString(wanted) {
		return release, "", fault.New("INVALID_RESPONSE", "Release 资产名称无效")
	}
	var chosen *remoteAsset
	for i := range selected.Assets {
		if selected.Assets[i].Name != wanted {
			continue
		}
		if chosen != nil {
			return release, "", fault.New("UPDATE_UNAVAILABLE", "Release 存在重复的目标资产")
		}
		chosen = &selected.Assets[i]
	}
	if chosen == nil || chosen.Size <= 0 || chosen.Size > s.options.MaxDownloadBytes {
		return release, "", fault.New("UPDATE_UNAVAILABLE", "没有唯一匹配当前平台且大小有效的资产")
	}
	if !s.releaseAsset(chosen.URL, selected.Tag, chosen.Name) {
		return release, "", fault.New("INVALID_RESPONSE", "资产不属于所选 Release")
	}
	hash := strings.TrimPrefix(chosen.Digest, "sha256:")
	if chosen.Digest == "" {
		var sums *remoteAsset
		for i := range selected.Assets {
			if selected.Assets[i].Name != "SHA256SUMS" {
				continue
			}
			if sums != nil {
				return release, "", fault.New("INVALID_RESPONSE", "存在重复摘要清单")
			}
			sums = &selected.Assets[i]
		}
		if sums == nil || !s.releaseAsset(sums.URL, selected.Tag, sums.Name) {
			return release, "", fault.New("UPDATE_UNAVAILABLE", "资产缺少 SHA-256")
		}
		contents, err := s.get(ctx, sums.URL, 2<<20)
		if err != nil {
			return release, "", err
		}
		scanner := bufio.NewScanner(strings.NewReader(string(contents)))
		for scanner.Scan() {
			match := checksumLine.FindStringSubmatch(strings.TrimSuffix(scanner.Text(), "\r"))
			if len(match) != 3 || match[2] != wanted {
				continue
			}
			if hash != "" {
				return release, "", fault.New("INVALID_RESPONSE", "目标资产有多个摘要")
			}
			hash = match[1]
		}
		if err := scanner.Err(); err != nil {
			return release, "", fault.New("INVALID_RESPONSE", "摘要清单无效")
		}
	} else if !strings.HasPrefix(chosen.Digest, "sha256:") {
		return release, "", fault.New("UPDATE_UNAVAILABLE", "资产未提供 SHA-256")
	}
	decoded, err := hex.DecodeString(hash)
	if err != nil || len(decoded) != sha256.Size {
		return release, "", fault.New("INVALID_RESPONSE", "SHA-256 摘要无效")
	}
	release.Size, release.SHA256 = chosen.Size, strings.ToLower(hash)
	return release, chosen.URL, nil
}

func (s *Service) releaseAsset(address, tag, name string) bool {
	u, err := url.Parse(address)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.RawQuery == "" && u.Fragment == "" &&
		u.Path == "/"+s.options.Repository+"/releases/download/"+tag+"/"+name
}

// Install starts exactly one task using the version/digest the administrator saw.
func (s *Service) Install(version, digest string) (sdk.State, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return sdk.State{}, fault.New("UNAVAILABLE", "更新服务已停止")
	}
	if s.options.Installer == nil {
		s.mu.Unlock()
		return sdk.State{}, fault.New("UNSUPPORTED_CAPABILITY", "当前宿主没有注册升级处理器")
	}
	if s.busy() {
		s.mu.Unlock()
		return sdk.State{}, fault.New("BUSY", "已有更新任务或待重启更新")
	}
	r := s.state.Release
	if r == nil || !r.Available || r.SHA256 == "" || s.downloadURL == "" || r.Version != version || r.SHA256 != digest || time.Since(s.state.CheckedAt) > time.Hour {
		s.mu.Unlock()
		return sdk.State{}, fault.New("REVISION_CONFLICT", "版本信息已变化或过期，请重新检查")
	}
	release, address, id := *r, s.downloadURL, securefs.ID()
	s.state.Phase, s.state.TaskID, s.state.LastError, s.state.Backup = "downloading", id, "", ""
	s.wg.Add(1)
	s.mu.Unlock()
	state := s.Status()
	s.publish("update.changed", state)
	go s.install(release, address, id)
	return state, nil
}

func (s *Service) install(release sdk.Release, address, id string) {
	defer s.wg.Done()
	ctx, cancel := context.WithTimeout(s.ctx, 20*time.Minute)
	defer cancel()
	var path string
	var err error
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("host update handler panicked")
		}
		s.mu.Lock()
		if err != nil {
			s.state.Phase, s.state.LastError = "failed", fault.Public(err).Code
			if path != "" {
				_ = os.Remove(path)
			}
		} else {
			s.state.Phase = "handed_off"
		}
		s.mu.Unlock()
		s.publish("update.changed", s.Status())
	}()
	entries, readErr := os.ReadDir(s.dir)
	if readErr != nil {
		err = readErr
		return
	}
	if len(entries) >= 4 {
		err = fault.New("CAPACITY_EXCEEDED", "更新缓存已满，请清理宿主已消费的安装资产")
		return
	}
	path = filepath.Join(s.dir, id+"-"+release.AssetName)
	file, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if openErr != nil {
		err = openErr
		return
	}
	defer file.Close()
	response, fetchErr := s.open(ctx, address)
	if fetchErr != nil {
		err = fetchErr
		return
	}
	defer response.Body.Close()
	hash := sha256.New()
	var written int64
	written, err = io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, release.Size+1))
	if err != nil {
		return
	}
	if written != release.Size || hex.EncodeToString(hash.Sum(nil)) != release.SHA256 {
		err = fault.New("INVALID_RESPONSE", "更新资产大小或 SHA-256 不匹配")
		return
	}
	if err = file.Sync(); err != nil {
		return
	}
	if err = file.Close(); err != nil {
		return
	}
	if err = ctx.Err(); err != nil {
		return
	}
	var database, configuration string
	database, configuration, err = s.backup(ctx, id)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.state.Phase, s.state.Backup = "handing_off", filepath.Base(database)
	s.mu.Unlock()
	s.publish("update.changed", s.Status())
	err = s.options.Installer(ctx, sdk.Staged{Release: release, Path: path, DatabaseBackup: database, ConfigBackup: configuration})
}

func (s *Service) Start() {
	s.mu.Lock()
	if s.closed || s.started || s.options.CheckInterval == 0 {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		defer s.wg.Done()
		timer := time.NewTicker(s.options.CheckInterval)
		defer timer.Stop()
		for {
			_, _ = s.Check(s.ctx)
			select {
			case <-s.ctx.Done():
				return
			case <-timer.C:
			}
		}
	}()
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.wg.Wait()
	s.client.CloseIdleConnections()
}

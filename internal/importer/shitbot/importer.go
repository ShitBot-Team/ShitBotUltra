package shitbot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/binding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage/sqlite"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

const MaxSourceBytes = 64 << 20
const maxRows = 20000

type Database interface {
	storage.Database
	Backup(context.Context, string) error
}
type Source struct {
	ID           string `json:"id"`
	NetworkID    string `json:"network_id"`
	IdentityMode string `json:"identity_mode"`
}
type Request struct {
	Sources []Source `json:"sources"`
	Exclude []string `json:"exclude"`
}
type Upload struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Row struct {
	ID      string         `json:"id"`
	Sources []string       `json:"sources"`
	Record  binding.Record `json:"record"`
	Status  string         `json:"status"`
	Reason  string         `json:"reason,omitempty"`
}
type Report struct {
	ID            string     `json:"id"`
	Revision      int64      `json:"revision"`
	ExpiresAt     time.Time  `json:"expires_at"`
	Rows          []Row      `json:"rows"`
	Errors        []string   `json:"errors"`
	Ready         bool       `json:"ready"`
	Added         int        `json:"added"`
	Duplicates    int        `json:"duplicates"`
	Conflicts     int        `json:"conflicts"`
	Excluded      int        `json:"excluded"`
	Backup        string     `json:"backup,omitempty"`
	CommittedAt   *time.Time `json:"committed_at,omitempty"`
	RestoredAt    *time.Time `json:"restored_at,omitempty"`
	RestoreBackup string     `json:"restore_backup,omitempty"`
}

type preview struct {
	report Report
	hashes map[string]string
	digest string
}
type Service struct {
	db         Database
	dir        string
	maxPlayers int
	networks   map[string]string
	mu         sync.Mutex
	previews   map[string]preview
}

func New(db Database, dir string, maxPlayers int, networks map[string]string) *Service {
	return &Service{db: db, dir: dir, maxPlayers: maxPlayers, networks: networks, previews: map[string]preview{}}
}

var sourceID = regexp.MustCompile(`^[a-f0-9]{32}$`)

func (s *Service) sourcePath(id string) (string, error) {
	if !sourceID.MatchString(id) {
		return "", fault.New("INVALID_INPUT", "导入文件标识无效")
	}
	return filepath.Join(s.dir, "imports", id+".sqlite"), nil
}

func (s *Service) Upload(ctx context.Context, reader io.Reader) (Upload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "imports"))
	if err != nil {
		return Upload{}, err
	}
	var size int64
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && info.Mode().IsRegular() {
			size += info.Size()
		}
	}
	if len(entries) >= 64 || size >= 512<<20 {
		return Upload{}, fault.New("CAPACITY_EXCEEDED", "导入暂存空间已满，请删除不再需要的源文件")
	}
	id := securefs.ID()
	path, _ := s.sourcePath(id)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Upload{}, err
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(reader, MaxSourceBytes+1))
	if err != nil {
		return Upload{}, err
	}
	if err := ctx.Err(); err != nil {
		return Upload{}, err
	}
	if n < 100 || n > MaxSourceBytes || size+n > 512<<20 {
		return Upload{}, fault.New("INVALID_INPUT", "源文件过小、超过 64 MiB 或超出暂存总量限制")
	}
	if err := f.Sync(); err != nil {
		return Upload{}, err
	}
	if err := f.Close(); err != nil {
		return Upload{}, err
	}
	ok = true
	return Upload{ID: id, SHA256: hex.EncodeToString(hash.Sum(nil)), Size: n}, nil
}

func (s *Service) DeleteSource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.sourcePath(id)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return fault.New("NOT_FOUND", "导入文件不存在")
	}
	return err
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxSourceBytes {
		return "", errors.New("invalid import source")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(f, MaxSourceBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readSource(ctx context.Context, path string, source Source) ([]Row, error) {
	db, err := sql.Open("sqlite", sqlite.DSN(path)+"?mode=ro&immutable=1&_pragma=query_only(1)&_pragma=busy_timeout(2000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var version int
	if err := db.QueryRowContext(ctx, "SELECT version FROM shitbot_schema WHERE schema_key='main'").Scan(&version); err != nil {
		return nil, errors.New("无法识别 ShitBot schema")
	}
	if version != 7 {
		return nil, fmt.Errorf("暂不支持 ShitBot schema %d，仅接受 schema 7 的一致性副本", version)
	}
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity); err != nil || integrity != "ok" {
		return nil, errors.New("源数据库损坏或不是一致性副本")
	}
	rows, err := db.QueryContext(ctx, "SELECT id,player_name,COALESCE(player_uuid,''),COALESCE(qq_id,''),created_at,updated_at FROM shitbot_bindings ORDER BY id LIMIT ?", maxRows+1)
	if err != nil {
		return nil, errors.New("ShitBot 白名单表结构无效")
	}
	defer rows.Close()
	result := []Row{}
	for rows.Next() {
		var id int64
		var name, uuid, qq string
		var created, updated int64
		if err := rows.Scan(&id, &name, &uuid, &qq, &created, &updated); err != nil {
			return nil, errors.New("白名单记录字段无效")
		}
		r := Row{ID: source.ID + ":" + fmt.Sprint(id), Sources: []string{source.ID}, Status: "add", Record: binding.Record{NetworkID: source.NetworkID, Player: bridge.Player{Name: name, UUID: strings.ToLower(uuid), IdentityMode: source.IdentityMode}, CreatedAt: time.UnixMilli(created).UTC(), UpdatedAt: time.UnixMilli(updated).UTC()}}
		if qq != "" {
			r.Record.Owner = &adapter.Identity{Platform: "qq.numeric", Scope: "global", ID: qq}
			if !regexp.MustCompile(`^[1-9][0-9]{4,19}$`).MatchString(qq) {
				r.Status = "conflict"
				r.Reason = "QQ 数字身份无效"
			}
		}
		if err := binding.ValidatePlayer(r.Record.Player); err != nil {
			r.Status = "conflict"
			r.Reason = err.Error()
		}
		if created < 0 || updated < created || updated > time.Now().Add(24*time.Hour).UnixMilli() {
			r.Status = "conflict"
			r.Reason = "源记录时间戳无效"
		}
		result = append(result, r)
		if len(result) > maxRows {
			return nil, errors.New("源文件超过 20000 条记录")
		}
	}
	return result, rows.Err()
}

func equivalent(a, b binding.Record) bool {
	if a.NetworkID != b.NetworkID || a.Player != b.Player {
		return false
	}
	if a.Owner == nil || b.Owner == nil {
		return a.Owner == nil && b.Owner == nil
	}
	return *a.Owner == *b.Owner
}

func (s *Service) Preview(ctx context.Context, request Request) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	for id, p := range s.previews {
		if now.After(p.report.ExpiresAt) {
			delete(s.previews, id)
		}
	}
	if len(s.previews) >= 16 {
		return Report{}, fault.New("BUSY", "待执行预览过多")
	}
	if len(request.Sources) == 0 || len(request.Sources) > 8 || len(request.Exclude) > maxRows {
		return Report{}, fault.New("INVALID_INPUT", "一次导入需要 1 至 8 个源文件")
	}
	p := preview{report: Report{ID: securefs.ID(), ExpiresAt: now.Add(15 * time.Minute), Rows: []Row{}, Errors: []string{}}, hashes: map[string]string{}}
	exclude := map[string]bool{}
	for _, id := range request.Exclude {
		exclude[id] = true
	}
	for _, source := range request.Sources {
		if mode, ok := s.networks[source.NetworkID]; !ok || mode != source.IdentityMode {
			return Report{}, fault.New("INVALID_INPUT", "导入范围或身份模式与目标网络不一致")
		}
		if _, exists := p.hashes[source.ID]; exists {
			return Report{}, fault.New("INVALID_INPUT", "重复的导入源")
		}
		path, err := s.sourcePath(source.ID)
		if err != nil {
			return Report{}, err
		}
		hash, err := hashFile(path)
		if err != nil {
			return Report{}, fault.New("INVALID_SOURCE", "导入源不可读取")
		}
		p.hashes[source.ID] = hash
		rows, err := readSource(ctx, path, source)
		if err != nil {
			p.report.Errors = append(p.report.Errors, source.ID+": "+err.Error())
			continue
		}
		for i := range rows {
			if exclude[rows[i].ID] {
				rows[i].Status = "excluded"
				rows[i].Reason = "用户排除"
				delete(exclude, rows[i].ID)
			}
		}
		p.report.Rows = append(p.report.Rows, rows...)
		if len(p.report.Rows) > maxRows {
			return Report{}, fault.New("CAPACITY_EXCEEDED", "合并后的记录数超过 20000")
		}
	}
	if len(exclude) > 0 {
		return Report{}, fault.New("INVALID_INPUT", "排除列表包含未知记录，请重新预览")
	}
	err := s.db.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		p.report.Revision, err = storage.Revision(ctx, tx)
		if err != nil {
			return err
		}
		names := map[string]int{}
		foldedNames := map[string]int{}
		uuids := map[string]int{}
		counts := map[string]int{}
		for i := range p.report.Rows {
			r := &p.report.Rows[i]
			if r.Status != "add" {
				continue
			}
			record := r.Record
			key := record.NetworkID + "\x1f" + record.Player.Name
			if old, ok := names[key]; ok {
				previous := &p.report.Rows[old]
				if equivalent(previous.Record, record) && previous.Status != "conflict" {
					r.Status = "duplicate"
					previous.Sources = append(previous.Sources, r.Sources...)
				} else {
					r.Status = "conflict"
					r.Reason = "多个来源对同一玩家的身份或绑定不一致"
					previous.Status = "conflict"
					previous.Reason = r.Reason
				}
				continue
			}
			names[key] = i
			foldedKey := record.NetworkID + "\x1f" + strings.ToLower(record.Player.Name)
			if old, exists := foldedNames[foldedKey]; exists && p.report.Rows[old].Record.Player.Name != record.Player.Name {
				r.Status = "conflict"
				r.Reason = "多个来源的玩家名仅大小写不同，请明确排除或映射"
				p.report.Rows[old].Status = "conflict"
				p.report.Rows[old].Reason = r.Reason
				continue
			}
			foldedNames[foldedKey] = i
			if record.Player.UUID != "" {
				uuidKey := record.NetworkID + "\x1f" + record.Player.UUID
				if old, ok := uuids[uuidKey]; ok {
					r.Status = "conflict"
					r.Reason = "同一 UUID 对应不同玩家名"
					p.report.Rows[old].Status = "conflict"
					p.report.Rows[old].Reason = r.Reason
					continue
				}
				uuids[uuidKey] = i
			}
			existing, err := binding.Find(ctx, tx, record.NetworkID, record.Player.Name)
			if err == nil {
				if equivalent(existing, record) {
					r.Status = "duplicate"
				} else {
					r.Status = "conflict"
					r.Reason = "目标库已有不同身份、绑定或无账号白名单"
				}
				continue
			}
			if !fault.Is(err, "NOT_FOUND") {
				return err
			}
			// Names remain case-sensitive. Ambiguous legacy case mappings require
			// an explicit exclusion instead of silently creating another account.
			var ambiguous int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM bindings WHERE network_id=? AND (LOWER(player_name)=LOWER(?) OR (player_uuid<>'' AND player_uuid=?))", record.NetworkID, record.Player.Name, record.Player.UUID).Scan(&ambiguous); err != nil {
				return err
			}
			if ambiguous > 0 {
				r.Status = "conflict"
				r.Reason = "目标库存在大小写或 UUID 身份歧义"
				continue
			}
			if record.Owner != nil {
				ownerKey := record.NetworkID + "\x1f" + record.Owner.Key()
				count, ok := counts[ownerKey]
				if !ok {
					if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM bindings WHERE network_id=? AND platform=? AND scope=? AND user_id=?", record.NetworkID, record.Owner.Platform, record.Owner.Scope, record.Owner.ID).Scan(&count); err != nil {
						return err
					}
				}
				count++
				counts[ownerKey] = count
				if count > s.maxPlayers {
					r.Status = "conflict"
					r.Reason = "合并后超过账号绑定上限"
				}
			}
		}
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	for _, r := range p.report.Rows {
		switch r.Status {
		case "add":
			p.report.Added++
		case "duplicate":
			p.report.Duplicates++
		case "conflict":
			p.report.Conflicts++
		case "excluded":
			p.report.Excluded++
		}
	}
	p.report.Ready = p.report.Conflicts == 0 && len(p.report.Errors) == 0
	data, _ := json.Marshal(struct {
		Request Request
		Hashes  map[string]string
	}{request, p.hashes})
	sum := sha256.Sum256(data)
	p.digest = hex.EncodeToString(sum[:])
	s.previews[p.report.ID] = p
	return p.report, nil
}

func (s *Service) Commit(ctx context.Context, id string) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A repeated execute request returns its durable report, even after restart.
	var saved []byte
	err := s.db.QueryRowContext(ctx, "SELECT report FROM import_jobs WHERE id=?", id).Scan(&saved)
	if err == nil {
		var report Report
		err = json.Unmarshal(saved, &report)
		return report, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Report{}, err
	}
	p, ok := s.previews[id]
	if !ok || time.Now().After(p.report.ExpiresAt) {
		return Report{}, fault.New("PREVIEW_EXPIRED", "预览已过期，请重新生成")
	}
	if !p.report.Ready {
		return Report{}, fault.New("IMPORT_CONFLICT", "请排除或处理冲突后重新预览")
	}
	for source, hash := range p.hashes {
		path, _ := s.sourcePath(source)
		current, err := hashFile(path)
		if err != nil || current != hash {
			return Report{}, fault.New("REVISION_CONFLICT", "源文件已变化，请重新预览")
		}
	}
	revision, err := storage.Revision(ctx, s.db)
	if err != nil {
		return Report{}, err
	}
	if revision != p.report.Revision {
		return Report{}, fault.New("REVISION_CONFLICT", "目标绑定已变化，请重新预览")
	}
	backup := filepath.Join(s.dir, "backups", "before-import-"+id+"-"+securefs.ID()+".sqlite")
	if err := s.db.Backup(ctx, backup); err != nil {
		return Report{}, err
	}
	p.report.Backup = filepath.Base(backup)
	now := time.Now().UTC()
	p.report.CommittedAt = &now
	err = s.db.Transaction(ctx, func(tx *sql.Tx) error {
		revision, err := storage.Revision(ctx, tx)
		if err != nil {
			return err
		}
		if revision != p.report.Revision {
			return fault.New("REVISION_CONFLICT", "目标绑定已变化，请重新预览")
		}
		for _, row := range p.report.Rows {
			if row.Status == "add" {
				if err := binding.Insert(ctx, tx, row.Record, s.maxPlayers); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, "DELETE FROM bind_codes WHERE network_id=? AND player_name=?", row.Record.NetworkID, row.Record.Player.Name); err != nil {
					return err
				}
			}
		}
		if p.report.Added > 0 {
			if err := storage.BumpRevision(ctx, tx); err != nil {
				return err
			}
		}
		data, err := json.Marshal(p.report)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO import_jobs(id,digest,report,created_at) VALUES(?,?,?,?)", id, p.digest, data, now.UnixMilli())
		return err
	})
	if err != nil {
		return Report{}, err
	}
	delete(s.previews, id)
	return p.report, nil
}

// Restore only removes rows introduced by this job which still match their
// recorded identity and timestamps. Any subsequent edit aborts the whole undo.
func (s *Service) Restore(ctx context.Context, id string) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT report FROM import_jobs WHERE id=?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, fault.New("NOT_FOUND", "导入任务不存在")
	}
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(data, &report); err != nil {
		return Report{}, err
	}
	if report.RestoredAt != nil {
		return report, nil
	}
	backup := filepath.Join(s.dir, "backups", "before-restore-"+id+"-"+securefs.ID()+".sqlite")
	if err := s.db.Backup(ctx, backup); err != nil {
		return Report{}, err
	}
	now := time.Now().UTC()
	report.RestoredAt = &now
	report.RestoreBackup = filepath.Base(backup)
	err = s.db.Transaction(ctx, func(tx *sql.Tx) error {
		changed := false
		for _, row := range report.Rows {
			if row.Status != "add" {
				continue
			}
			current, err := binding.Find(ctx, tx, row.Record.NetworkID, row.Record.Player.Name)
			if fault.Is(err, "NOT_FOUND") {
				continue
			}
			if err != nil {
				return err
			}
			if !equivalent(current, row.Record) || !current.CreatedAt.Equal(row.Record.CreatedAt) || !current.UpdatedAt.Equal(row.Record.UpdatedAt) {
				return fault.New("REVISION_CONFLICT", "导入记录已被后续业务修改，整批恢复已取消")
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM bindings WHERE network_id=? AND player_name=?", current.NetworkID, current.Player.Name); err != nil {
				return err
			}
			changed = true
		}
		if changed {
			if err := storage.BumpRevision(ctx, tx); err != nil {
				return err
			}
		}
		data, err := json.Marshal(report)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE import_jobs SET report=? WHERE id=?", data, id)
		return err
	})
	if err != nil {
		return Report{}, err
	}
	return report, nil
}

func (s *Service) Report(ctx context.Context, id string) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.previews[id]; ok {
		return p.report, nil
	}
	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT report FROM import_jobs WHERE id=?", id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Report{}, fault.New("NOT_FOUND", "导入报告不存在")
	}
	if err != nil {
		return Report{}, err
	}
	var report Report
	err = json.Unmarshal(data, &report)
	return report, err
}

func (s *Service) Sources() ([]Upload, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.dir, "imports"))
	if err != nil {
		return nil, err
	}
	result := []Upload{}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".sqlite")
		if !sourceID.MatchString(id) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		path, _ := s.sourcePath(id)
		hash, err := hashFile(path)
		if err != nil {
			return nil, err
		}
		result = append(result, Upload{ID: id, SHA256: hash, Size: info.Size()})
	}
	slices.SortFunc(result, func(a, b Upload) int { return strings.Compare(a.ID, b.ID) })
	return result, nil
}

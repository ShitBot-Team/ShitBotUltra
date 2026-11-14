package binding

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/securefs"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

type Record struct {
	NetworkID string            `json:"network_id"`
	Player    bridge.Player     `json:"player"`
	Owner     *adapter.Identity `json:"owner,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type cachedCode struct {
	player  bridge.Player
	code    string
	expires time.Time
}
type Service struct {
	db       storage.Database
	settings config.Binding
	mu       sync.Mutex
	codes    map[string]cachedCode
}

func New(db storage.Database, settings config.Binding) *Service {
	return &Service{db: db, settings: settings, codes: map[string]cachedCode{}}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ValidatePlayer(p bridge.Player) error {
	if !utf8.ValidString(p.Name) || strings.TrimSpace(p.Name) != p.Name || p.Name == "" || utf8.RuneCountInString(p.Name) > 64 || strings.ContainsFunc(p.Name, unicode.IsControl) {
		return fault.New("INVALID_PLAYER", "玩家名为空、过长或含控制字符")
	}
	if p.IdentityMode != "online" && p.IdentityMode != "forwarded" && p.IdentityMode != "offline" {
		return fault.New("INVALID_PLAYER", "玩家身份模式无效")
	}
	if p.UUID != "" && !uuidPattern.MatchString(p.UUID) {
		return fault.New("INVALID_PLAYER", "UUID 格式无效")
	}
	return nil
}

func normalize(p bridge.Player) bridge.Player { p.UUID = strings.ToLower(p.UUID); return p }

const columns = "network_id, player_name, player_uuid, identity_mode, platform, scope, user_id, created_at, updated_at"

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Record, error) {
	var r Record
	var owner adapter.Identity
	var created, updated int64
	err := row.Scan(&r.NetworkID, &r.Player.Name, &r.Player.UUID, &r.Player.IdentityMode, &owner.Platform, &owner.Scope, &owner.ID, &created, &updated)
	if owner.ID != "" {
		r.Owner = &owner
	}
	r.CreatedAt = time.UnixMilli(created).UTC()
	r.UpdatedAt = time.UnixMilli(updated).UTC()
	return r, err
}

func Find(ctx context.Context, q storage.Querier, network, name string) (Record, error) {
	r, err := scan(q.QueryRowContext(ctx, "SELECT "+columns+" FROM bindings WHERE network_id=? AND player_name=?", network, name))
	if errors.Is(err, sql.ErrNoRows) {
		err = fault.New("NOT_FOUND", "玩家不在白名单中")
	}
	return r, err
}

func (s *Service) Find(ctx context.Context, network, name string) (Record, error) {
	return Find(ctx, s.db, network, name)
}

func List(ctx context.Context, q storage.Querier, network string, owner *adapter.Identity, limit, offset int) ([]Record, error) {
	if limit < 1 || limit > 1000 || offset < 0 {
		return nil, fault.New("INVALID_INPUT", "分页参数无效")
	}
	query := "SELECT " + columns + " FROM bindings WHERE network_id=?"
	args := []any{network}
	if owner != nil {
		if err := owner.Validate(); err != nil {
			return nil, err
		}
		query += " AND platform=? AND scope=? AND user_id=?"
		args = append(args, owner.Platform, owner.Scope, owner.ID)
	}
	query += " ORDER BY player_name LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []Record{}
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *Service) List(ctx context.Context, network string, owner *adapter.Identity, limit, offset int) ([]Record, error) {
	return List(ctx, s.db, network, owner, limit, offset)
}

// Insert is shared by administrator changes and the transactional import writer.
// It never replaces a binding, demotes a bound player, or merges distinct identities.
func Insert(ctx context.Context, q storage.Querier, r Record, maxPlayers int) error {
	if !adapter.ValidID(r.NetworkID) {
		return fault.New("INVALID_INPUT", "网络标识无效")
	}
	if err := ValidatePlayer(r.Player); err != nil {
		return err
	}
	r.Player = normalize(r.Player)
	if _, err := Find(ctx, q, r.NetworkID, r.Player.Name); err == nil {
		return fault.New("ALREADY_BOUND", "玩家已有白名单或绑定")
	} else if !fault.Is(err, "NOT_FOUND") {
		return err
	}
	if r.Player.UUID != "" && r.Player.IdentityMode != "offline" {
		var name string
		err := q.QueryRowContext(ctx, "SELECT player_name FROM bindings WHERE network_id=? AND player_uuid=? AND identity_mode<>'offline'", r.NetworkID, r.Player.UUID).Scan(&name)
		if err == nil {
			return fault.New("IDENTITY_CONFLICT", "UUID 已用于其他玩家名，请明确处理身份冲突")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	var owner adapter.Identity
	if r.Owner != nil {
		owner = *r.Owner
		if err := owner.Validate(); err != nil {
			return fault.New("INVALID_INPUT", "账号身份无效")
		}
		var count int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM bindings WHERE network_id=? AND platform=? AND scope=? AND user_id=?", r.NetworkID, owner.Platform, owner.Scope, owner.ID).Scan(&count); err != nil {
			return err
		}
		if count >= maxPlayers {
			return fault.New("BINDING_LIMIT", "账号绑定数量已达到上限")
		}
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = r.CreatedAt
	}
	_, err := q.ExecContext(ctx, "INSERT INTO bindings("+columns+") VALUES(?,?,?,?,?,?,?,?,?)", r.NetworkID, r.Player.Name, r.Player.UUID, r.Player.IdentityMode, owner.Platform, owner.Scope, owner.ID, r.CreatedAt.UnixMilli(), r.UpdatedAt.UnixMilli())
	return err
}

func (s *Service) Add(ctx context.Context, r Record) error {
	return s.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := Insert(ctx, tx, r, s.settings.MaxPlayersPerIdentity); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM bind_codes WHERE network_id=? AND player_name=?", r.NetworkID, r.Player.Name); err != nil {
			return err
		}
		return storage.BumpRevision(ctx, tx)
	})
}

func (s *Service) Remove(ctx context.Context, network, name string, owner *adapter.Identity) (Record, error) {
	var removed Record
	err := s.db.Transaction(ctx, func(tx *sql.Tx) error {
		var err error
		removed, err = Find(ctx, tx, network, name)
		if err != nil {
			return err
		}
		if owner != nil && (removed.Owner == nil || *owner != *removed.Owner) {
			return fault.New("FORBIDDEN", "不能解绑其他账号的玩家")
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM bindings WHERE network_id=? AND player_name=?", network, name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM bind_codes WHERE network_id=? AND player_name=?", network, name); err != nil {
			return err
		}
		return storage.BumpRevision(ctx, tx)
	})
	return removed, err
}

func (s *Service) Authorize(ctx context.Context, node config.Node, player bridge.Player) (bridge.LoginDecision, error) {
	if err := ValidatePlayer(player); err != nil {
		return bridge.LoginDecision{}, err
	}
	player = normalize(player)
	if player.IdentityMode != node.IdentityMode || (player.IdentityMode != "offline" && player.UUID == "") {
		return bridge.LoginDecision{}, fault.New("IDENTITY_CONFLICT", "玩家身份与节点配置不匹配")
	}
	if !s.settings.Enabled || !node.LoginGate {
		return bridge.LoginDecision{Allowed: true}, nil
	}
	var decision bridge.LoginDecision
	err := s.db.Transaction(ctx, func(tx *sql.Tx) error {
		r, err := Find(ctx, tx, node.NetworkID, player.Name)
		if fault.Is(err, "NOT_FOUND") {
			return nil
		}
		if err != nil {
			return err
		}
		if r.Player.IdentityMode != player.IdentityMode || (r.Player.UUID != "" && r.Player.UUID != player.UUID) {
			return fault.New("IDENTITY_CONFLICT", "玩家身份与已有白名单不一致")
		}
		if r.Player.UUID == "" && player.UUID != "" {
			var count int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM bindings WHERE network_id=? AND player_uuid=? AND identity_mode<>'offline'", node.NetworkID, player.UUID).Scan(&count); err != nil {
				return err
			}
			if count != 0 && player.IdentityMode != "offline" {
				return fault.New("IDENTITY_CONFLICT", "UUID 已绑定其他玩家名")
			}
			if _, err := tx.ExecContext(ctx, "UPDATE bindings SET player_uuid=?, updated_at=? WHERE network_id=? AND player_name=?", player.UUID, time.Now().UnixMilli(), node.NetworkID, player.Name); err != nil {
				return err
			}
			if err := storage.BumpRevision(ctx, tx); err != nil {
				return err
			}
		}
		decision.Allowed = true
		return nil
	})
	if err != nil || decision.Allowed {
		return decision, err
	}
	return s.issue(ctx, node.NetworkID, player)
}

func digest(salt, code string) []byte { sum := sha256.Sum256([]byte(salt + ":" + code)); return sum[:] }

func (s *Service) issue(ctx context.Context, network string, player bridge.Player) (bridge.LoginDecision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	key := network + "\x1f" + player.Name
	for key, c := range s.codes {
		if !now.Before(c.expires) {
			delete(s.codes, key)
		}
	}
	if c, ok := s.codes[key]; ok && c.player == player {
		var remaining int
		if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM bind_codes WHERE network_id=? AND player_name=? AND expires_at>? AND attempts<?", network, player.Name, now.UnixMilli(), s.settings.MaxAttempts).Scan(&remaining); err != nil {
			return bridge.LoginDecision{}, err
		}
		if remaining == 1 {
			return denied(c), nil
		}
	}
	if len(s.codes) >= 10000 {
		return bridge.LoginDecision{}, fault.New("BUSY", "验证码请求过多，请稍后再试")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(100000000))
	if err != nil {
		return bridge.LoginDecision{}, err
	}
	c := cachedCode{player: player, code: fmt.Sprintf("%08d", n), expires: now.Add(time.Duration(s.settings.CodeTTLSeconds) * time.Second)}
	salt := securefs.ID()
	err = s.db.Transaction(ctx, func(tx *sql.Tx) error {
		// Recheck after the initial whitelist lookup: a concurrent administrator or
		// redemption may have added this player before code issuance acquired the writer.
		if _, err := Find(ctx, tx, network, player.Name); err == nil {
			return fault.New("RETRY_LOGIN", "白名单已更新，请重新连接")
		} else if !fault.Is(err, "NOT_FOUND") {
			return err
		}
		var attempts int
		var expires int64
		err := tx.QueryRowContext(ctx, "SELECT attempts, expires_at FROM bind_codes WHERE network_id=? AND player_name=?", network, player.Name).Scan(&attempts, &expires)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if expires > now.UnixMilli() && attempts >= s.settings.MaxAttempts {
			return fault.New("RATE_LIMITED", "验证尝试过多，请等待验证码过期")
		}
		if expires <= now.UnixMilli() {
			attempts = 0
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO bind_codes(network_id,player_name,player_uuid,identity_mode,code_hash,salt,expires_at,attempts) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(network_id,player_name) DO UPDATE SET player_uuid=excluded.player_uuid,identity_mode=excluded.identity_mode,code_hash=excluded.code_hash,salt=excluded.salt,expires_at=excluded.expires_at,attempts=excluded.attempts`, network, player.Name, player.UUID, player.IdentityMode, digest(salt, c.code), salt, c.expires.UnixMilli(), attempts)
		return err
	})
	if err != nil {
		return bridge.LoginDecision{}, err
	}
	s.codes[key] = c
	return denied(c), nil
}

func denied(c cachedCode) bridge.LoginDecision {
	return bridge.LoginDecision{Reason: "请在已配置的机器人会话中发送 /bind \"玩家名\" 验证码", Code: c.code, ExpiresAt: &c.expires}
}

func (s *Service) Redeem(ctx context.Context, network, name, code string, owner adapter.Identity) (Record, error) {
	if err := owner.Validate(); err != nil {
		return Record{}, fault.New("INVALID_INPUT", "账号身份无效")
	}
	if name == "" || len(name) > 256 || len(code) > 64 {
		return Record{}, fault.New("INVALID_INPUT", "玩家名或验证码无效")
	}
	now := time.Now().UTC()
	var result Record
	var rejected error
	err := s.db.Transaction(ctx, func(tx *sql.Tx) error {
		// Rejected attempts still commit; returning the business error from this
		// callback would roll the throttle back along with the attempted binding.
		_, err := tx.ExecContext(ctx, `INSERT INTO bind_attempts(network_id,actor_key,attempts,expires_at) VALUES(?,?,1,?)
		ON CONFLICT(network_id,actor_key) DO UPDATE SET attempts=CASE WHEN expires_at<=? THEN 1 ELSE attempts+1 END,expires_at=CASE WHEN expires_at<=? THEN excluded.expires_at ELSE expires_at END`, network, owner.Key(), now.Add(time.Duration(s.settings.AttemptWindowSeconds)*time.Second).UnixMilli(), now.UnixMilli(), now.UnixMilli())
		if err != nil {
			return err
		}
		var attempts int
		if err := tx.QueryRowContext(ctx, "SELECT attempts FROM bind_attempts WHERE network_id=? AND actor_key=?", network, owner.Key()).Scan(&attempts); err != nil {
			return err
		}
		if attempts > s.settings.MaxAttempts {
			rejected = fault.New("RATE_LIMITED", "验证尝试过多，请稍后再试")
			return nil
		}
		var p bridge.Player
		p.Name = name
		var hash []byte
		var salt string
		var expires int64
		var failures int
		err = tx.QueryRowContext(ctx, "SELECT player_uuid,identity_mode,code_hash,salt,expires_at,attempts FROM bind_codes WHERE network_id=? AND player_name=?", network, name).Scan(&p.UUID, &p.IdentityMode, &hash, &salt, &expires, &failures)
		if errors.Is(err, sql.ErrNoRows) {
			rejected = fault.New("INVALID_CODE", "验证码无效或已过期")
			return nil
		}
		if err != nil {
			return err
		}
		if expires <= now.UnixMilli() || failures >= s.settings.MaxAttempts {
			rejected = fault.New("INVALID_CODE", "验证码无效或已过期")
			return nil
		}
		if subtle.ConstantTimeCompare(hash, digest(salt, code)) != 1 {
			_, err := tx.ExecContext(ctx, "UPDATE bind_codes SET attempts=attempts+1 WHERE network_id=? AND player_name=?", network, name)
			rejected = fault.New("INVALID_CODE", "验证码无效或已过期")
			return err
		}
		result = Record{NetworkID: network, Player: p, Owner: &owner, CreatedAt: now, UpdatedAt: now}
		if err := Insert(ctx, tx, result, s.settings.MaxPlayersPerIdentity); err != nil {
			if e := fault.Public(err); e.Code != "INTERNAL" {
				rejected = err
				return nil
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM bind_codes WHERE network_id=? AND player_name=?", network, name); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM bind_attempts WHERE network_id=? AND actor_key=?", network, owner.Key()); err != nil {
			return err
		}
		return storage.BumpRevision(ctx, tx)
	})
	if err != nil {
		return Record{}, err
	}
	if rejected != nil {
		return Record{}, rejected
	}
	s.mu.Lock()
	delete(s.codes, network+"\x1f"+name)
	s.mu.Unlock()
	return result, nil
}

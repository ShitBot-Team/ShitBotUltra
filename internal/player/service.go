package player

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/binding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
)

type Service struct{ db storage.Database }

func New(db storage.Database) *Service { return &Service{db: db} }

type Profile struct {
	NetworkID    string        `json:"network_id"`
	Player       bridge.Player `json:"player"`
	TotalSeconds int64         `json:"total_seconds"`
	Online       bool          `json:"online"`
	LastSeenAt   time.Time     `json:"last_seen_at"`
	NodeID       string        `json:"node_id"`
}

// Recover closes sessions at their last persisted heartbeat, excluding downtime.
func (s *Service) Recover(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE player_stats SET total_seconds=total_seconds+MAX(0,(last_seen_at-session_started_at)/1000),session_started_at=NULL WHERE session_started_at IS NOT NULL`)
	return err
}

func (s *Service) Apply(ctx context.Context, node config.Node, e bridge.Envelope) error {
	if !node.SessionAuthority {
		return fault.New("FORBIDDEN", "此节点不是网络玩家会话的权威来源")
	}
	players := []bridge.Player{}
	switch e.Type {
	case "players.snapshot":
		var snapshot bridge.OnlineSnapshot
		if err := json.Unmarshal(e.Payload, &snapshot); err != nil {
			return fault.New("INVALID_INPUT", "在线快照格式无效")
		}
		players = snapshot.Players
		if len(players) > 10000 {
			return fault.New("INVALID_INPUT", "在线快照过大")
		}
	case "player.joined", "player.left":
		var event bridge.PlayerEvent
		if err := json.Unmarshal(e.Payload, &event); err != nil {
			return fault.New("INVALID_INPUT", "玩家事件格式无效")
		}
		players = append(players, event.Player)
	default:
		return fault.New("UNSUPPORTED_EVENT", "未知玩家事件")
	}
	seen := map[string]bool{}
	for i, p := range players {
		if err := binding.ValidatePlayer(p); err != nil {
			return err
		}
		if p.IdentityMode != node.IdentityMode || (p.IdentityMode != "offline" && p.UUID == "") || seen[p.Name] {
			return fault.New("IDENTITY_CONFLICT", "快照身份不一致或玩家重复")
		}
		seen[p.Name] = true
		players[i].UUID = strings.ToLower(p.UUID)
	}
	now := time.Now().UnixMilli()
	return s.db.Transaction(ctx, func(tx *sql.Tx) error {
		claimed, err := storage.Claim(ctx, tx, "bridge:"+node.ID+":"+e.BootID, e.MessageID)
		if err != nil || !claimed {
			return err
		}
		if e.Type == "players.snapshot" {
			rows, err := tx.QueryContext(ctx, "SELECT player_name,boot_id FROM player_stats WHERE network_id=? AND session_started_at IS NOT NULL", node.NetworkID)
			if err != nil {
				return err
			}
			stale := []string{}
			for rows.Next() {
				var name, boot string
				if err := rows.Scan(&name, &boot); err != nil {
					rows.Close()
					return err
				}
				if !seen[name] || boot != e.BootID {
					stale = append(stale, name)
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
			for _, name := range stale {
				if _, err := tx.ExecContext(ctx, `UPDATE player_stats SET total_seconds=total_seconds+MAX(0,(last_seen_at-session_started_at)/1000),session_started_at=NULL WHERE network_id=? AND player_name=?`, node.NetworkID, name); err != nil {
					return err
				}
			}
		}
		for _, p := range players {
			if e.Type == "player.left" {
				_, err = tx.ExecContext(ctx, `UPDATE player_stats SET total_seconds=total_seconds+MAX(0,(?-session_started_at)/1000),session_started_at=NULL,last_seen_at=? WHERE network_id=? AND player_name=? AND node_id=? AND boot_id=? AND session_started_at IS NOT NULL`, now, now, node.NetworkID, p.Name, node.ID, e.BootID)
			} else {
				_, err = tx.ExecContext(ctx, `INSERT INTO player_stats(network_id,player_name,player_uuid,identity_mode,total_seconds,session_started_at,last_seen_at,node_id,boot_id) VALUES(?,?,?,?,0,?,?,?,?)
				ON CONFLICT(network_id,player_name) DO UPDATE SET player_uuid=excluded.player_uuid,identity_mode=excluded.identity_mode,session_started_at=COALESCE(player_stats.session_started_at,excluded.session_started_at),last_seen_at=excluded.last_seen_at,node_id=excluded.node_id,boot_id=excluded.boot_id`, node.NetworkID, p.Name, p.UUID, p.IdentityMode, now, now, node.ID, e.BootID)
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) Heartbeat(ctx context.Context, nodeID, bootID string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE player_stats SET last_seen_at=? WHERE node_id=? AND boot_id=? AND session_started_at IS NOT NULL", time.Now().UnixMilli(), nodeID, bootID)
	return err
}
func (s *Service) Disconnect(ctx context.Context, nodeID, bootID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE player_stats SET total_seconds=total_seconds+MAX(0,(last_seen_at-session_started_at)/1000),session_started_at=NULL WHERE node_id=? AND boot_id=? AND session_started_at IS NOT NULL`, nodeID, bootID)
	return err
}

func (s *Service) List(ctx context.Context, network string, onlineOnly bool, limit, offset int) ([]Profile, error) {
	if limit < 1 || limit > 1000 || offset < 0 {
		return nil, fault.New("INVALID_INPUT", "分页参数无效")
	}
	query := "SELECT network_id,player_name,player_uuid,identity_mode,total_seconds,session_started_at,last_seen_at,node_id FROM player_stats WHERE network_id=?"
	if onlineOnly {
		query += " AND session_started_at IS NOT NULL"
	}
	query += " ORDER BY player_name LIMIT ? OFFSET ?"
	rows, err := s.db.QueryContext(ctx, query, network, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Profile{}
	now := time.Now().UnixMilli()
	for rows.Next() {
		var p Profile
		var started sql.NullInt64
		var seen int64
		if err := rows.Scan(&p.NetworkID, &p.Player.Name, &p.Player.UUID, &p.Player.IdentityMode, &p.TotalSeconds, &started, &seen, &p.NodeID); err != nil {
			return nil, err
		}
		p.Online = started.Valid
		if started.Valid && now > started.Int64 {
			p.TotalSeconds += (now - started.Int64) / 1000
		}
		p.LastSeenAt = time.UnixMilli(seen).UTC()
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Service) Get(ctx context.Context, network, name string) (Profile, error) {
	var profile Profile
	var started sql.NullInt64
	var seen int64
	err := s.db.QueryRowContext(ctx, "SELECT network_id,player_name,player_uuid,identity_mode,total_seconds,session_started_at,last_seen_at,node_id FROM player_stats WHERE network_id=? AND player_name=?", network, name).Scan(&profile.NetworkID, &profile.Player.Name, &profile.Player.UUID, &profile.Player.IdentityMode, &profile.TotalSeconds, &started, &seen, &profile.NodeID)
	if err == sql.ErrNoRows {
		return profile, fault.New("NOT_FOUND", "尚无此玩家的游戏会话记录")
	}
	if err != nil {
		return profile, err
	}
	profile.Online = started.Valid
	if started.Valid && time.Now().UnixMilli() > started.Int64 {
		profile.TotalSeconds += (time.Now().UnixMilli() - started.Int64) / 1000
	}
	profile.LastSeenAt = time.UnixMilli(seen).UTC()
	return profile, nil
}

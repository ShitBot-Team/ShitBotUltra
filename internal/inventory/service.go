package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	bridge "github.com/ShitBot-Team/ShitBotUltra/api/bridge/v1"
	"github.com/ShitBot-Team/ShitBotUltra/internal/binding"
	"github.com/ShitBot-Team/ShitBotUltra/internal/config"
	"github.com/ShitBot-Team/ShitBotUltra/internal/fault"
	"github.com/ShitBot-Team/ShitBotUltra/internal/storage"
	"github.com/ShitBot-Team/ShitBotUltra/sdk/adapter"
)

type Service struct {
	db        storage.Database
	retention time.Duration
}

func New(db storage.Database, days int) *Service {
	return &Service{db: db, retention: time.Duration(days) * 24 * time.Hour}
}

func (s *Service) Save(ctx context.Context, node config.Node, snapshot bridge.InventorySnapshot) error {
	if err := binding.ValidatePlayer(snapshot.Player); err != nil {
		return err
	}
	if snapshot.Player.IdentityMode != node.IdentityMode || (snapshot.Player.IdentityMode != "offline" && snapshot.Player.UUID == "") || snapshot.FormatVersion != 1 || len(snapshot.Data) == 0 || len(snapshot.Data) > 2<<20 || !json.Valid(snapshot.Data) {
		return fault.New("INVALID_INPUT", "背包快照格式、大小或身份无效")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(snapshot.Data, &object); err != nil || object == nil {
		return fault.New("INVALID_INPUT", "背包数据必须为 JSON 对象")
	}
	now := time.Now()
	if snapshot.CapturedAt.After(now.Add(30*time.Second)) || snapshot.CapturedAt.Before(now.Add(-s.retention)) {
		return fault.New("INVALID_INPUT", "背包快照已过期或来自未来")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO inventory_snapshots(network_id,server_id,player_name,player_uuid,identity_mode,captured_at,format_version,payload) VALUES(?,?,?,?,?,?,?,?)
	ON CONFLICT(network_id,server_id,player_name) DO UPDATE SET player_uuid=excluded.player_uuid,identity_mode=excluded.identity_mode,captured_at=excluded.captured_at,format_version=excluded.format_version,payload=excluded.payload WHERE excluded.captured_at>inventory_snapshots.captured_at`, node.NetworkID, node.ServerID, snapshot.Player.Name, strings.ToLower(snapshot.Player.UUID), snapshot.Player.IdentityMode, snapshot.CapturedAt.UnixMilli(), snapshot.FormatVersion, []byte(snapshot.Data))
	return err
}

func (s *Service) Get(ctx context.Context, network, server, name string, owner *adapter.Identity) (bridge.InventorySnapshot, error) {
	var snapshot bridge.InventorySnapshot
	var at int64
	var payload []byte
	err := s.db.Transaction(ctx, func(tx *sql.Tx) error {
		var bound binding.Record
		if owner != nil {
			var err error
			bound, err = binding.Find(ctx, tx, network, name)
			if err != nil {
				return err
			}
			if bound.Owner == nil || *bound.Owner != *owner {
				return fault.New("FORBIDDEN", "背包只允许绑定账号查看")
			}
		}
		err := tx.QueryRowContext(ctx, "SELECT player_name,player_uuid,identity_mode,captured_at,format_version,payload FROM inventory_snapshots WHERE network_id=? AND server_id=? AND player_name=? AND captured_at>=?", network, server, name, time.Now().Add(-s.retention).UnixMilli()).Scan(&snapshot.Player.Name, &snapshot.Player.UUID, &snapshot.Player.IdentityMode, &at, &snapshot.FormatVersion, &payload)
		if errors.Is(err, sql.ErrNoRows) {
			return fault.New("NOT_FOUND", "没有可用的背包快照")
		}
		if err != nil {
			return err
		}
		if owner != nil && (bound.Player.IdentityMode != snapshot.Player.IdentityMode || (bound.Player.UUID != "" && bound.Player.UUID != snapshot.Player.UUID)) {
			return fault.New("IDENTITY_CONFLICT", "背包快照与当前绑定身份不一致")
		}
		return nil
	})
	snapshot.CapturedAt = time.UnixMilli(at).UTC()
	snapshot.Data = json.RawMessage(payload)
	return snapshot, err
}

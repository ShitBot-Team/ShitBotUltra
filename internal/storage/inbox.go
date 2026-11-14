package storage

import (
	"context"
	"time"
)

// Claim must run in the same transaction as database-only effects. For external
// chat/actions it is an at-most-once attempt, not an exactly-once delivery promise.
func Claim(ctx context.Context, q Querier, source, id string) (bool, error) {
	result, err := q.ExecContext(ctx, "INSERT INTO consumed_events(source,event_id,expires_at) VALUES(?,?,?) ON CONFLICT DO NOTHING", source, id, time.Now().Add(24*time.Hour).UnixMilli())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func Cleanup(ctx context.Context, q Querier, inventoryDays int) error {
	now := time.Now()
	for _, stmt := range []struct {
		query  string
		before int64
	}{
		{"DELETE FROM consumed_events WHERE expires_at<?", now.UnixMilli()},
		{"DELETE FROM bind_codes WHERE expires_at<?", now.UnixMilli()},
		{"DELETE FROM bind_attempts WHERE expires_at<?", now.UnixMilli()},
		{"DELETE FROM bridge_cursors WHERE updated_at<?", now.Add(-24 * time.Hour).UnixMilli()},
		{"DELETE FROM inventory_snapshots WHERE captured_at<?", now.Add(-time.Duration(inventoryDays) * 24 * time.Hour).UnixMilli()},
	} {
		if _, err := q.ExecContext(ctx, stmt.query, stmt.before); err != nil {
			return err
		}
	}
	return nil
}

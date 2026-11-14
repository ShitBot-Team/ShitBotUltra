package storage

import (
	"context"
	"database/sql"
)

type Querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type Database interface {
	Querier
	Transaction(context.Context, func(*sql.Tx) error) error
}

func Revision(ctx context.Context, q Querier) (int64, error) {
	var revision int64
	err := q.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='bindings_revision'").Scan(&revision)
	return revision, err
}

func BumpRevision(ctx context.Context, q Querier) error {
	_, err := q.ExecContext(ctx, "UPDATE metadata SET value=value+1 WHERE key='bindings_revision'")
	return err
}

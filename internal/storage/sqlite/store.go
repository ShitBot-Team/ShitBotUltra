package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

const schemaVersion = 2

type Store struct{ *sql.DB }

func DSN(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", DSN(path)+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	// A single writer serializes code redemption, quotas, revisions and import commits.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{DB: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) Transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) migrate(ctx context.Context) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error {
		var version int
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
			return err
		}
		if version > schemaVersion {
			return fmt.Errorf("database schema %d is newer than supported %d", version, schemaVersion)
		}
		for next := version + 1; next <= schemaVersion; next++ {
			data, err := migrations.ReadFile(fmt.Sprintf("migrations/%03d.sql", next))
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, string(data)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", next)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Revision(ctx context.Context) (int64, error) {
	var revision int64
	err := s.DB.QueryRowContext(ctx, "SELECT value FROM metadata WHERE key='bindings_revision'").Scan(&revision)
	return revision, err
}

func BumpRevision(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "UPDATE metadata SET value=value+1 WHERE key='bindings_revision'")
	return err
}

// Backup uses SQLite's consistent snapshot operation, never a copy of a live WAL database.
func (s *Store) Backup(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("backup requires an absolute path")
	}
	_, err := s.DB.ExecContext(ctx, "VACUUM INTO ?", filepath.ToSlash(path))
	return err
}

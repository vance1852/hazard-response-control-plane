package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vance1852/hazard-response-control-plane/internal/migrate"
)

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path, migrationDir string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("database path is required")
	}
	dsn := dataSourceName(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	db.SetConnMaxLifetime(30 * time.Minute)
	cleanup := func(cause error) (*Store, error) { _ = db.Close(); return nil, cause }
	if err := db.PingContext(ctx); err != nil {
		return cleanup(fmt.Errorf("ping sqlite: %w", err))
	}
	if err := configure(ctx, db); err != nil {
		return cleanup(err)
	}
	if err := migrate.FromDirectory(ctx, db, migrationDir); err != nil {
		return cleanup(fmt.Errorf("migrate sqlite: %w", err))
	}
	return &Store{db: db}, nil
}

func dataSourceName(path string) string {
	if path == ":memory:" {
		return "file:hazard-memory-" + fmt.Sprintf("%d", time.Now().UnixNano()) + "?mode=memory&cache=shared"
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file:" + url.PathEscape(filepath.ToSlash(abs))
}

func configure(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`PRAGMA busy_timeout = 5000`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure sqlite with %q: %w", statement, err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ping(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	var one int
	if err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		return fmt.Errorf("readiness query: %w", err)
	}
	if one != 1 {
		return fmt.Errorf("readiness query returned %d", one)
	}
	return nil
}

func (s *Store) DB() *sql.DB { return s.db }

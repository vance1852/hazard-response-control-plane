package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Migration struct {
	Version  int
	Name     string
	SQL      string
	Checksum string
}

func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	result := make([]Migration, 0, len(entries))
	seen := make(map[int]string)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("migration %q must use NNN_name.sql", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migration %q has invalid version", entry.Name())
		}
		if prior, exists := seen[version]; exists {
			return nil, fmt.Errorf("migration version %d duplicated by %q and %q", version, prior, entry.Name())
		}
		content, err := fs.ReadFile(fsys, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(content)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		digest := sha256.Sum256(content)
		result = append(result, Migration{Version: version, Name: entry.Name(), SQL: string(content), Checksum: hex.EncodeToString(digest[:])})
		seen[version] = entry.Name()
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	if len(result) == 0 {
		return nil, fmt.Errorf("no migrations found")
	}
	for index, migration := range result {
		want := index + 1
		if migration.Version != want {
			return nil, fmt.Errorf("migration sequence expected version %d, found %d", want, migration.Version)
		}
	}
	return result, nil
}

func Apply(ctx context.Context, db *sql.DB, migrations []Migration) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	applied, err := readApplied(ctx, db)
	if err != nil {
		return err
	}
	known := make(map[int]Migration, len(migrations))
	for _, migration := range migrations {
		known[migration.Version] = migration
	}
	for version, record := range applied {
		migration, ok := known[version]
		if !ok {
			return fmt.Errorf("database contains unknown migration version %d", version)
		}
		if record.Name != migration.Name || record.Checksum != migration.Checksum {
			return fmt.Errorf("migration %d conflicts with applied history", version)
		}
	}
	for _, migration := range migrations {
		if _, ok := applied[migration.Version]; ok {
			continue
		}
		if err := applyOne(ctx, db, migration); err != nil {
			return err
		}
	}
	return nil
}

type appliedRecord struct{ Name, Checksum string }

func readApplied(ctx context.Context, db *sql.DB) (map[int]appliedRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("query migration ledger: %w", err)
	}
	defer rows.Close()
	result := make(map[int]appliedRecord)
	for rows.Next() {
		var version int
		var record appliedRecord
		if err := rows.Scan(&version, &record.Name, &record.Checksum); err != nil {
			return nil, fmt.Errorf("scan migration ledger: %w", err)
		}
		result[version] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migration ledger: %w", err)
	}
	return result, nil
}

func applyOne(ctx context.Context, db *sql.DB, migration Migration) error {
	if err := executeMigrationBody(ctx, db, migration); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES(?, ?, ?, ?)`,
		migration.Version, migration.Name, migration.Checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}

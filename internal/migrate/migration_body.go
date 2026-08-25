package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// applyOne executes a migration's SQL and records it in the ledger within a
// single transaction. The schema change and the ledger row commit together, so
// a failure to record the ledger (or any statement in the body) rolls back
// the structural change too. This keeps the database and the ledger in lock
// step: either a migration is fully applied and recorded, or it leaves no
// trace, so a later retry applies it exactly once.
func applyOne(ctx context.Context, db *sql.DB, migration Migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
		return fmt.Errorf("execute migration %d %s: %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES(?, ?, ?, ?)`,
		migration.Version, migration.Name, migration.Checksum, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	committed = true
	return nil
}

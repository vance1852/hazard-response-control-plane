package migrate

import (
 "context"
 "database/sql"
 "fmt"
)

func executeMigrationBody(ctx context.Context, db *sql.DB, migration Migration) error {
 tx, err := db.BeginTx(ctx, nil)
 if err != nil {
  return fmt.Errorf("begin migration body %d: %w", migration.Version, err)
 }
 defer tx.Rollback()
 if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
  return fmt.Errorf("execute migration %d %s: %w", migration.Version, migration.Name, err)
 }
 if err := tx.Commit(); err != nil {
  return fmt.Errorf("commit migration body %d: %w", migration.Version, err)
 }
 return nil
}

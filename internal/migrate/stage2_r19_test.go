package migrate

import (
 "context"
 "database/sql"
 "testing"
 "testing/fstest"

 _ "modernc.org/sqlite"
)

func TestStage2MigrationBodyRollsBackWhenLedgerWriteFails(t *testing.T) {
 db, err := sql.Open("sqlite", "file:stage2-r19?mode=memory&cache=shared")
 if err != nil { t.Fatal(err) }
 defer db.Close()
 if _, err = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT NOT NULL,checksum TEXT NOT NULL,applied_at TEXT NOT NULL);
  CREATE TRIGGER reject_r19_history BEFORE INSERT ON schema_migrations BEGIN SELECT RAISE(ABORT, 'ledger unavailable'); END;`); err != nil { t.Fatal(err) }
 fsys := fstest.MapFS{"001_hazard_index.sql": {Data: []byte("CREATE TABLE hazard_recovery_index(id TEXT PRIMARY KEY, region_id TEXT NOT NULL);")}}
 migrations, err := Load(fsys)
 if err != nil { t.Fatal(err) }
 if err = Apply(context.Background(), db, migrations); err == nil {
  t.Fatal("migration succeeded while its history ledger rejected the write")
 }
 var tableCount int
 if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='hazard_recovery_index'`).Scan(&tableCount); err != nil { t.Fatal(err) }
 if tableCount != 0 {
  t.Fatalf("failed migration left schema body committed: table count=%d", tableCount)
 }
 if _, err = db.Exec(`DROP TRIGGER reject_r19_history`); err != nil { t.Fatal(err) }
 if err = Apply(context.Background(), db, migrations); err != nil {
  t.Fatalf("migration retry after ledger recovery: %v", err)
 }
 var historyCount int
 if err = db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=1 AND name='001_hazard_index.sql'`).Scan(&historyCount); err != nil { t.Fatal(err) }
 if tableCount, err = schemaObjectCount(db, "hazard_recovery_index"); err != nil || tableCount != 1 || historyCount != 1 {
  t.Fatalf("retry state: table=%d history=%d err=%v", tableCount, historyCount, err)
 }
}

func schemaObjectCount(db *sql.DB, name string) (int, error) {
 var count int
 err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count)
 return count, err
}

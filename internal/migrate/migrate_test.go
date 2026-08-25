package migrate

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadRequiresSequentialVersions(t *testing.T) {
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("CREATE TABLE one (id INTEGER);")}, "003_third.sql": {Data: []byte("CREATE TABLE three (id INTEGER);")}}
	if _, err := Load(fsys); err == nil {
		t.Fatal("gap accepted")
	}
}
func TestLoadRejectsDuplicateVersion(t *testing.T) {
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("CREATE TABLE one (id INTEGER);")}, "001_again.sql": {Data: []byte("CREATE TABLE two (id INTEGER);")}}
	if _, err := Load(fsys); err == nil {
		t.Fatal("duplicate accepted")
	}
}
func TestLoadComputesChecksum(t *testing.T) {
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("CREATE TABLE one (id INTEGER);")}}
	migrations, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || len(migrations[0].Checksum) != 64 {
		t.Fatalf("migrations=%#v", migrations)
	}
}
func TestApplyCreatesLedgerAndTables(t *testing.T) {
	db, err := sql.Open("sqlite", "file:migrate-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("CREATE TABLE one (id INTEGER);")}, "002_second.sql": {Data: []byte("CREATE TABLE two (id INTEGER);")}}
	migrations, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(context.Background(), db, migrations); err != nil {
		t.Fatal(err)
	}
	if err := Apply(context.Background(), db, migrations); err != nil {
		t.Fatal(err)
	}
	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if count != 2 {
		t.Fatalf("ledger count=%d", count)
	}
}
func TestApplyRejectsUnknownAppliedVersion(t *testing.T) {
	db, _ := sql.Open("sqlite", "file:migrate-unknown?mode=memory&cache=shared")
	defer db.Close()
	_, _ = db.Exec(`CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY,name TEXT,checksum TEXT,applied_at TEXT);INSERT INTO schema_migrations VALUES(9,'old','hash','now')`)
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("CREATE TABLE one (id INTEGER);")}}
	migrations, _ := Load(fsys)
	if err := Apply(context.Background(), db, migrations); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error=%v", err)
	}
}
func TestLoadRejectsEmptyMigration(t *testing.T) {
	fsys := fstest.MapFS{"001_first.sql": {Data: []byte("  \n")}}
	if _, err := Load(fsys); err == nil {
		t.Fatal("empty migration accepted")
	}
}

// TestApplyBodyFailureIsAtomic verifies that a migration whose SQL body fails
// leaves neither the structural change nor a ledger row behind, so a retry
// applies it exactly once.
func TestApplyBodyFailureIsAtomic(t *testing.T) {
	db, err := sql.Open("sqlite", "file:migrate-atomic?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 002 fails: it references a table that does not exist yet.
	fsys := fstest.MapFS{
		"001_first.sql":  {Data: []byte("CREATE TABLE one (id INTEGER);")},
		"002_second.sql": {Data: []byte("CREATE TABLE two (id INTEGER); SELECT * FROM missing_table;")},
	}
	migrations, err := Load(fsys)
	if err != nil {
		t.Fatal(err)
	}

	if err := Apply(context.Background(), db, migrations); err == nil {
		t.Fatal("expected migration failure, got nil")
	}

	// Ledger must only contain version 1 (1's body succeeded; 2 rolled back fully).
	var ledgerCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&ledgerCount); err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("ledger count=%d, want 1 (atomic ledger + schema)", ledgerCount)
	}

	// Table "two" must not exist because the whole version-2 transaction rolled back.
	var name string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='two'`).Scan(&name)
	if err == nil {
		t.Fatalf("table two survived failed migration; atomicity broken")
	}

	// Retry with a corrected migration: now both versions apply exactly once.
	fsysFixed := fstest.MapFS{
		"001_first.sql":  {Data: []byte("CREATE TABLE one (id INTEGER);")},
		"002_second.sql": {Data: []byte("CREATE TABLE two (id INTEGER);")},
	}
	migrationsFixed, err := Load(fsysFixed)
	if err != nil {
		t.Fatal(err)
	}
	// Version 1 is recorded already; the retry must skip it and apply only version 2.
	if err := Apply(context.Background(), db, migrationsFixed); err != nil {
		t.Fatalf("retry apply: %v", err)
	}

	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='two'`).Scan(&name); err != nil {
		t.Fatalf("table two missing after retry: %v", err)
	}
	var versions int
	_ = db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&versions)
	if versions != 2 {
		t.Fatalf("ledger versions=%d, want 2 after retry", versions)
	}
}

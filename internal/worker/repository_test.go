package worker

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"testing"
	"time"
)

func TestSQLRepositoryClaimsAndFinishesJob(t *testing.T) {
	db, _ := sql.Open("sqlite", "file:worker-test?mode=memory&cache=shared")
	defer db.Close()
	_, err := db.Exec(`CREATE TABLE jobs(id TEXT PRIMARY KEY,kind TEXT,aggregate_type TEXT,aggregate_id TEXT,payload_json TEXT,status TEXT,attempts INTEGER,max_attempts INTEGER,available_at TEXT,lease_owner TEXT,lease_until TEXT,last_error TEXT,version INTEGER,updated_at TEXT);CREATE TABLE job_attempts(id TEXT,job_id TEXT,attempt_number INTEGER,worker_id TEXT,started_at TEXT)`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_, err = db.Exec(`INSERT INTO jobs VALUES('j','sync','incident','i','{}','pending',0,3,?,NULL,NULL,NULL,1,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	repo := SQLRepository{DB: db}
	jobs, err := repo.Claim(context.Background(), "w", now, time.Minute, 2)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
	if _, err := repo.RecordAttempt(context.Background(), jobs[0], "w", now); err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(context.Background(), jobs[0], "succeeded", nil, now); err != nil {
		t.Fatal(err)
	}
}
func TestSQLRepositoryRecoversExpiredLease(t *testing.T) {
	db, _ := sql.Open("sqlite", "file:worker-recover?mode=memory&cache=shared")
	defer db.Close()
	_, _ = db.Exec(`CREATE TABLE jobs(id TEXT PRIMARY KEY,kind TEXT,aggregate_type TEXT,aggregate_id TEXT,payload_json TEXT,status TEXT,attempts INTEGER,max_attempts INTEGER,available_at TEXT,lease_owner TEXT,lease_until TEXT,last_error TEXT,version INTEGER,updated_at TEXT)`)
	now := time.Now().UTC()
	past := now.Add(-time.Minute)
	_, _ = db.Exec(`INSERT INTO jobs VALUES('j','sync','incident','i','{}','running',1,3,?,?,?,NULL,1,?)`, past.Format(time.RFC3339Nano), "w", past.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	repo := SQLRepository{DB: db}
	if err := repo.RecoverExpired(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var status string
	_ = db.QueryRow(`SELECT status FROM jobs WHERE id='j'`).Scan(&status)
	if status != "retry_wait" {
		t.Fatalf("status=%s", status)
	}
}

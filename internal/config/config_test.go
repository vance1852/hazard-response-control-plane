package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfigIsValid(t *testing.T) {
	for _, name := range []string{"HAZARD_HTTP_ADDR", "HAZARD_DATABASE_PATH", "HAZARD_SESSION_TTL", "HAZARD_WORKER_POLL_INTERVAL", "HAZARD_WORKER_LEASE"} {
		_ = os.Unsetenv(name)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionTTL != 12*time.Hour || cfg.WorkerBatchSize < 1 {
		t.Fatalf("config=%#v", cfg)
	}
}
func TestConfigRejectsInvalidDuration(t *testing.T) {
	t.Setenv("HAZARD_SESSION_TTL", "not-duration")
	if _, err := Load(); err == nil {
		t.Fatal("invalid duration accepted")
	}
}
func TestConfigRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("HAZARD_LOG_LEVEL", "verbose")
	if _, err := Load(); err == nil {
		t.Fatal("invalid log level accepted")
	}
}
func TestConfigValidatesWorkerLease(t *testing.T) {
	cfg := Config{HTTPAddr: ":1", DatabasePath: "x", SessionTTL: time.Hour, ShutdownTimeout: time.Second, WorkerPollInterval: time.Second, WorkerLease: time.Second, WorkerBatchSize: 1, RequestBodyLimit: 1024, LogLevel: "info"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("equal worker lease accepted")
	}
}

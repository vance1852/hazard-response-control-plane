package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr           string
	DatabasePath       string
	SessionTTL         time.Duration
	ShutdownTimeout    time.Duration
	WorkerPollInterval time.Duration
	WorkerLease        time.Duration
	WorkerBatchSize    int
	BootstrapAdmin     string
	BootstrapPassword  string
	RequestBodyLimit   int64
	ReadHeaderTimeout  time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	IdleTimeout        time.Duration
	LogLevel           string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          env("HAZARD_HTTP_ADDR", ":8080"),
		DatabasePath:      env("HAZARD_DATABASE_PATH", "hazard.db"),
		BootstrapAdmin:    env("HAZARD_BOOTSTRAP_ADMIN", "commander"),
		BootstrapPassword: os.Getenv("HAZARD_BOOTSTRAP_PASSWORD"),
		LogLevel:          strings.ToLower(env("HAZARD_LOG_LEVEL", "info")),
	}
	var err error
	if cfg.SessionTTL, err = duration("HAZARD_SESSION_TTL", 12*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = duration("HAZARD_SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WorkerPollInterval, err = duration("HAZARD_WORKER_POLL_INTERVAL", 500*time.Millisecond); err != nil {
		return Config{}, err
	}
	if cfg.WorkerLease, err = duration("HAZARD_WORKER_LEASE", 30*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ReadHeaderTimeout, err = duration("HAZARD_READ_HEADER_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.ReadTimeout, err = duration("HAZARD_READ_TIMEOUT", 15*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WriteTimeout, err = duration("HAZARD_WRITE_TIMEOUT", 20*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.IdleTimeout, err = duration("HAZARD_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.WorkerBatchSize, err = integer("HAZARD_WORKER_BATCH_SIZE", 8, 1, 100); err != nil {
		return Config{}, err
	}
	limit, err := integer("HAZARD_REQUEST_BODY_LIMIT", 1<<20, 1024, 16<<20)
	if err != nil {
		return Config{}, err
	}
	cfg.RequestBodyLimit = int64(limit)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.HTTPAddr) == "" {
		problems = append(problems, "HTTP address is required")
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		problems = append(problems, "database path is required")
	}
	if c.SessionTTL <= 0 {
		problems = append(problems, "session TTL must be positive")
	}
	if c.ShutdownTimeout <= 0 {
		problems = append(problems, "shutdown timeout must be positive")
	}
	if c.WorkerPollInterval <= 0 {
		problems = append(problems, "worker poll interval must be positive")
	}
	if c.WorkerLease <= c.WorkerPollInterval {
		problems = append(problems, "worker lease must exceed poll interval")
	}
	if c.WorkerBatchSize < 1 {
		problems = append(problems, "worker batch size must be positive")
	}
	if c.RequestBodyLimit < 1024 {
		problems = append(problems, "request body limit is too small")
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, "log level must be debug, info, warn, or error")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func env(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return strings.TrimSpace(value)
	}
	return fallback
}

func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw := env(name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	return value, nil
}

func integer(name string, fallback, min, max int) (int, error) {
	raw := env(name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	return value, nil
}

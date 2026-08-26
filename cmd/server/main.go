package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/vance1852/hazard-response-control-plane/internal/audit"
	"github.com/vance1852/hazard-response-control-plane/internal/clock"
	"github.com/vance1852/hazard-response-control-plane/internal/config"
	"github.com/vance1852/hazard-response-control-plane/internal/dispatch"
	"github.com/vance1852/hazard-response-control-plane/internal/evacuation"
	"github.com/vance1852/hazard-response-control-plane/internal/hazard"
	"github.com/vance1852/hazard-response-control-plane/internal/httpapi"
	"github.com/vance1852/hazard-response-control-plane/internal/identity"
	"github.com/vance1852/hazard-response-control-plane/internal/store/sqlite"
	"github.com/vance1852/hazard-response-control-plane/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(cfg.LogLevel)}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	migrationDir := os.Getenv("HAZARD_MIGRATION_DIR")
	if migrationDir == "" {
		migrationDir = filepath.Join(".", "migrations")
	}
	store, err := sqlite.Open(ctx, cfg.DatabasePath, migrationDir)
	if err != nil {
		return err
	}
	defer store.Close()
	clk := clock.System{}
	identityService, err := identity.NewService(store, clk, cfg.SessionTTL)
	if err != nil {
		return err
	}
	hazardService, err := hazard.NewService(store, clk)
	if err != nil {
		return err
	}
	evacuationService, err := evacuation.NewService(store, clk)
	if err != nil {
		return err
	}
	dispatchService, err := dispatch.NewService(store, clk)
	if err != nil {
		return err
	}
	auditService, err := audit.NewService(store)
	if err != nil {
		return err
	}
	if cfg.BootstrapPassword != "" {
		if _, created, bootstrapErr := identity.BootstrapCommander(ctx, store, clk, cfg.BootstrapAdmin, cfg.BootstrapPassword); bootstrapErr != nil {
			return bootstrapErr
		} else if created {
			logger.Info("bootstrap commander created", "username", cfg.BootstrapAdmin)
		}
	}
	workerRepo := worker.SQLRepository{DB: store.DB()}
	durableWorker, err := worker.New(workerRepo, "hazard-server", cfg.WorkerPollInterval, cfg.WorkerLease, cfg.WorkerBatchSize)
	if err != nil {
		return err
	}
	durableWorker.Register("session.cleanup", func(ctx context.Context, job worker.Job) error {
		_, err := identityService.PurgeExpired(ctx, 100)
		return err
	})
	workerCtx, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	go durableWorker.Run(workerCtx)
	api, err := httpapi.New(httpapi.Deps{Identity: identityService, Hazard: hazardService, Evacuation: evacuationService, Dispatch: dispatchService, Audit: auditService, Health: store, Logger: logger, BodyLimit: cfg.RequestBodyLimit})
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: api.Handler(), ReadHeaderTimeout: cfg.ReadHeaderTimeout, ReadTimeout: cfg.ReadTimeout, WriteTimeout: cfg.WriteTimeout, IdleTimeout: cfg.IdleTimeout}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("hazard control plane listening", "addr", cfg.HTTPAddr)
		serverErr <- server.ListenAndServe()
	}()
	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	cancelWorker()
	durableWorker.Stop()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	return nil
}

func parseLevel(value string) slog.Level {
	switch value {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

var _ = time.Second

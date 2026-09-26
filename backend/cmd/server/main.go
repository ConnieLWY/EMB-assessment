package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ev-charger-assessment/backend/internal/config"
	"ev-charger-assessment/backend/internal/database"
	"ev-charger-assessment/backend/internal/server"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := database.Open(connectCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	// Fail early if migrations have not completed; the service never runs migrations itself.
	var version int
	var dirty bool
	checkCtx, checkCancel := context.WithTimeout(ctx, 5*time.Second)
	err = pool.QueryRow(checkCtx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty)
	checkCancel()
	if err != nil || dirty || version < 2 {
		return errors.New("database migrations must complete before backend startup")
	}
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.NewRouter(server.Dependencies{Pool: pool}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.ListenAndServe() }()
	slog.Info("HTTP server starting", "address", cfg.HTTPAddr)
	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			_ = srv.Close()
			return err
		}
		return nil
	}
}

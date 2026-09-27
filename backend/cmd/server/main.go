package main

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ev-charger-assessment/backend/internal/charger"
	"ev-charger-assessment/backend/internal/config"
	"ev-charger-assessment/backend/internal/database"
	"ev-charger-assessment/backend/internal/realtime"
	"ev-charger-assessment/backend/internal/server"
	"ev-charger-assessment/backend/internal/simulator"
)

//go:generate go tool swag init -g main.go -d .,../../internal/auth,../../internal/charger,../../internal/httpapi,../../internal/realtime --parseInternal --output ../../internal/apidocs --outputTypes json

// @title EV Charger API
// @version 0.2.0
// @description Live charger status and reservation application API. Sign in through POST /api/auth/login; the browser sends the HttpOnly session cookie automatically. Mutation requests require an allowed Origin.
// @BasePath /
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
	if err != nil || dirty || version < 3 {
		return errors.New("database migrations must complete before backend startup")
	}
	hub := realtime.NewHub(cfg.FrontendOrigin, cfg.APIOrigin)
	defer hub.Close()
	chargerService := charger.NewService(charger.NewStore(pool), time.Now, hub.Publish)
	startupCtx, startupCancel := context.WithTimeout(ctx, 10*time.Second)
	err = chargerService.Reconcile(startupCtx, time.Now())
	startupCancel()
	if err != nil {
		return err
	}
	workerCtx, workerCancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		simulator.Run(workerCtx, time.Second, func(ctx context.Context) error {
			return chargerService.Reconcile(ctx, time.Now())
		})
	}()
	go func() {
		defer workers.Done()
		simulator.Run(workerCtx, 12*time.Second, func(ctx context.Context) error {
			return chargerService.Simulate(ctx, time.Now(), rand.Intn)
		})
	}()
	defer func() { workerCancel(); workers.Wait() }()
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.NewRouter(server.Dependencies{Pool: pool, FrontendOrigin: cfg.FrontendOrigin, APIOrigin: cfg.APIOrigin, CookieSecure: cfg.CookieSecure, Hub: hub, ChargerService: chargerService}),
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

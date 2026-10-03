package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/lemonbw/booking-api/internal/adapter/httpapi"
	"github.com/lemonbw/booking-api/internal/adapter/postgres"
	"github.com/lemonbw/booking-api/internal/config"
	"github.com/lemonbw/booking-api/internal/platform/clock"
	"github.com/lemonbw/booking-api/internal/platform/idgen"
	"github.com/lemonbw/booking-api/internal/platform/logger"
	"github.com/lemonbw/booking-api/internal/platform/metrics"
	"github.com/lemonbw/booking-api/internal/platform/version"
	"github.com/lemonbw/booking-api/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log, err := logger.New(cfg.Log)
	if err != nil {
		return err
	}
	slog.SetDefault(log)
	log.Info("starting booking api", "config", cfg.Redacted())

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.Open(rootCtx, postgres.Config{
		URL:             cfg.Database.URL,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		MaxConnIdleTime: cfg.Database.MaxConnIdleTime,
		ConnectTimeout:  cfg.Database.ConnectTimeout,
		WaitTimeout:     cfg.Database.WaitTimeout,
	}, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrationsMode, err := postgres.ParseMode(cfg.Migrations.Mode)
	if err != nil {
		return err
	}
	if err := postgres.Migrate(rootCtx, pool, migrationsMode, log); err != nil {
		return err
	}

	registry := metrics.New()
	registry.SetBuildInfo(version.Version)

	observedPool := postgres.WithObserver(pool, registry)

	bookingRepo := postgres.NewBookingRepository(observedPool)
	roomRepo := postgres.NewRoomRepository(observedPool)
	clk := clock.New()
	svc := usecase.NewBookingService(bookingRepo, roomRepo, clk, idgen.NewUUIDGenerator(), log, registry, cfg.Booking.MaxSeriesOccurrence, cfg.Booking.PageSize)

	auth, err := httpapi.AuthenticatorFromConfig(httpapi.AuthConfig{
		Mode:         cfg.Security.AuthMode,
		JWTSecret:    cfg.Security.JWTSecret,
		Issuer:       cfg.Security.JWTIssuer,
		Audience:     cfg.Security.JWTAudience,
		HeaderName:   cfg.Security.HeaderName,
		HeaderSecret: cfg.Security.HeaderSecret,
	})
	if err != nil {
		return err
	}

	svc.StartRetentionSweep(rootCtx, usecase.PurgeRetention{
		Retention: cfg.Operations.Retention,
		Interval:  cfg.Operations.PurgeInterval,
		Batch:     cfg.Operations.PurgeBatch,
	})

	handler := httpapi.NewHandler(svc, log, clk.Now, cfg.HTTP.MaxRequestBody)
	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           httpapi.NewRouter(handler, pool, auth, log, registry),
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server listening", "addr", cfg.HTTP.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		return err
	case <-rootCtx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(rootCtx), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	log.Info("http server stopped")
	return <-serverErr
}

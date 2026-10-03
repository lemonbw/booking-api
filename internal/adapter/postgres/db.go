package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration

	ConnectTimeout time.Duration

	WaitTimeout time.Duration
}

func Open(ctx context.Context, cfg Config, log *slog.Logger) (*pgxpool.Pool, error) {
	if log == nil {
		return nil, errors.New("postgres.Open: logger must not be nil")
	}
	poolCfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}
	if err := waitForDB(ctx, pool, cfg, log); err != nil {
		pool.Close()
		return nil, err
	}
	log.Info("database is ready", "max_conns", cfg.MaxConns)
	return pool, nil
}
func waitForDB(ctx context.Context, pool *pgxpool.Pool, cfg Config, log *slog.Logger) error {
	deadline := time.Now().Add(cfg.WaitTimeout)
	backoff := 100 * time.Millisecond
	const maxBackoff = 2 * time.Second
	attempt := 0

	for {
		attempt++

		probeCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
		err := pool.Ping(probeCtx)
		cancel()

		if err == nil {
			if attempt > 1 {
				log.Info("database connection established", "attempts", attempt)
			}
			return nil
		}
		if ctx.Err() != nil {
			return fmt.Errorf("wait for database: %w", errors.Join(ctx.Err(), err))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("database is not ready after %s (%d attempts): %w", cfg.WaitTimeout, attempt, err)
		}
		jitter := time.Duration(rand.Int64N(int64(backoff/2) + 1))
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for database: %w", ctx.Err())
		case <-time.After(backoff + jitter):
		}
		if backoff < maxBackoff {
			backoff *= 2
		}
	}
}

type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = (pgx.Tx)(nil)
)

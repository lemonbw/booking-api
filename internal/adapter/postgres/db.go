package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lemonbw/booking-api/internal/domain"
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

const (
	sqlStateDataException = "22P02"

	sqlStateExclusionViolation  = "23P01"
	sqlStateForeignKeyViolation = "23503"
	sqlStateCheckViolation      = "23514"
	sqlStateUniqueViolation     = "23505"
	sqlStateDeadlockDetected    = "40P01"
	sqlStateSerializationFail   = "40001"
	sqlStateQueryCanceled       = "57014"
	sqlStateLockNotAvailable    = "55P03"
	sqlStateTooManyConnections  = "53300"
	sqlStateCannotConnectNow    = "57P03"
	sqlStateAdminShutdown       = "57P01"
	sqlStateCrashShutdown       = "57P02"
	sqlStateClass08             = "08"
)

const (
	constraintRoomNoOverlap  = "bookings_room_no_overlap"
	constraintRoomForeignKey = "bookings_room_id_fkey"
	constraintTimeOrder      = "bookings_time_order"
)

func translateError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		if isTransportError(err) {
			return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
		}
		return err
	}
	switch {
	case pgErr.Code == sqlStateExclusionViolation:
		if pgErr.ConstraintName == constraintRoomNoOverlap {
			return fmt.Errorf("%w: constraint %s", domain.ErrOverlap, pgErr.ConstraintName)
		}
		return fmt.Errorf("%w: constraint %s", domain.ErrIntegrity, pgErr.ConstraintName)

	case pgErr.Code == sqlStateCheckViolation:
		if pgErr.ConstraintName == constraintTimeOrder {
			return fmt.Errorf("%w: constraint %s", domain.ErrInvalidTimeRange, pgErr.ConstraintName)
		}
		return fmt.Errorf("%w: check constraint %s violated", domain.ErrIntegrity, pgErr.ConstraintName)

	case pgErr.Code == sqlStateDataException:
		if pgErr.ColumnName == "id" {
			return fmt.Errorf("%w: malformed identifier", domain.ErrInvalidBookingID)
		}
		return fmt.Errorf("%w: malformed value in column %s", domain.ErrInvalidTimeRange, pgErr.ColumnName)

	case pgErr.Code == sqlStateUniqueViolation:

		return fmt.Errorf("%w: unique constraint %s violated", domain.ErrIntegrity, pgErr.ConstraintName)

	case pgErr.Code == sqlStateSerializationFail, pgErr.Code == sqlStateDeadlockDetected,
		pgErr.Code == sqlStateLockNotAvailable, pgErr.Code == sqlStateQueryCanceled,
		pgErr.Code == sqlStateTooManyConnections, pgErr.Code == sqlStateCannotConnectNow,
		pgErr.Code == sqlStateAdminShutdown, pgErr.Code == sqlStateCrashShutdown,
		strings.HasPrefix(pgErr.Code, sqlStateClass08):
		return fmt.Errorf("%w: %s", domain.ErrUnavailable, pgErr.Code)

	default:
		return err
	}
}
func isTransportError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return !errors.Is(err, context.Canceled)
	}
	msg := err.Error()
	for _, needle := range []string{
		"conn closed",
		"connection refused",
		"connection reset",
		"broken pipe",
		"closed pool",
		"no connection",
		"unexpected EOF",
		"failed to connect",
		"host is down",
		"could not connect",
		"timeout",
		"deadline exceeded",
		"pool is closed",
		"write: broken",
		"read: connection",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

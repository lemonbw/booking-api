package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const migrationsLockKey int64 = 8_240_117_113_004_2

const migrationsLockTimeout = 30 * time.Second

type Mode string

const (
	ModeApply Mode = "apply"

	ModeCheck Mode = "check"
)

func ParseMode(value string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ModeApply):
		return ModeApply, nil
	case string(ModeCheck):
		return ModeCheck, nil
	default:
		return "", fmt.Errorf("unknown migrations mode %q (expected %q or %q)", value, ModeApply, ModeCheck)
	}
}
func Migrate(ctx context.Context, pool *pgxpool.Pool, mode Mode, log *slog.Logger) error {
	if log == nil {
		return errors.New("postgres.Migrate: logger must not be nil")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migrations transaction: %w", translateError(err))
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := lockMigrations(ctx, tx, log); err != nil {
		return err
	}

	if err := ensureMigrationsTable(ctx, tx); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, tx)
	if err != nil {
		return err
	}
	names, err := migrationNames()
	if err != nil {
		return err
	}
	var pending []string
	for _, name := range names {
		if !applied[name] {
			pending = append(pending, name)
		}
	}
	if mode == ModeCheck {
		if len(pending) > 0 {
			return fmt.Errorf("database schema is out of date: pending migrations %s (run the migration job, or set MIGRATIONS_MODE=apply)",
				strings.Join(pending, ", "))
		}
		log.Info("database schema is up to date", "migrations", len(applied))
		return tx.Commit(ctx)
	}
	for _, name := range pending {
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, translateError(err))
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("record %s: %w", name, err)
		}
		log.Info("migration applied", "version", name)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", translateError(err))
	}
	return nil
}

func lockMigrations(ctx context.Context, tx pgx.Tx, log *slog.Logger) error {
	deadline := time.Now().Add(migrationsLockTimeout)
	backoff := 50 * time.Millisecond

	for {
		var acquired bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, migrationsLockKey).Scan(&acquired); err != nil {
			return fmt.Errorf("acquire migrations lock: %w", translateError(err))
		}
		if acquired {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("another instance has held the migrations lock for more than %s", migrationsLockTimeout)
		}
		log.Info("waiting for the migrations lock", "holder", "another instance is migrating")

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for migrations lock: %w", ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < time.Second {
			backoff *= 2
		}
	}
}
func ensureMigrationsTable(ctx context.Context, tx pgx.Tx) error {
	const q = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`
	if _, err := tx.Exec(ctx, q); err != nil {
		return fmt.Errorf("create schema_migrations: %w", translateError(err))
	}
	return nil
}
func appliedVersions(ctx context.Context, tx pgx.Tx) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan schema_migrations: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema_migrations: %w", err)
	}
	return applied, nil
}
func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		return nil, errors.New("no migrations found")
	}
	sort.Strings(names)
	return names, nil
}

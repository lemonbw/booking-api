package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAppliesDefaults(t *testing.T) {
	cfg := load(t)

	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want %q", cfg.HTTP.Addr, ":8080")
	}
	if cfg.Database.MaxConns != 10 {
		t.Errorf("Database.MaxConns = %d, want 10", cfg.Database.MaxConns)
	}
	if cfg.HTTP.ShutdownTimeout != 15*time.Second {
		t.Errorf("HTTP.ShutdownTimeout = %s, want 15s", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.Booking.MaxSeriesOccurrence != 52 {
		t.Errorf("Booking.MaxSeriesOccurrence = %d, want 52", cfg.Booking.MaxSeriesOccurrence)
	}
	if cfg.Booking.PageSize != 100 {
		t.Errorf("Booking.PageSize = %d, want 100", cfg.Booking.PageSize)
	}
	if cfg.Log.Level != "info" || cfg.Log.Format != "json" {
		t.Errorf("Log = %+v, want level info and format json", cfg.Log)
	}
	if cfg.Security.AuthMode != "none" {
		t.Errorf("Security.AuthMode = %q, want %q", cfg.Security.AuthMode, "none")
	}
	if cfg.Migrations.Mode != "apply" {
		t.Errorf("Migrations.Mode = %q, want %q", cfg.Migrations.Mode, "apply")
	}
}

func TestLoadReadsTheEnvironment(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9999")
	t.Setenv("DB_MAX_CONNS", "42")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("AUTH_MODE", "JWT")
	t.Setenv("AUTH_JWT_SECRET", "a-jwt-secret-long-enough-to-pass-validation")
	t.Setenv("BOOKING_PAGE_SIZE", "7")
	t.Setenv("MIGRATIONS_MODE", "check")

	cfg := load(t)

	if cfg.HTTP.Addr != ":9999" {
		t.Errorf("HTTP.Addr = %q, want %q", cfg.HTTP.Addr, ":9999")
	}
	if cfg.Database.MaxConns != 42 {
		t.Errorf("Database.MaxConns = %d, want 42", cfg.Database.MaxConns)
	}
	if cfg.HTTP.ShutdownTimeout != 3*time.Second {
		t.Errorf("HTTP.ShutdownTimeout = %s, want 3s", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "text" {
		t.Errorf("Log = %+v, want level debug and format text", cfg.Log)
	}
	if cfg.Security.AuthMode != "jwt" {
		t.Errorf("Security.AuthMode = %q, want it lowercased to jwt", cfg.Security.AuthMode)
	}
	if cfg.Booking.PageSize != 7 {
		t.Errorf("Booking.PageSize = %d, want 7", cfg.Booking.PageSize)
	}
	if cfg.Migrations.Mode != "check" {
		t.Errorf("Migrations.Mode = %q, want check", cfg.Migrations.Mode)
	}
}

func TestLoadFallsBackWhenTheValueIsUnusable(t *testing.T) {
	t.Setenv("DB_MAX_CONNS", "not-a-number")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "not-a-duration")

	cfg := load(t)

	if cfg.Database.MaxConns != 10 {
		t.Errorf("Database.MaxConns = %d, want the default 10", cfg.Database.MaxConns)
	}
	if cfg.HTTP.ShutdownTimeout != 15*time.Second {
		t.Errorf("HTTP.ShutdownTimeout = %s, want the default 15s", cfg.HTTP.ShutdownTimeout)
	}
}

func TestLoadRejectsABlankValueInsteadOfDefaulting(t *testing.T) {
	cases := map[string]struct {
		key       string
		want      string
		authMode  string
		authExtra map[string]string
	}{
		"listen address": {key: "HTTP_ADDR", want: "HTTP_ADDR"},
		"database url":   {key: "DATABASE_URL", want: "DATABASE_URL"},
		"auth header name": {
			key:      "AUTH_HEADER_NAME",
			want:     "AUTH_HEADER_NAME",
			authMode: "header",
			authExtra: map[string]string{
				"AUTH_HEADER_SECRET": "a-header-secret-long-enough-to-pass-validation",
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.authMode != "" {
				t.Setenv("AUTH_MODE", tc.authMode)
			}
			for key, value := range tc.authExtra {
				t.Setenv(key, value)
			}
			t.Setenv(tc.key, "   ")

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load() error = %v, want it to name %s", err, tc.want)
			}
		})
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_MAX_CONNS", "0")
	t.Setenv("DB_MIN_CONNS", "5")
	t.Setenv("HTTP_SHUTDOWN_TIMEOUT", "0s")
	t.Setenv("HTTP_MAX_BODY_BYTES", "-1")
	t.Setenv("BOOKING_MAX_SERIES_OCCURRENCES", "0")
	t.Setenv("BOOKING_PAGE_SIZE", "0")
	t.Setenv("AUTH_MODE", "basic")
	t.Setenv("MIGRATIONS_MODE", "rollback")
	t.Setenv("PURGE_INTERVAL", "0s")
	t.Setenv("PURGE_BATCH_SIZE", "0")
	t.Setenv("RETENTION_CANCELLED", "-1h")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() returned nil error, want a validation failure")
	}

	message := err.Error()
	for _, want := range []string{
		"DATABASE_URL",
		"DB_MAX_CONNS",
		"DB_MIN_CONNS",
		"HTTP_SHUTDOWN_TIMEOUT",
		"HTTP_MAX_BODY_BYTES",
		"BOOKING_MAX_SERIES_OCCURRENCES",
		"BOOKING_PAGE_SIZE",
		"AUTH_MODE",
		"MIGRATIONS_MODE",
		"PURGE_INTERVAL",
		"PURGE_BATCH_SIZE",
		"RETENTION_CANCELLED",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error does not mention %s:\n%s", want, message)
		}
	}
}

func TestLoadRejectsAnAuthModeWithoutItsSecret(t *testing.T) {
	t.Setenv("AUTH_MODE", "jwt")
	t.Setenv("AUTH_JWT_SECRET", "too-short")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "AUTH_JWT_SECRET") {
		t.Fatalf("Load() error = %v, want it to name AUTH_JWT_SECRET", err)
	}

	t.Setenv("AUTH_MODE", "header")
	t.Setenv("AUTH_HEADER_NAME", "")
	t.Setenv("AUTH_HEADER_SECRET", "too-short")

	_, err = Load()
	if err == nil || !strings.Contains(err.Error(), "AUTH_HEADER_NAME") {
		t.Fatalf("Load() error = %v, want it to name AUTH_HEADER_NAME", err)
	}
}

func TestRedactedHidesSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:supersecretpassword@localhost:5432/booking")
	t.Setenv("AUTH_JWT_SECRET", "a-very-secret-value-that-must-not-leak")

	cfg := load(t)

	rendered := cfg.Redacted()
	for _, secret := range []string{"supersecretpassword", "a-very-secret-value-that-must-not-leak"} {
		if strings.Contains(rendered, secret) {
			t.Errorf("Redacted() leaked %q: %s", secret, rendered)
		}
	}
	if !strings.Contains(rendered, "http_addr=") {
		t.Errorf("Redacted() = %q, want it to report the settings it loaded", rendered)
	}
}

func load(t *testing.T) Config {
	t.Helper()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}
	return cfg
}

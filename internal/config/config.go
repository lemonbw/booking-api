package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lemonbw/booking-api/internal/platform/logger"
)

type Config struct {
	HTTP     HTTP
	Database Database
	Log      logger.Config
}

type HTTP struct {
	Addr              string
	ReadTimeout       time.Duration
	ReadHeaderTimeout time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxRequestBody    int64
}

type Database struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration

	ConnectTimeout time.Duration

	WaitTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTP: HTTP{
			Addr:              env("HTTP_ADDR", ":8080"),
			ReadTimeout:       envDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			ReadHeaderTimeout: envDuration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			WriteTimeout:      envDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:       envDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   envDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
			MaxRequestBody:    int64(envInt("HTTP_MAX_BODY_BYTES", 64*1024)),
		},
		Database: Database{
			URL:             env("DATABASE_URL", "postgres://booking:booking@localhost:5432/booking?sslmode=disable"),
			MaxConns:        int32(envInt("DB_MAX_CONNS", 10)),
			MinConns:        int32(envInt("DB_MIN_CONNS", 1)),
			MaxConnLifetime: envDuration("DB_MAX_CONN_LIFETIME", time.Hour),
			MaxConnIdleTime: envDuration("DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
			ConnectTimeout:  envDuration("DB_CONNECT_TIMEOUT", 5*time.Second),
			WaitTimeout:     envDuration("DB_WAIT_TIMEOUT", 60*time.Second),
		},
		Booking: Booking{
			MaxSeriesOccurrence: envInt("BOOKING_MAX_SERIES_OCCURRENCES", 52),
		},
		Log: logger.Config{
			Level:  env("LOG_LEVEL", "info"),
			Format: env("LOG_FORMAT", "json"),
		},
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
func (c Config) validate() error {
	var errs []error

	if strings.TrimSpace(c.Database.URL) == "" {
		errs = append(errs, errors.New("DATABASE_URL must not be empty"))
	}
	if c.Database.MaxConns < 1 {
		errs = append(errs, errors.New("DB_MAX_CONNS must be >= 1"))
	}
	if c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		errs = append(errs, errors.New("DB_MIN_CONNS must be >= 0 and <= DB_MAX_CONNS"))
	}
	if strings.TrimSpace(c.HTTP.Addr) == "" {
		errs = append(errs, errors.New("HTTP_ADDR must not be empty"))
	}
	for name, d := range map[string]time.Duration{
		"HTTP_READ_TIMEOUT":        c.HTTP.ReadTimeout,
		"HTTP_READ_HEADER_TIMEOUT": c.HTTP.ReadHeaderTimeout,
		"HTTP_WRITE_TIMEOUT":       c.HTTP.WriteTimeout,
		"HTTP_SHUTDOWN_TIMEOUT":    c.HTTP.ShutdownTimeout,
		"DB_WAIT_TIMEOUT":          c.Database.WaitTimeout,
	} {
		if d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be positive", name))
		}
	}
	if c.HTTP.MaxRequestBody <= 0 {
		errs = append(errs, errors.New("HTTP_MAX_BODY_BYTES must be positive"))
	}
	if c.Booking.MaxSeriesOccurrence <= 0 {
		errs = append(errs, errors.New("BOOKING_MAX_SERIES_OCCURRENCES must be positive"))
	}
	return errors.Join(errs...)
}
func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
func envInt(key string, fallback int) int {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
}
func envDuration(key string, fallback time.Duration) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	v, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
}
func (c Config) Redacted() string {
	return fmt.Sprintf(
		"http_addr=%s read_timeout=%s shutdown_timeout=%s db_max_conns=%d db_wait_timeout=%s log_level=%s",
		c.HTTP.Addr, c.HTTP.ReadTimeout, c.HTTP.ShutdownTimeout,
		c.Database.MaxConns, c.Database.WaitTimeout, c.Log.Level,
	)
}

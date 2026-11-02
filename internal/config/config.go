// Package config loads the API's configuration from the environment.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Env names the deployment the process runs in.
type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvProduction  Env = "production"
)

// Config holds every setting the API reads at startup.
type Config struct {
	Env             Env
	HTTPAddr        string
	DatabaseURL     string
	DBMaxConns      int32
	LogLevel        slog.Level
	ShutdownTimeout time.Duration
}

// IsProduction reports whether the API runs in production.
func (c Config) IsProduction() bool { return c.Env == EnvProduction }

// Load reads configuration through getenv and validates it. Every problem is
// reported at once, so a misconfigured deploy fails with one clear message.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Env:             Env(cmp.Or(getenv("APP_ENV"), string(EnvDevelopment))),
		HTTPAddr:        cmp.Or(getenv("HTTP_ADDR"), ":5000"),
		DatabaseURL:     getenv("DATABASE_URL"),
		DBMaxConns:      10,
		ShutdownTimeout: 15 * time.Second,
	}

	var errs []error
	switch cfg.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test or production, got %q", cfg.Env))
	}
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if v := getenv("DB_MAX_CONNS"); v != "" {
		n, err := strconv.ParseInt(v, 10, 32)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("DB_MAX_CONNS: %w", err))
		case n < 1:
			errs = append(errs, fmt.Errorf("DB_MAX_CONNS must be at least 1, got %d", n))
		default:
			cfg.DBMaxConns = int32(n)
		}
	}
	if v := getenv("SHUTDOWN_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err))
		case d <= 0:
			errs = append(errs, fmt.Errorf("SHUTDOWN_TIMEOUT must be positive, got %s", d))
		default:
			cfg.ShutdownTimeout = d
		}
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(cmp.Or(getenv("LOG_LEVEL"), "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	return cfg, errors.Join(errs...)
}

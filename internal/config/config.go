// Package config loads the API's configuration from the environment.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"
)

// minSecretBytes is the shortest JWT_SECRET accepted: 256 bits for HS256.
const minSecretBytes = 32

// Env names the deployment the process runs in.
type Env string

// Deployments the API knows.
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

	JWTSecret      []byte
	AccessTokenTTL time.Duration
	// AppURL is the frontend's origin; emailed links point there.
	AppURL string
	SMTP   SMTP
	// UploadDir holds files people attach (memo photos); created if missing.
	UploadDir string
}

// SMTP configures outgoing email. With no Host, mail is logged instead of
// sent, which is refused in production.
type SMTP struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
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
		JWTSecret:       []byte(getenv("JWT_SECRET")),
		AccessTokenTTL:  15 * time.Minute,
		AppURL:          cmp.Or(getenv("APP_URL"), "http://localhost:3000"),
		UploadDir:       cmp.Or(getenv("UPLOAD_DIR"), "data/uploads"),
		SMTP: SMTP{
			Host:     getenv("SMTP_HOST"),
			Port:     cmp.Or(getenv("SMTP_PORT"), "587"),
			Username: getenv("SMTP_USERNAME"),
			Password: getenv("SMTP_PASSWORD"),
			From:     cmp.Or(getenv("MAIL_FROM"), "Prokolpo <no-reply@prokolpo.app>"),
		},
	}

	errs := []error{
		cfg.validateBasics(),
		parseInt32(getenv, "DB_MAX_CONNS", 1, &cfg.DBMaxConns),
		parseDuration(getenv, "SHUTDOWN_TIMEOUT", time.Nanosecond, 24*time.Hour, &cfg.ShutdownTimeout),
		parseDuration(getenv, "ACCESS_TOKEN_TTL", time.Minute, time.Hour, &cfg.AccessTokenTTL),
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(cmp.Or(getenv("LOG_LEVEL"), "info"))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL: %w", err))
	}
	errs = append(errs, cfg.validateAuth())
	return cfg, errors.Join(errs...)
}

func (c Config) validateBasics() error {
	var errs []error
	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test or production, got %q", c.Env))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	return errors.Join(errs...)
}

func (c Config) validateAuth() error {
	var errs []error
	if len(c.JWTSecret) < minSecretBytes {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d bytes", minSecretBytes))
	}
	if u, err := url.Parse(c.AppURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("APP_URL must be an http(s) origin, got %q", c.AppURL))
	}
	if c.IsProduction() && c.SMTP.Host == "" {
		errs = append(errs, errors.New("SMTP_HOST is required in production (emails carry reset links)"))
	}
	return errors.Join(errs...)
}

// parseInt32 sets *dst from key when the variable is present and at least lowest.
func parseInt32(getenv func(string) string, key string, lowest int64, dst *int32) error {
	v := getenv(key)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	switch {
	case err != nil:
		return fmt.Errorf("%s: %w", key, err)
	case n < lowest:
		return fmt.Errorf("%s must be at least %d, got %d", key, lowest, n)
	default:
		*dst = int32(n)
		return nil
	}
}

// parseDuration sets *dst from key when the variable is present and within [lowest, highest].
func parseDuration(getenv func(string) string, key string, lowest, highest time.Duration, dst *time.Duration) error {
	v := getenv(key)
	if v == "" {
		return nil
	}
	d, err := time.ParseDuration(v)
	switch {
	case err != nil:
		return fmt.Errorf("%s: %w", key, err)
	case d < lowest || d > highest:
		return fmt.Errorf("%s must be between %s and %s, got %s", key, lowest, highest, d)
	default:
		*dst = d
		return nil
	}
}

package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func envOf(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

const testSecret = "0123456789abcdef0123456789abcdef"

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{"DATABASE_URL": "postgres://localhost/prokolpo", "JWT_SECRET": testSecret}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Env != EnvDevelopment {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvDevelopment)
	}
	if cfg.HTTPAddr != ":5000" {
		t.Errorf("HTTPAddr = %q, want :5000", cfg.HTTPAddr)
	}
	if cfg.DBMaxConns != 10 {
		t.Errorf("DBMaxConns = %d, want 10", cfg.DBMaxConns)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("ShutdownTimeout = %s, want 15s", cfg.ShutdownTimeout)
	}
	if cfg.AccessTokenTTL != 15*time.Minute || cfg.AppURL != "http://localhost:3000" || cfg.SMTP.Port != "587" {
		t.Errorf("auth defaults not applied: ttl %s, app url %q, smtp port %q", cfg.AccessTokenTTL, cfg.AppURL, cfg.SMTP.Port)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"APP_ENV":          "production",
		"HTTP_ADDR":        ":8080",
		"DATABASE_URL":     "postgres://db/prokolpo",
		"DB_MAX_CONNS":     "25",
		"LOG_LEVEL":        "warn",
		"SHUTDOWN_TIMEOUT": "30s",
		"JWT_SECRET":       testSecret,
		"ACCESS_TOKEN_TTL": "10m",
		"APP_URL":          "https://prokolpo.app",
		"SMTP_HOST":        "smtp.example.com",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !cfg.IsProduction() {
		t.Error("IsProduction() = false, want true")
	}
	if cfg.HTTPAddr != ":8080" || cfg.DBMaxConns != 25 || cfg.LogLevel != slog.LevelWarn || cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := Load(envOf(map[string]string{
		"APP_ENV":          "staging",
		"DB_MAX_CONNS":     "0",
		"LOG_LEVEL":        "loud",
		"SHUTDOWN_TIMEOUT": "-1s",
		"JWT_SECRET":       "short",
		"ACCESS_TOKEN_TTL": "5h",
		"APP_URL":          "prokolpo.app",
	}))
	if err == nil {
		t.Fatal("Load: want an error, got nil")
	}

	for _, want := range []string{"APP_ENV", "DATABASE_URL", "DB_MAX_CONNS", "LOG_LEVEL", "SHUTDOWN_TIMEOUT", "JWT_SECRET", "ACCESS_TOKEN_TTL", "APP_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestLoadRequiresSMTPInProduction(t *testing.T) {
	_, err := Load(envOf(map[string]string{"APP_ENV": "production", "DATABASE_URL": "postgres://db/p", "JWT_SECRET": testSecret}))
	if err == nil || !strings.Contains(err.Error(), "SMTP_HOST") {
		t.Fatalf("Load err = %v, want it to require SMTP_HOST", err)
	}
}

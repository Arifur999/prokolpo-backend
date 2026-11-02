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

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{"DATABASE_URL": "postgres://localhost/prokolpo"}))
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
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(envOf(map[string]string{
		"APP_ENV":          "production",
		"HTTP_ADDR":        ":8080",
		"DATABASE_URL":     "postgres://db/prokolpo",
		"DB_MAX_CONNS":     "25",
		"LOG_LEVEL":        "warn",
		"SHUTDOWN_TIMEOUT": "30s",
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
	}))
	if err == nil {
		t.Fatal("Load: want an error, got nil")
	}

	for _, want := range []string{"APP_ENV", "DATABASE_URL", "DB_MAX_CONNS", "LOG_LEVEL", "SHUTDOWN_TIMEOUT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

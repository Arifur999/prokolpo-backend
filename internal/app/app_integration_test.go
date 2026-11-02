//go:build integration

package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestRunServesHealthAndShutsDown boots the real API against TEST_DATABASE_URL,
// checks the health probe, then cancels and expects a clean exit.
func TestRunServesHealthAndShutsDown(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	addr := freeAddr(t)
	env := map[string]string{"APP_ENV": "test", "HTTP_ADDR": addr, "DATABASE_URL": dbURL, "LOG_LEVEL": "error", "JWT_SECRET": "integration-test-secret-32-bytes-long"}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, func(k string) string { return env[k] }, io.Discard) }()

	status := waitForHealth(t, "http://"+addr+"/api/v1/health")
	if status != http.StatusOK {
		t.Fatalf("health status = %d, want 200", status)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v after cancel, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	return addr
}

func waitForHealth(t *testing.T, url string) int {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close() // only the status matters
			return resp.StatusCode
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("API did not answer on %s within 10s", url)
	return 0
}

package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/httpx"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func serve(t *testing.T, db fakeDB, method, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	New(Deps{Log: slog.New(slog.DiscardHandler), DB: db}).ServeHTTP(rec, httptest.NewRequest(method, path, http.NoBody))

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, rec.Body.String())
	}
	return rec, body
}

func TestHealth(t *testing.T) {
	tests := []struct {
		name         string
		db           fakeDB
		wantStatus   int
		wantDatabase string
	}{
		{"database up", fakeDB{}, http.StatusOK, "up"},
		{"database down", fakeDB{err: errors.New("connection refused")}, http.StatusServiceUnavailable, "down"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := serve(t, tt.db, http.MethodGet, "/api/v1/health")

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			data, ok := body["data"].(map[string]any)
			if !ok || data["database"] != tt.wantDatabase {
				t.Errorf("data = %v, want database %q", body["data"], tt.wantDatabase)
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Error("health response must not be cached")
			}
		})
	}
}

func TestUnknownRouteIsEnvelope404(t *testing.T) {
	rec, body := serve(t, fakeDB{}, http.MethodGet, "/api/v1/nope")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if body["success"] != false || body["message"] == "" {
		t.Errorf("body = %v, want the error envelope", body)
	}
	if rec.Header().Get(httpx.RequestIDHeader) == "" {
		t.Error("response has no request id")
	}
}

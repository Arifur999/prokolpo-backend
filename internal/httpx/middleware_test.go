package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWithRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		wantSame bool
	}{
		{"keeps a sane id", "req-123_abc", true},
		{"replaces a missing id", "", false},
		{"replaces an id with spaces", "drop table", false},
		{"replaces an overlong id", strings.Repeat("a", 65), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := WithRequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = RequestID(r.Context()) }))

			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			if tt.incoming != "" {
				req.Header.Set(RequestIDHeader, tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if seen == "" {
				t.Fatal("handler saw no request id")
			}
			if got := rec.Header().Get(RequestIDHeader); got != seen {
				t.Errorf("response header %q, context %q: want the same id", got, seen)
			}
			if (seen == tt.incoming) != tt.wantSame {
				t.Errorf("id = %q, incoming %q, wantSame %v", seen, tt.incoming, tt.wantSame)
			}
		})
	}
}

func TestRecoverPanics(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	h := RecoverPanics(log, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"success":false`) || strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("body = %s, want the generic error envelope", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "boom") {
		t.Errorf("panic was not logged: %s", logs.String())
	}
}

func TestLogRequests(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /projects/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	rec := httptest.NewRecorder()
	LogRequests(log, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/projects/secret-id", http.NoBody))

	line := logs.String()
	if !strings.Contains(line, `"route":"GET /projects/{id}"`) || !strings.Contains(line, `"status":418`) {
		t.Errorf("log line missing route or status: %s", line)
	}
	if strings.Contains(line, "secret-id") {
		t.Errorf("raw path leaked into logs: %s", line)
	}
}

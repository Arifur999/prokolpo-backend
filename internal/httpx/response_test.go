package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, rec.Body.String())
	}
	return body
}

func TestResponderJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	NewResponder(slog.New(slog.DiscardHandler)).JSON(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody), http.StatusCreated, "Project created", map[string]string{"id": "p1"})

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	want := map[string]any{"success": true, "message": "Project created", "data": map[string]any{"id": "p1"}}
	if diff := cmp.Diff(want, decodeBody(t, rec)); diff != "" {
		t.Errorf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestResponderList(t *testing.T) {
	rec := httptest.NewRecorder()
	NewResponder(slog.New(slog.DiscardHandler)).List(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody), "", []string{"a", "b"}, NewMeta(2, 20, 41))

	want := map[string]any{
		"success": true, "message": "", "data": []any{"a", "b"},
		"meta": map[string]any{"page": 2.0, "limit": 20.0, "total": 41.0, "totalPages": 3.0},
	}
	if diff := cmp.Diff(want, decodeBody(t, rec)); diff != "" {
		t.Errorf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestResponderErrorMapsKinds(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
	}{
		{apperr.Invalid("bad"), http.StatusUnprocessableEntity},
		{apperr.Unauthorized("who"), http.StatusUnauthorized},
		{apperr.Forbidden("no"), http.StatusForbidden},
		{apperr.NotFound("gone"), http.StatusNotFound},
		{apperr.Conflict("dup"), http.StatusConflict},
		{apperr.TooManyRequests("slow"), http.StatusTooManyRequests},
		{apperr.Unavailable("maintenance"), http.StatusServiceUnavailable},
		{fmt.Errorf("load project: %w", apperr.NotFound("gone")), http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewResponder(slog.New(slog.DiscardHandler)).Error(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody), tt.err)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestResponderErrorKeepsFieldErrors(t *testing.T) {
	rec := httptest.NewRecorder()
	err := apperr.Invalid("Check the highlighted fields.", apperr.FieldError{Path: "budget", Message: "Must be positive."})
	NewResponder(slog.New(slog.DiscardHandler)).Error(rec, httptest.NewRequest(http.MethodPost, "/", http.NoBody), err)

	want := map[string]any{
		"success": false, "message": "Check the highlighted fields.",
		"errorSource": []any{map[string]any{"path": "budget", "message": "Must be positive."}},
	}
	if diff := cmp.Diff(want, decodeBody(t, rec)); diff != "" {
		t.Errorf("body mismatch (-want +got):\n%s", diff)
	}
}

func TestResponderErrorCarriesCode(t *testing.T) {
	rec := httptest.NewRecorder()
	err := apperr.WithCode(apperr.Forbidden("Your account is suspended."), "ACCOUNT_SUSPENDED")
	NewResponder(slog.New(slog.DiscardHandler)).Error(rec, httptest.NewRequest(http.MethodPost, "/", http.NoBody), err)

	if got := decodeBody(t, rec)["code"]; got != "ACCOUNT_SUSPENDED" {
		t.Errorf("code = %v, want ACCOUNT_SUSPENDED", got)
	}
}

func TestResponderErrorHidesUnexpectedErrors(t *testing.T) {
	var logs bytes.Buffer
	rec := httptest.NewRecorder()
	NewResponder(slog.New(slog.NewJSONHandler(&logs, nil))).Error(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody), errors.New("pq: password authentication failed"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	body := decodeBody(t, rec)
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("internal error leaked to client: %s", rec.Body.String())
	}
	if body["errorSource"] == nil {
		t.Error("errorSource is null, want an empty list")
	}
	if !strings.Contains(logs.String(), "password authentication failed") {
		t.Errorf("unexpected error was not logged: %s", logs.String())
	}
}

func TestNewMeta(t *testing.T) {
	tests := []struct {
		page, limit, total, wantPages int
	}{
		{1, 20, 0, 0},
		{1, 20, 20, 1},
		{1, 20, 21, 2},
		{1, 0, 5, 0},
	}
	for _, tt := range tests {
		if got := NewMeta(tt.page, tt.limit, tt.total).TotalPages; got != tt.wantPages {
			t.Errorf("NewMeta(%d, %d, %d).TotalPages = %d, want %d", tt.page, tt.limit, tt.total, got, tt.wantPages)
		}
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name   string `json:"name"`
		Budget int    `json:"budget"`
	}
	tests := []struct {
		name    string
		body    io.Reader
		wantErr string
	}{
		{"valid", strings.NewReader(`{"name":"Mirpur","budget":100}`), ""},
		{"empty", strings.NewReader(""), "Request body is empty."},
		{"unknown field", strings.NewReader(`{"name":"x","role":"admin"}`), "known fields"},
		{"wrong type", strings.NewReader(`{"budget":"lots"}`), "wrong type"},
		{"trailing data", strings.NewReader(`{"name":"x"}{}`), "single JSON object"},
		{"too large", strings.NewReader(`{"name":"` + strings.Repeat("a", DefaultMaxBodyBytes) + `"}`), "too large"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst payload
			err := DecodeJSON(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", tt.body), &dst)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("DecodeJSON: %v", err)
				}
				return
			}
			appErr, ok := apperr.As(err)
			if !ok || appErr.Kind != apperr.KindInvalid {
				t.Fatalf("err = %v, want an apperr Invalid", err)
			}
			if !strings.Contains(appErr.Message, tt.wantErr) {
				t.Errorf("message = %q, want it to contain %q", appErr.Message, tt.wantErr)
			}
		})
	}
}

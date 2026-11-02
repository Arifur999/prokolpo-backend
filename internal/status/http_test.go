package status

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/httpx"
	"github.com/Arifur999/prokolpo-backend/internal/token"
)

type fixed struct {
	status Status
	err    error
}

func (f fixed) Current(context.Context) (Status, error) { return f.status, f.err }

func TestBlockDuringMaintenance(t *testing.T) {
	issuer := token.NewIssuer([]byte("status-test-secret-32-bytes-long!!"), time.Minute)
	staff, _, err := issuer.Issue("u1", token.RoleStaff)
	if err != nil {
		t.Fatal(err)
	}
	user, _, err := issuer.Issue("u2", token.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	on := fixed{status: Status{Maintenance: Maintenance{Enabled: true}}}

	tests := []struct {
		name   string
		src    Source
		path   string
		bearer string
		want   int
	}{
		{"off", fixed{}, "/api/v1/projects", "", http.StatusOK},
		{"on blocks users", on, "/api/v1/projects", user, http.StatusServiceUnavailable},
		{"on blocks visitors", on, "/api/v1/support/requests", "", http.StatusServiceUnavailable},
		{"on lets staff through", on, "/api/v1/projects", staff, http.StatusOK},
		{"on keeps login open", on, "/api/v1/auth/login", "", http.StatusOK},
		{"on keeps status open", on, "/api/v1/status", "", http.StatusOK},
		{"unreadable flag fails open", fixed{err: errors.New("db down")}, "/api/v1/projects", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
			h := BlockDuringMaintenance(tt.src, issuer, httpx.NewResponder(slog.New(slog.DiscardHandler)))(ok)
			req := httptest.NewRequest(http.MethodGet, tt.path, http.NoBody)
			if tt.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tt.bearer)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

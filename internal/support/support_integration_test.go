//go:build integration

package support

import (
	"context"
	"testing"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/testdb"
)

func TestCreateNumbersRequestsAndLimitsSpam(t *testing.T) {
	svc := NewService(testdb.Open(t))
	ctx := context.Background()
	req := Request{Name: "তানিয়া রহমান", Phone: "01755889900", Topic: "login", Source: "forgot", Message: "পাসওয়ার্ড মনে নেই"}

	first, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	second, err := svc.Create(ctx, req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if first.Reference == second.Reference || first.Reference[:2] != "S-" {
		t.Errorf("references %q and %q", first.Reference, second.Reference)
	}

	if _, thirdErr := svc.Create(ctx, req); thirdErr != nil {
		t.Fatalf("third Create: %v", thirdErr)
	}
	_, err = svc.Create(ctx, req)
	if e, ok := apperr.As(err); !ok || e.Code != codeTooMany {
		t.Fatalf("fourth Create err = %v, want %s", err, codeTooMany)
	}
}

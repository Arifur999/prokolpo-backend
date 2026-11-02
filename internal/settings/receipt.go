package settings

import (
	"context"
	"fmt"
	"regexp"
	"slices"

	"github.com/jackc/pgx/v5"
)

// ReceiptInput is the "রশিদ ও প্রিন্ট" form after validation. Blank text falls back to the
// owner's company, the project's location and the owner's phone.
type ReceiptInput struct {
	Company, Address, Phone, Footer *string
	Prefix, Lang, Paper             string
}

type receiptBody struct {
	Company string `json:"company"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
	Prefix  string `json:"prefix"`
	Lang    string `json:"lang"`
	Paper   string `json:"paper"`
	Footer  string `json:"footer"`
}

var prefixPattern = regexp.MustCompile(`^[A-Z]{2,5}$`)

func (b receiptBody) validate(lang string) (ReceiptInput, error) {
	f := &fields{lang: lang}
	in := ReceiptInput{
		Company: f.text("company", b.Company, 80, "company"),
		Address: f.text("address", b.Address, 160, "address"),
		Phone:   f.text("phone", b.Phone, 30, "phone"),
		Footer:  f.text("footer", b.Footer, 160, "footer"),
		Prefix:  b.Prefix, Lang: b.Lang, Paper: b.Paper,
	}
	if !prefixPattern.MatchString(b.Prefix) {
		f.add("prefix", "prefix")
	}
	if !slices.Contains([]string{"bn", "en", "both"}, b.Lang) {
		f.add("lang", "choice")
	}
	if !slices.Contains([]string{"A4", "A5"}, b.Paper) {
		f.add("paper", "choice")
	}
	return in, f.err()
}

// SaveReceipt sets what this project's receipts print. A new prefix applies to new receipts
// only; the numbers already printed keep theirs.
func (s *Service) SaveReceipt(ctx context.Context, userID, projectID string, in ReceiptInput) error {
	return s.owned(ctx, userID, projectID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE projects SET receipt_company = $2, receipt_address = $3, receipt_phone = $4, receipt_prefix = $5,
			       receipt_lang = $6, receipt_paper = $7, receipt_footer = $8
			WHERE id = $1`, projectID, in.Company, in.Address, in.Phone, in.Prefix, in.Lang, in.Paper, in.Footer)
		if err != nil {
			return fmt.Errorf("save receipt settings: %w", err)
		}
		return logSettings(ctx, tx, projectID, userID, "receipt", in.Prefix, nil)
	})
}

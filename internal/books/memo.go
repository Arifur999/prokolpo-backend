package books

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
	"github.com/Arifur999/prokolpo-backend/internal/project"
)

// MaxMemoBytes is the largest memo photo accepted (board ExpenseNew: "সর্বোচ্চ 5MB").
const MaxMemoBytes = 5 << 20

const codeNotImage = "MEMO_NOT_IMAGE"

// Files stores memo photos.
type Files interface {
	Save(name string, r io.Reader) error
	Open(name string) (io.ReadSeekCloser, error)
	Remove(name string) error
}

// The photo types accepted, judged by the bytes rather than by what the upload claims.
var memoTypes = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp"}

// SaveMemo attaches a photo of the memo to a bill, replacing any earlier one.
func (s *Service) SaveMemo(ctx context.Context, userID, projectID, expenseID string, r io.Reader) error {
	if s.files == nil {
		return errors.New("memo storage is not configured")
	}
	head := bufio.NewReaderSize(r, 512)
	sniff, _ := head.Peek(512) // a short file is sniffed from what there is
	ext, ok := memoTypes[http.DetectContentType(sniff)]
	if !ok {
		return messages.Fail(apperr.KindInvalid, codeNotImage)
	}
	return s.write(ctx, userID, projectID, func(tx pgx.Tx, _ project.Membership) error {
		e, err := expenseByID(ctx, tx, projectID, expenseID, false)
		if err != nil {
			return err
		}
		var old *string
		if err = tx.QueryRow(ctx, `SELECT memo_path FROM expenses WHERE id = $1`, e.ID).Scan(&old); err != nil {
			return fmt.Errorf("read memo path: %w", err)
		}
		name := projectID + "/" + e.ID + "." + ext
		if saveErr := s.files.Save(name, head); saveErr != nil {
			return saveErr
		}
		if _, err = tx.Exec(ctx, `UPDATE expenses SET memo_path = $2 WHERE id = $1`, e.ID, name); err != nil {
			return fmt.Errorf("save memo path: %w", err)
		}
		if old != nil && *old != name {
			return s.files.Remove(*old)
		}
		return nil
	})
}

// Memo opens the memo photo of a bill for a member of the project.
func (s *Service) Memo(ctx context.Context, userID, projectID, expenseID string) (io.ReadSeekCloser, string, error) {
	if _, err := s.members.Member(ctx, userID, projectID); err != nil {
		return nil, "", err
	}
	if !isID(expenseID) || s.files == nil {
		return nil, "", notFound()
	}
	var name *string
	err := s.pool.QueryRow(ctx, `SELECT memo_path FROM expenses WHERE id = $1 AND project_id = $2`, expenseID, projectID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && name == nil) {
		return nil, "", notFound()
	}
	if err != nil {
		return nil, "", fmt.Errorf("read memo path: %w", err)
	}
	f, err := s.files.Open(*name)
	return f, *name, err
}

package books

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	// Eleven digits of taka is more than any one entry in these books.
	maxAmount   = 99_999_999_999
	maxNoteLen  = 500
	maxShortLen = 120
)

// text trims s and returns nil for blank, flagging path when longer than limit.
func (f *fields) text(path, s string, limit int) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if utf8.RuneCountInString(s) > limit {
		f.add(path, "text")
	}
	return &s
}

// amount checks a required positive amount.
func (f *fields) amount(path string, n int64, key string) int64 {
	if n <= 0 || n > maxAmount {
		f.add(path, key)
	}
	return n
}

// date parses YYYY-MM-DD; blank means today.
func (f *fields) date(path, s string, today time.Time) time.Time {
	if strings.TrimSpace(s) == "" {
		return today
	}
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		f.add(path, "date")
	}
	return d
}

// ref checks an optional id: blank is nil, anything else must look like an id.
func (f *fields) ref(path, s, key string) *string {
	if s == "" {
		return nil
	}
	if !isID(s) {
		f.add(path, key)
	}
	return &s
}

// isUniqueViolation reports a duplicate-key error from PostgreSQL.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

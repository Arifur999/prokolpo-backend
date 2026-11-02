// Package apperr defines the errors services return when a request cannot be
// served as asked. The HTTP layer maps each Kind to a status; services never
// deal in status codes.
package apperr

import "errors"

// Kind classifies an error by what the caller should do about it.
type Kind int

// Kinds of error. KindUnknown is the zero value and maps to a 500.
const (
	KindUnknown Kind = iota
	KindInvalid
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindTooManyRequests
	KindUnavailable
)

// FieldError points at one invalid input field. Path uses the request's JSON
// names, dotted for nesting ("phases.0.name").
type FieldError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Error is an expected failure whose Message is safe to show to the user.
// Code, when set, is a stable machine-readable name ("ACCOUNT_SUSPENDED") for
// failures the client must handle differently from a plain message.
type Error struct {
	Kind    Kind
	Code    string
	Message string
	Fields  []FieldError
	Err     error
}

// WithCode returns err with code attached, if err is an *Error; other errors
// pass through unchanged.
func WithCode(err error, code string) error {
	e, ok := As(err)
	if !ok {
		return err
	}
	coded := *e
	coded.Code = code
	return &coded
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// Invalid reports input that breaks a rule; fields say which inputs, if known.
func Invalid(message string, fields ...FieldError) error {
	return &Error{Kind: KindInvalid, Message: message, Fields: fields}
}

// Unauthorized reports a missing, expired or wrong credential.
func Unauthorized(message string) error { return &Error{Kind: KindUnauthorized, Message: message} }

// Forbidden reports a known caller who may not do this.
func Forbidden(message string) error { return &Error{Kind: KindForbidden, Message: message} }

// NotFound reports a resource that does not exist or is not the caller's to see.
func NotFound(message string) error { return &Error{Kind: KindNotFound, Message: message} }

// Conflict reports a request that clashes with current state, such as a duplicate.
func Conflict(message string) error { return &Error{Kind: KindConflict, Message: message} }

// TooManyRequests reports a caller who hit a rate limit.
func TooManyRequests(message string) error {
	return &Error{Kind: KindTooManyRequests, Message: message}
}

// Unavailable reports a service that is down on purpose, such as maintenance.
func Unavailable(message string) error { return &Error{Kind: KindUnavailable, Message: message} }

// As returns the *Error in err's chain, if there is one.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

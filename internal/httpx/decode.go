package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// DefaultMaxBodyBytes caps a JSON request body. Forms in this app are small;
// file uploads use their own endpoints and limits.
const DefaultMaxBodyBytes = 1 << 20

// DecodeJSON reads exactly one JSON object with known fields into dst. The
// errors it returns are *apperr.Error, safe to show to the client.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, DefaultMaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	// Anything after the object, even a stray "}", makes the body invalid.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return apperr.Invalid("Request body must be a single JSON object.")
	}
	return nil
}

func decodeError(err error) error {
	var tooLarge *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooLarge):
		return apperr.Invalid("Request body is too large.")
	case errors.As(err, &typeErr) && typeErr.Field != "":
		return apperr.Invalid("Some fields have the wrong type.", apperr.FieldError{Path: typeErr.Field, Message: "Wrong type."})
	case errors.Is(err, io.EOF):
		return apperr.Invalid("Request body is empty.")
	default:
		return apperr.Invalid("Request body must be a single JSON object with known fields.")
	}
}

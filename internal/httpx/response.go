// Package httpx holds the HTTP plumbing every handler shares: the response
// envelope, error mapping, JSON decoding and middleware.
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Arifur999/prokolpo-backend/internal/apperr"
)

// Meta describes one page of a list response.
type Meta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// NewMeta derives the page count from total and limit.
func NewMeta(page, limit, total int) Meta {
	pages := 0
	if limit > 0 {
		pages = (total + limit - 1) / limit
	}
	return Meta{Page: page, Limit: limit, Total: total, TotalPages: pages}
}

type successBody struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data"`
	Meta    *Meta  `json:"meta,omitempty"`
}

// internalErrorMessage is all a client learns about a failure we did not expect.
const internalErrorMessage = "Something went wrong on our side. Please try again."

type errorBody struct {
	Success     bool                `json:"success"`
	Message     string              `json:"message"`
	Code        string              `json:"code,omitempty"`
	ErrorSource []apperr.FieldError `json:"errorSource"`
}

func internalErrorBody() errorBody {
	return errorBody{Message: internalErrorMessage, ErrorSource: []apperr.FieldError{}}
}

// Responder writes every response in the API's envelope.
type Responder struct {
	log *slog.Logger
}

// NewResponder returns a Responder that logs unexpected errors to log.
func NewResponder(log *slog.Logger) *Responder {
	return &Responder{log: log}
}

// JSON writes {success: true, message, data}.
func (rs *Responder) JSON(w http.ResponseWriter, r *http.Request, status int, message string, data any) {
	rs.write(w, r, status, successBody{Success: true, Message: message, Data: data})
}

// List writes {success: true, message, data, meta} for one page of results.
func (rs *Responder) List(w http.ResponseWriter, r *http.Request, message string, data any, meta Meta) {
	rs.write(w, r, http.StatusOK, successBody{Success: true, Message: message, Data: data, Meta: &meta})
}

// Error writes {success: false, message, errorSource}. An *apperr.Error keeps
// its message; anything else is logged here, once, and the client gets a
// generic message with no internals.
func (rs *Responder) Error(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := apperr.As(err)
	if !ok {
		rs.log.ErrorContext(r.Context(), "request failed", "err", err, "route", r.Pattern, "request_id", RequestID(r.Context()))
		rs.write(w, r, http.StatusInternalServerError, internalErrorBody())
		return
	}

	fields := appErr.Fields
	if fields == nil {
		fields = []apperr.FieldError{}
	}
	rs.write(w, r, statusOf(appErr.Kind), errorBody{Message: appErr.Message, Code: appErr.Code, ErrorSource: fields})
}

func statusOf(kind apperr.Kind) int {
	switch kind {
	case apperr.KindInvalid:
		return http.StatusUnprocessableEntity
	case apperr.KindUnauthorized:
		return http.StatusUnauthorized
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindTooManyRequests:
		return http.StatusTooManyRequests
	case apperr.KindUnknown:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// write encodes body before sending anything, so an encoding failure can still
// become a clean 500.
func (rs *Responder) write(w http.ResponseWriter, r *http.Request, status int, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		rs.log.ErrorContext(r.Context(), "encode response", "err", err, "route", r.Pattern, "request_id", RequestID(r.Context()))
		payload, _ = json.Marshal(internalErrorBody()) // a fixed struct of strings always encodes
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(payload) // a failed write means the client is gone; nothing left to tell it
}

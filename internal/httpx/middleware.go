package httpx

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// LogRequests writes one line per request: route template, status, duration.
// The raw path is left out because it can carry ids and tokens.
func LogRequests(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.InfoContext(r.Context(), "http request",
			"method", r.Method,
			"route", r.Pattern,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestID(r.Context()),
		)
	})
}

// RecoverPanics turns a handler panic into a logged 500 in the API's envelope.
// net/http would recover it anyway, but only by dropping the connection.
func RecoverPanics(log *slog.Logger, next http.Handler) http.Handler {
	responder := NewResponder(log)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v) // deliberate abort: let net/http handle it
			}
			log.ErrorContext(r.Context(), "panic serving request", "panic", v, "stack", string(debug.Stack()), "request_id", RequestID(r.Context()))
			responder.write(w, r, http.StatusInternalServerError, internalErrorBody())
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (sr *statusRecorder) Unwrap() http.ResponseWriter { return sr.ResponseWriter }

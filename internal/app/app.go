// Package app wires the API together and runs it until its context ends.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/Arifur999/prokolpo-backend/internal/config"
	"github.com/Arifur999/prokolpo-backend/internal/database"
	"github.com/Arifur999/prokolpo-backend/internal/server"
)

// Run starts the API and blocks until ctx is cancelled or the server fails.
// Its environment comes in as parameters so tests can run it in-process.
// A shutdown triggered by ctx is a clean exit and returns nil.
func Run(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := newLogger(cfg, stdout)

	pool, err := database.Connect(ctx, cfg.DatabaseURL, cfg.DBMaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Handler:           server.New(server.Deps{Log: log, DB: pool}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
		BaseContext:       func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	log.InfoContext(ctx, "http server listening", "addr", ln.Addr().String(), "env", string(cfg.Env))

	serveErr := make(chan error, 1) // buffered: the goroutine can always exit, even after Run returns
	go func() { serveErr <- srv.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	log.InfoContext(ctx, "shutting down", "timeout", cfg.ShutdownTimeout.String())
	// ctx is already cancelled; shutdown gets its own deadline.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		// In-flight requests did not finish in time: cut them off.
		return errors.Join(fmt.Errorf("shutdown http server: %w", err), srv.Close())
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server: %w", err)
	}
	return nil
}

// newLogger writes JSON in production, where logs are shipped and parsed, and
// readable text everywhere else.
func newLogger(cfg config.Config, w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

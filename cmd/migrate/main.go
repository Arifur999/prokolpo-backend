// Command migrate applies or rolls back the API's database migrations.
//
// Usage: migrate [up | down | status | version]
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/pressly/goose/v3"

	"github.com/Arifur999/prokolpo-backend/internal/config"
	"github.com/Arifur999/prokolpo-backend/internal/database"
	"github.com/Arifur999/prokolpo-backend/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "migrate: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := "up"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	// .env is a local convenience; in containers the variables come from the environment.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL, 1)
	if err != nil {
		return err
	}
	defer pool.Close()

	return migrate(ctx, pool, command)
}

func migrate(ctx context.Context, pool *pgxpool.Pool, command string) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	switch command {
	case "up":
		results, err := provider.Up(ctx)
		for _, r := range results {
			fmt.Printf("applied %s (%s)\n", r.Source.Path, r.Duration)
		}
		return err
	case "down":
		r, err := provider.Down(ctx)
		if r != nil {
			fmt.Printf("rolled back %s\n", r.Source.Path)
		}
		return err
	case "status":
		statuses, err := provider.Status(ctx)
		for _, s := range statuses {
			fmt.Printf("%-8s %s\n", s.State, s.Source.Path)
		}
		return err
	case "version":
		v, err := provider.GetDBVersion(ctx)
		fmt.Printf("version %d\n", v)
		return err
	default:
		return fmt.Errorf("unknown command %q (want up, down, status or version)", command)
	}
}

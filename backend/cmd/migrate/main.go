package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/config"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	database, err := config.LoadDatabase()
	if err != nil {
		logger.Error("invalid database configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := postgres.Open(connectContext, database)
	cancel()
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Up(ctx, pool); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations are up to date")
}

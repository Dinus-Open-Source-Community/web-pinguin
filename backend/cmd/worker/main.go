package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/analytics"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/config"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("analytics worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	database, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	interval, err := positiveIntEnv("WORKER_INTERVAL_SECONDS", 60)
	if err != nil {
		return err
	}
	batchSize, err := positiveIntEnv("WORKER_BATCH_SIZE", 5000)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := postgres.Open(ctx, database)
	if err != nil {
		return err
	}
	defer pool.Close()

	worker := analytics.NewWorker(pool)
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	logger.Info("analytics worker started", "interval_seconds", interval, "batch_size", batchSize)

	for {
		started := time.Now().UTC()
		logger.Info("aggregation batch started", "started_at", started)
		processed, err := worker.AggregatePending(ctx, batchSize)
		duration := time.Since(started)
		checkpointCtx, checkpointCancel := context.WithTimeout(context.Background(), 3*time.Second)
		checkpointErr := worker.RecordBatch(checkpointCtx, started, processed, duration, err)
		checkpointCancel()
		if err != nil {
			logger.Error("aggregation batch failed", "error_code", "database_or_aggregation_error", "duration_ms", duration.Milliseconds(), "checkpoint_error", checkpointErr)
		} else {
			logger.Info("aggregation batch completed", "processed", processed, "duration_ms", duration.Milliseconds(), "checkpoint_error", checkpointErr)
		}

		select {
		case <-ctx.Done():
			logger.Info("analytics worker shutting down")
			return nil
		case <-ticker.C:
		}
	}
}

func positiveIntEnv(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 || parsed > 100000 {
		return 0, &configError{message: key + " must be between 1 and 100000"}
	}
	return parsed, nil
}

type configError struct{ message string }

func (e *configError) Error() string { return e.message }

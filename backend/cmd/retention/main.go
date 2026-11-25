package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/config"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/postgres"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/retention"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("retention job stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	db, err := config.LoadDatabase()
	if err != nil {
		return err
	}
	interval, err := setting("RETENTION_INTERVAL_SECONDS", 86400)
	if err != nil {
		return err
	}
	batch, err := setting("RETENTION_BATCH_SIZE", 1000)
	if err != nil {
		return err
	}
	policy := retention.Policy{}
	if policy.RawEventsDays, err = setting("RETENTION_RAW_EVENTS_DAYS", 90); err != nil {
		return err
	}
	if policy.SurveyAnswersDays, err = setting("RETENTION_SURVEY_ANSWERS_DAYS", 365); err != nil {
		return err
	}
	if policy.SurveyContactsDays, err = setting("RETENTION_SURVEY_CONTACTS_DAYS", 90); err != nil {
		return err
	}
	if policy.AuditLogsDays, err = setting("RETENTION_AUDIT_LOGS_DAYS", 365); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := postgres.Open(ctx, db)
	if err != nil {
		return err
	}
	defer pool.Close()
	job, err := retention.New(pool, batch)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()
	logger.Info("retention job started", "interval_seconds", interval, "batch_size", batch, "raw_events_days", policy.RawEventsDays, "survey_answers_days", policy.SurveyAnswersDays, "survey_contacts_days", policy.SurveyContactsDays, "audit_logs_days", policy.AuditLogsDays)
	for {
		started := time.Now()
		runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		counts, runErr := job.Run(runCtx, policy)
		cancel()
		if runErr != nil {
			logger.Error("retention run failed", "duration_ms", time.Since(started).Milliseconds(), "error", runErr)
		} else {
			logger.Info("retention run completed", "duration_ms", time.Since(started).Milliseconds(), "raw_events_deleted", counts.RawEvents, "survey_responses_deleted", counts.SurveyResponses, "survey_responses_retained_for_fulfillment", counts.SurveyResponsesBlockedByFulfillment, "survey_contacts_deleted", counts.SurveyContacts, "idempotency_keys_deleted", counts.IdempotencyKeys, "audit_logs_deleted", counts.AuditLogs)
		}
		select {
		case <-ctx.Done():
			logger.Info("retention job shutting down")
			return nil
		case <-ticker.C:
		}
	}
}

func setting(name string, fallback int) (int, error) {
	v := os.Getenv(name)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 36500 {
		return 0, &configValueError{name}
	}
	return n, nil
}

type configValueError struct{ name string }

func (e *configValueError) Error() string { return e.name + " must be between 1 and 36500" }

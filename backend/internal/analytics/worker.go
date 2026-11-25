package analytics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	pool *pgxpool.Pool
}

type WorkerStatus struct {
	LastStartedAt       *time.Time `json:"last_started_at"`
	LastSucceededAt     *time.Time `json:"last_succeeded_at"`
	LastBatchProcessed  int64      `json:"last_batch_processed"`
	LastBatchDurationMS int64      `json:"last_batch_duration_ms"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastErrorCode       *string    `json:"last_error_code"`
	PendingEvents       int64      `json:"pending_events"`
}

func (w *Worker) RecordBatch(ctx context.Context, started time.Time, processed int64, duration time.Duration, batchErr error) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
	}
	var errorCode any
	if batchErr != nil {
		errorCode = "database_or_aggregation_error"
	}
	_, err := w.pool.Exec(ctx, `INSERT INTO worker_checkpoints
		(stream_id,source_position,last_started_at,last_succeeded_at,last_batch_processed,last_batch_duration_ms,consecutive_failures,last_error_code,updated_at)
		VALUES ('download-aggregator','', $1, CASE WHEN $5 THEN now() ELSE NULL END, $2, $3, CASE WHEN $5 THEN 0 ELSE 1 END, $4, now())
		ON CONFLICT (stream_id) DO UPDATE SET last_started_at=EXCLUDED.last_started_at,
		last_succeeded_at=CASE WHEN $5 THEN now() ELSE worker_checkpoints.last_succeeded_at END,
		last_batch_processed=EXCLUDED.last_batch_processed,last_batch_duration_ms=EXCLUDED.last_batch_duration_ms,
		consecutive_failures=CASE WHEN $5 THEN 0 ELSE worker_checkpoints.consecutive_failures+1 END,
		last_error_code=EXCLUDED.last_error_code,updated_at=now()`, started, processed, duration.Milliseconds(), errorCode, batchErr == nil)
	return err
}

func (w *Worker) Status(ctx context.Context) (WorkerStatus, error) {
	var status WorkerStatus
	err := w.pool.QueryRow(ctx, `SELECT w.last_started_at,w.last_succeeded_at,COALESCE(w.last_batch_processed,0),COALESCE(w.last_batch_duration_ms,0),COALESCE(w.consecutive_failures,0),w.last_error_code
		FROM (VALUES (1)) seed(n) LEFT JOIN worker_checkpoints w ON w.stream_id='download-aggregator'`).Scan(&status.LastStartedAt, &status.LastSucceededAt, &status.LastBatchProcessed, &status.LastBatchDurationMS, &status.ConsecutiveFailures, &status.LastErrorCode)
	if err != nil {
		return status, err
	}
	if err := w.pool.QueryRow(ctx, `SELECT count(*) FROM download_events_raw WHERE aggregated_at IS NULL`).Scan(&status.PendingEvents); err != nil {
		return status, err
	}
	return status, nil
}

func NewWorker(pool *pgxpool.Pool) *Worker {
	return &Worker{pool: pool}
}

func (w *Worker) AggregatePending(ctx context.Context, limit int) (int64, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	result, err := tx.Exec(ctx, `
		WITH batch AS MATERIALIZED (
			SELECT e.id, e.occurred_at, e.file_path, e.bytes_sent, e.completed_estimate,
			       COALESCE(g.region_code, 'ZZ/UNKNOWN') AS region_code, repo.path_prefix
			FROM download_events_raw AS e
			LEFT JOIN geo_regions AS g ON g.id = e.geo_region_id
			LEFT JOIN LATERAL (
				SELECT COALESCE(NULLIF(r.path_prefix,''),r.slug) AS path_prefix
				FROM repositories r
				WHERE e.file_path = COALESCE(NULLIF(r.path_prefix,''),r.slug)
				   OR starts_with(e.file_path, COALESCE(NULLIF(r.path_prefix,''),r.slug) || '/')
				ORDER BY length(COALESCE(NULLIF(r.path_prefix,''),r.slug)) DESC
				LIMIT 1
			) repo ON true
			WHERE e.aggregated_at IS NULL
			ORDER BY e.occurred_at, e.id
			LIMIT $1
			FOR UPDATE OF e SKIP LOCKED
		), grouped AS (
			SELECT date_trunc('hour', occurred_at) AS bucket_start,
			       'hour'::text AS bucket_size,
			       COALESCE(NULLIF(batch.path_prefix, ''), NULLIF(split_part(ltrim(file_path, '/'), '/', 1), ''), '/') AS file_path_prefix,
			       region_code,
			       COUNT(*)::bigint AS request_count,
			       COUNT(*) FILTER (WHERE completed_estimate)::bigint AS completed_count,
			       COALESCE(SUM(bytes_sent), 0)::bigint AS bytes_sent
			FROM batch
			GROUP BY 1, 2, 3, 4
		), upserted AS (
			INSERT INTO download_aggregates
				(bucket_start, bucket_size, file_path_prefix, region_code, request_count, completed_count, bytes_sent)
			SELECT bucket_start, bucket_size, file_path_prefix, region_code, request_count, completed_count, bytes_sent
			FROM grouped
			ON CONFLICT (bucket_start, bucket_size, file_path_prefix, region_code)
			DO UPDATE SET
				request_count = download_aggregates.request_count + EXCLUDED.request_count,
				completed_count = download_aggregates.completed_count + EXCLUDED.completed_count,
				bytes_sent = download_aggregates.bytes_sent + EXCLUDED.bytes_sent,
				updated_at = now()
			RETURNING 1
		)
		UPDATE download_events_raw AS e
		SET aggregated_at = now()
		FROM batch
		WHERE e.id = batch.id
	`, limit)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

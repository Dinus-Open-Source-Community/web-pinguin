DROP INDEX IF EXISTS audit_logs_created_retention_idx;
DROP INDEX IF EXISTS idempotency_expiry_retention_idx;
DROP INDEX IF EXISTS survey_contacts_created_retention_idx;
DROP INDEX IF EXISTS survey_responses_submitted_retention_idx;
DROP INDEX IF EXISTS download_events_created_retention_idx;

ALTER TABLE worker_checkpoints
    DROP COLUMN IF EXISTS last_error_code,
    DROP COLUMN IF EXISTS consecutive_failures,
    DROP COLUMN IF EXISTS last_batch_duration_ms,
    DROP COLUMN IF EXISTS last_batch_processed,
    DROP COLUMN IF EXISTS last_succeeded_at,
    DROP COLUMN IF EXISTS last_started_at;

ALTER TABLE worker_checkpoints
    ADD COLUMN last_started_at timestamptz,
    ADD COLUMN last_succeeded_at timestamptz,
    ADD COLUMN last_batch_processed bigint NOT NULL DEFAULT 0 CHECK (last_batch_processed >= 0),
    ADD COLUMN last_batch_duration_ms bigint NOT NULL DEFAULT 0 CHECK (last_batch_duration_ms >= 0),
    ADD COLUMN consecutive_failures integer NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    ADD COLUMN last_error_code text;

CREATE INDEX download_events_created_retention_idx ON download_events_raw (created_at, id);
CREATE INDEX survey_responses_submitted_retention_idx ON survey_responses (submitted_at, id);
CREATE INDEX survey_contacts_created_retention_idx ON survey_response_contacts (created_at, response_id);
CREATE INDEX idempotency_expiry_retention_idx ON survey_idempotency_keys (expires_at, id);
CREATE INDEX audit_logs_created_retention_idx ON audit_logs (created_at, id);

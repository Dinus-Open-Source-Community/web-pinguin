CREATE TABLE audit_logs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_identifier text,
    action text NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid,
    outcome text NOT NULL CHECK (outcome IN ('success', 'failure')),
    request_id text NOT NULL,
    safe_changes jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_recent_idx ON audit_logs (created_at DESC);
CREATE INDEX audit_logs_actor_recent_idx ON audit_logs (actor_identifier, created_at DESC);
CREATE INDEX audit_logs_resource_recent_idx ON audit_logs (resource_type, resource_id, created_at DESC);

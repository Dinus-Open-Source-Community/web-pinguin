CREATE TABLE geo_regions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    country_code text NOT NULL,
    region_code text NOT NULL,
    country_name text NOT NULL,
    region_name text NOT NULL,
    latitude double precision,
    longitude double precision,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (country_code, region_code),
    CHECK (latitude IS NULL OR latitude BETWEEN -90 AND 90),
    CHECK (longitude IS NULL OR longitude BETWEEN -180 AND 180)
);

INSERT INTO geo_regions (country_code, region_code, country_name, region_name)
VALUES ('ZZ', 'UNKNOWN', 'Unknown', 'Unknown')
ON CONFLICT (country_code, region_code) DO NOTHING;

CREATE TABLE download_events_raw (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_key text NOT NULL UNIQUE,
    source text NOT NULL CHECK (source IN ('portal', 'nginx')),
    occurred_at timestamptz NOT NULL,
    file_path text NOT NULL,
    http_status integer NOT NULL CHECK (http_status BETWEEN 100 AND 599),
    bytes_sent bigint NOT NULL DEFAULT 0 CHECK (bytes_sent >= 0),
    range_requested boolean NOT NULL DEFAULT false,
    completed_estimate boolean NOT NULL DEFAULT false,
    geo_region_id uuid REFERENCES geo_regions(id) ON DELETE SET NULL,
    aggregated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX download_events_recent_idx ON download_events_raw (occurred_at DESC);
CREATE INDEX download_events_portal_recent_idx ON download_events_raw (occurred_at DESC) WHERE source = 'portal';
CREATE INDEX download_events_pending_idx ON download_events_raw (occurred_at) WHERE aggregated_at IS NULL;
CREATE INDEX download_events_path_recent_idx ON download_events_raw (file_path, occurred_at DESC);

CREATE TABLE download_aggregates (
    bucket_start timestamptz NOT NULL,
    bucket_size text NOT NULL CHECK (bucket_size IN ('hour', 'day')),
    file_path_prefix text NOT NULL,
    region_code text NOT NULL DEFAULT 'ZZ/UNKNOWN',
    request_count bigint NOT NULL DEFAULT 0 CHECK (request_count >= 0),
    completed_count bigint NOT NULL DEFAULT 0 CHECK (completed_count >= 0),
    bytes_sent bigint NOT NULL DEFAULT 0 CHECK (bytes_sent >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (bucket_start, bucket_size, file_path_prefix, region_code)
);

CREATE INDEX download_aggregates_time_idx ON download_aggregates (bucket_start, bucket_size);
CREATE INDEX download_aggregates_path_time_idx ON download_aggregates (file_path_prefix, bucket_start);
CREATE INDEX download_aggregates_region_time_idx ON download_aggregates (region_code, bucket_start);

CREATE TABLE worker_checkpoints (
    stream_id text PRIMARY KEY,
    source_position text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

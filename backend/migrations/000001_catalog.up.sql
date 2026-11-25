CREATE TABLE repositories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    distribution text NOT NULL,
    category text NOT NULL CHECK (category IN ('iso', 'package', 'tool')),
    description text,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'hidden', 'retired')),
    mirror_base_url text,
    is_public boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX repositories_category_status_idx ON repositories (category, status);
CREATE INDEX repositories_distribution_idx ON repositories (distribution);

CREATE TABLE repository_releases (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id uuid NOT NULL REFERENCES repositories(id) ON DELETE RESTRICT,
    version text NOT NULL,
    released_at timestamptz,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (repository_id, version),
    UNIQUE (id, repository_id)
);

CREATE INDEX repository_releases_recent_idx ON repository_releases (repository_id, released_at DESC);

CREATE TABLE repository_files (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    repository_id uuid NOT NULL,
    release_id uuid NOT NULL,
    path text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    checksum text,
    checksum_algorithm text,
    download_url text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (release_id, repository_id) REFERENCES repository_releases(id, repository_id) ON DELETE RESTRICT,
    UNIQUE (release_id, path),
    CHECK ((checksum IS NULL) = (checksum_algorithm IS NULL))
);

CREATE INDEX repository_files_release_path_idx ON repository_files (release_id, path);

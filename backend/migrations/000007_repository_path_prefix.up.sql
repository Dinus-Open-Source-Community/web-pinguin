ALTER TABLE repositories
    ADD COLUMN path_prefix text NOT NULL DEFAULT '';

ALTER TABLE repositories
    ADD CONSTRAINT repositories_path_prefix_check
    CHECK (
        path_prefix = '' OR (
            char_length(path_prefix) <= 512
            AND path_prefix ~ '^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$'
        )
    );

CREATE UNIQUE INDEX repositories_path_prefix_unique_idx
    ON repositories (path_prefix)
    WHERE path_prefix <> '';

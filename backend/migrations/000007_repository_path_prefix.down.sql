DROP INDEX IF EXISTS repositories_path_prefix_unique_idx;
ALTER TABLE repositories DROP CONSTRAINT IF EXISTS repositories_path_prefix_check;
ALTER TABLE repositories DROP COLUMN IF EXISTS path_prefix;

package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("repository not found")

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Repository struct {
	ID           string  `json:"id"`
	Slug         string  `json:"slug"`
	PathPrefix   string  `json:"path_prefix"`
	Name         string  `json:"name"`
	Distribution string  `json:"distribution"`
	Category     string  `json:"category"`
	Description  *string `json:"description"`
	Status       string  `json:"status"`
}

type File struct {
	Path              string  `json:"path"`
	SizeBytes         int64   `json:"size_bytes"`
	Checksum          *string `json:"checksum"`
	ChecksumAlgorithm *string `json:"checksum_algorithm"`
	DownloadURL       string  `json:"download_url"`
}

type Release struct {
	Version    string `json:"version"`
	ReleasedAt any    `json:"released_at"`
	Files      []File `json:"files"`
}

type Detail struct {
	Repository
	Releases []Release `json:"releases"`
}

type Filters struct {
	Query, Category, Distribution, Sort, Order string
	Page, PageSize                             int
}

func (s *Store) List(ctx context.Context, f Filters) ([]Repository, int, error) {
	where := `WHERE is_public AND status = 'active'`
	args := []any{}
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	if f.Query != "" {
		p := add("%" + f.Query + "%")
		where += " AND (name ILIKE " + p + " OR slug ILIKE " + p + " OR distribution ILIKE " + p + " OR COALESCE(description, '') ILIKE " + p + ")"
	}
	if f.Category != "" {
		where += " AND category = " + add(f.Category)
	}
	if f.Distribution != "" {
		where += " AND distribution ILIKE " + add(f.Distribution)
	}
	var total int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM repositories "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	orderBy := "name"
	if f.Sort == "updated" {
		orderBy = "updated_at"
	}
	direction := "ASC"
	if strings.EqualFold(f.Order, "desc") {
		direction = "DESC"
	}
	limitArg := add(f.PageSize)
	offsetArg := add((f.Page - 1) * f.PageSize)
	query := "SELECT id, slug, COALESCE(NULLIF(path_prefix,''),slug), name, distribution, category, description, status FROM repositories " + where + " ORDER BY " + orderBy + " " + direction + ", slug ASC LIMIT " + limitArg + " OFFSET " + offsetArg
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]Repository, 0)
	for rows.Next() {
		var item Repository
		if err := rows.Scan(&item.ID, &item.Slug, &item.PathPrefix, &item.Name, &item.Distribution, &item.Category, &item.Description, &item.Status); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) Detail(ctx context.Context, id string) (Detail, error) {
	var result Detail
	err := s.pool.QueryRow(ctx, `SELECT id, slug, COALESCE(NULLIF(path_prefix,''),slug), name, distribution, category, description, status
		FROM repositories WHERE id::text = $1 AND is_public AND status = 'active'`, id).Scan(
		&result.ID, &result.Slug, &result.PathPrefix, &result.Name, &result.Distribution, &result.Category, &result.Description, &result.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Detail{}, ErrNotFound
		}
		return Detail{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT rel.version, rel.released_at, f.path, f.size_bytes,
		f.checksum, f.checksum_algorithm, f.download_url
		FROM repository_releases rel LEFT JOIN repository_files f ON f.release_id = rel.id
		WHERE rel.repository_id = $1::uuid
		ORDER BY rel.released_at DESC NULLS LAST, rel.version DESC, f.path ASC`, id)
	if err != nil {
		return Detail{}, err
	}
	defer rows.Close()
	byVersion := map[string]int{}
	for rows.Next() {
		var version string
		var releasedAt any
		var filePath *string
		var size *int64
		var checksum, algorithm, downloadURL *string
		if err := rows.Scan(&version, &releasedAt, &filePath, &size, &checksum, &algorithm, &downloadURL); err != nil {
			return Detail{}, err
		}
		index, ok := byVersion[version]
		if !ok {
			index = len(result.Releases)
			byVersion[version] = index
			result.Releases = append(result.Releases, Release{Version: version, ReleasedAt: releasedAt, Files: []File{}})
		}
		if filePath != nil {
			result.Releases[index].Files = append(result.Releases[index].Files, File{Path: *filePath, SizeBytes: *size, Checksum: checksum, ChecksumAlgorithm: algorithm, DownloadURL: *downloadURL})
		}
	}
	if err := rows.Err(); err != nil {
		return Detail{}, err
	}
	if result.Releases == nil {
		result.Releases = []Release{}
	}
	return result, nil
}

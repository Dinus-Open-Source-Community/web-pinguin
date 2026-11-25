package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/config"
	"github.com/Dinus-Open-Source-Community/web-pinguin/backend/internal/platform/postgres"
	"github.com/jackc/pgx/v5"
)

type manifest struct {
	Repositories []repository `json:"repositories"`
}
type repository struct {
	Slug          string    `json:"slug"`
	PathPrefix    string    `json:"path_prefix"`
	Name          string    `json:"name"`
	Distribution  string    `json:"distribution"`
	Category      string    `json:"category"`
	Description   *string   `json:"description"`
	Status        string    `json:"status"`
	MirrorBaseURL *string   `json:"mirror_base_url"`
	IsPublic      *bool     `json:"is_public"`
	Releases      []release `json:"releases"`
}
type release struct {
	Version    string     `json:"version"`
	ReleasedAt *time.Time `json:"released_at"`
	Files      []file     `json:"files"`
}
type file struct {
	Path              string  `json:"path"`
	SizeBytes         int64   `json:"size_bytes"`
	Checksum          *string `json:"checksum"`
	ChecksumAlgorithm *string `json:"checksum_algorithm"`
	DownloadURL       string  `json:"download_url"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)
var pathPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*(/[A-Za-z0-9][A-Za-z0-9._-]*)*$`)

const repositoryUpsertSQL = `INSERT INTO repositories(slug,path_prefix,name,distribution,category,description,status,mirror_base_url,is_public)
			VALUES($1,COALESCE(NULLIF($2,''),$1),$3,$4,$5,$6,$7,$8,COALESCE($9,true))
			ON CONFLICT(slug) DO UPDATE SET path_prefix=EXCLUDED.path_prefix,name=EXCLUDED.name,distribution=EXCLUDED.distribution,category=EXCLUDED.category,description=EXCLUDED.description,status=EXCLUDED.status,mirror_base_url=EXCLUDED.mirror_base_url,is_public=EXCLUDED.is_public,updated_at=now()
			WHERE repositories.path_prefix = EXCLUDED.path_prefix OR (repositories.path_prefix = '' AND EXCLUDED.path_prefix = repositories.slug)
			RETURNING id::text`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: catalog-import <manifest.json>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	var input manifest
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil {
		fail(err)
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		fail(errors.New("manifest must contain one JSON value"))
	}
	if err := validate(input); err != nil {
		fail(err)
	}
	db, err := config.LoadDatabase()
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := postgres.Open(ctx, db)
	if err != nil {
		fail(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		fail(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var repos, releases, files int
	for _, r := range input.Repositories {
		var repoID string
		err = tx.QueryRow(ctx, repositoryUpsertSQL, r.Slug, r.PathPrefix, r.Name, r.Distribution, r.Category, r.Description, r.Status, r.MirrorBaseURL, r.IsPublic).Scan(&repoID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				fail(fmt.Errorf("repository %q cannot change its path_prefix through catalog import", r.Slug))
			}
			fail(fmt.Errorf("repository %q: %w", r.Slug, err))
		}
		repos++
		for _, rel := range r.Releases {
			var releaseID string
			err = tx.QueryRow(ctx, `INSERT INTO repository_releases(repository_id,version,released_at) VALUES($1::uuid,$2,$3) ON CONFLICT(repository_id,version) DO UPDATE SET released_at=EXCLUDED.released_at,updated_at=now() RETURNING id::text`, repoID, rel.Version, rel.ReleasedAt).Scan(&releaseID)
			if err != nil {
				fail(fmt.Errorf("repository %q release %q: %w", r.Slug, rel.Version, err))
			}
			releases++
			for _, f := range rel.Files {
				_, err = tx.Exec(ctx, `INSERT INTO repository_files(repository_id,release_id,path,size_bytes,checksum,checksum_algorithm,download_url) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7) ON CONFLICT(release_id,path) DO UPDATE SET size_bytes=EXCLUDED.size_bytes,checksum=EXCLUDED.checksum,checksum_algorithm=EXCLUDED.checksum_algorithm,download_url=EXCLUDED.download_url,updated_at=now()`, repoID, releaseID, f.Path, f.SizeBytes, f.Checksum, f.ChecksumAlgorithm, f.DownloadURL)
				if err != nil {
					fail(fmt.Errorf("repository %q release %q file %q: %w", r.Slug, rel.Version, f.Path, err))
				}
				files++
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		fail(err)
	}
	fmt.Printf("catalog import completed: repositories_processed=%d releases_processed=%d files_processed=%d\n", repos, releases, files)
}

func validate(m manifest) error {
	seen := map[string]bool{}
	seenPathPrefixes := map[string]bool{}
	for i, r := range m.Repositories {
		if !slugPattern.MatchString(r.Slug) || seen[r.Slug] {
			return fmt.Errorf("repository %d has an invalid or duplicate slug", i)
		}
		seen[r.Slug] = true
		pathPrefix := r.PathPrefix
		if pathPrefix == "" {
			pathPrefix = r.Slug
		}
		if len(pathPrefix) > 512 || !pathPrefixPattern.MatchString(pathPrefix) || seenPathPrefixes[pathPrefix] {
			return fmt.Errorf("repository %q has an invalid or duplicate path_prefix", r.Slug)
		}
		seenPathPrefixes[pathPrefix] = true
		if strings.TrimSpace(r.Name) == "" || len([]rune(r.Name)) > 200 || strings.TrimSpace(r.Distribution) == "" || len([]rune(r.Distribution)) > 120 {
			return fmt.Errorf("repository %q requires name and distribution", r.Slug)
		}
		if r.Description != nil && len([]rune(*r.Description)) > 5000 {
			return fmt.Errorf("repository %q description exceeds 5000 characters", r.Slug)
		}
		if r.Category != "iso" && r.Category != "package" && r.Category != "tool" {
			return fmt.Errorf("repository %q has an invalid category", r.Slug)
		}
		if r.Status != "active" && r.Status != "hidden" && r.Status != "retired" {
			return fmt.Errorf("repository %q has an invalid status", r.Slug)
		}
		if r.MirrorBaseURL != nil && !validURL(*r.MirrorBaseURL) {
			return fmt.Errorf("repository %q has an invalid mirror_base_url", r.Slug)
		}
		versions := map[string]bool{}
		for _, rel := range r.Releases {
			if strings.TrimSpace(rel.Version) == "" || versions[rel.Version] {
				return fmt.Errorf("repository %q has an empty or duplicate release version", r.Slug)
			}
			versions[rel.Version] = true
			paths := map[string]bool{}
			for _, f := range rel.Files {
				clean := path.Clean(f.Path)
				if f.Path == "" || strings.Contains(f.Path, "\\") || strings.HasPrefix(clean, "../") || path.IsAbs(f.Path) || clean != f.Path || (f.Path != pathPrefix && !strings.HasPrefix(f.Path, pathPrefix+"/")) || f.SizeBytes < 0 || !validURL(f.DownloadURL) {
					return fmt.Errorf("repository %q release %q has an invalid file", r.Slug, rel.Version)
				}
				if paths[f.Path] {
					return fmt.Errorf("repository %q release %q repeats file path", r.Slug, rel.Version)
				}
				paths[f.Path] = true
				if (f.Checksum == nil) != (f.ChecksumAlgorithm == nil) {
					return fmt.Errorf("repository %q release %q checksum and algorithm must be set together", r.Slug, rel.Version)
				}
			}
		}
	}
	return nil
}
func validURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	return err == nil && u.IsAbs() && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil && u.Fragment == ""
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

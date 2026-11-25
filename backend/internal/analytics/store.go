package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DownloadEvent struct {
	ID                string
	OccurredAt        time.Time
	FilePath          string
	HTTPStatus        int
	BytesSent         int64
	RangeRequested    bool
	CompletedEstimate bool
}

type Summary struct {
	Period             string    `json:"period"`
	GeneratedAt        time.Time `json:"generated_at"`
	DownloadRequests   int64     `json:"download_requests"`
	CompletedDownloads int64     `json:"completed_downloads"`
	BandwidthBytes     int64     `json:"bandwidth_bytes"`
	PopularFiles       []TopFile `json:"popular_files"`
}

type TopFile struct {
	Path     string `json:"path"`
	Requests int64  `json:"requests"`
	Bytes    int64  `json:"bytes_sent"`
}

type TimeseriesPoint struct {
	BucketStart        time.Time `json:"bucket_start"`
	DownloadRequests   int64     `json:"download_requests"`
	CompletedDownloads int64     `json:"completed_downloads"`
	BytesSent          int64     `json:"bytes_sent"`
}

type Store struct {
	pool *pgxpool.Pool
}

type StatsFilter struct {
	RepositoryID string
	Distribution string
}

type RegionAggregate struct {
	Region    string   `json:"region"`
	Country   string   `json:"country"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Downloads int64    `json:"downloads"`
	Period    string   `json:"period"`
}

const minPublicRegionDownloads int64 = 5

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) RecordDownload(ctx context.Context, event DownloadEvent) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO download_events_raw
			(id, source_key, source, occurred_at, file_path, http_status, bytes_sent, range_requested, completed_estimate)
		VALUES ($1::uuid, $1::text, 'portal', $2, $3, $4, $5, $6, $7)
	`, event.ID, event.OccurredAt, event.FilePath, event.HTTPStatus, event.BytesSent, event.RangeRequested, event.CompletedEstimate)
	return err
}

type NginxEvent struct {
	SourceKey   string
	OccurredAt  time.Time
	FilePath    string
	HTTPStatus  int
	BytesSent   int64
	CountryCode string
	CountryName string
	RegionCode  string
	RegionName  string
	Latitude    *float64
	Longitude   *float64
}

// ImportNginxEvent deduplicates by a stable source key and keeps no client IP.
func (s *Store) ImportNginxEvent(ctx context.Context, event NginxEvent) error {
	country := strings.ToUpper(event.CountryCode)
	if len(country) != 2 || country < "AA" || country > "ZZ" {
		country = "ZZ"
	}
	countryName, regionCode, regionName := event.CountryName, event.RegionCode, event.RegionName
	if country == "ZZ" || countryName == "" || regionCode == "" || regionName == "" {
		country, countryName, regionCode, regionName = "ZZ", "Unknown", "UNKNOWN", "Unknown"
	}
	var regionID string
	err := s.pool.QueryRow(ctx, `INSERT INTO geo_regions (country_code,country_name,region_code,region_name,latitude,longitude)
		VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (country_code,region_code) DO UPDATE SET
		country_name=EXCLUDED.country_name, region_name=EXCLUDED.region_name,
		latitude=COALESCE(EXCLUDED.latitude,geo_regions.latitude),longitude=COALESCE(EXCLUDED.longitude,geo_regions.longitude)
		RETURNING id::text`, country, countryName, regionCode, regionName, event.Latitude, event.Longitude).Scan(&regionID)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO download_events_raw
		(source_key, source, occurred_at, file_path, http_status, bytes_sent, completed_estimate, geo_region_id)
		VALUES ($1, 'nginx', $2, $3, $4, $5, $6, $7::uuid) ON CONFLICT (source_key) DO NOTHING`,
		event.SourceKey, event.OccurredAt, event.FilePath, event.HTTPStatus, event.BytesSent,
		event.HTTPStatus == 200, regionID)
	return err
}

func NginxSourceKey(path string, offset int64, line []byte) string {
	seed := []byte(path + ":" + strconv.FormatInt(offset, 10) + ":")
	sum := sha256.Sum256(append(seed, line...))
	return hex.EncodeToString(sum[:])
}

func (s *Store) Summary(ctx context.Context, period string, since time.Time, filter StatsFilter) (Summary, error) {
	result := Summary{Period: period, GeneratedAt: time.Now().UTC(), PopularFiles: []TopFile{}}
	where := `e.occurred_at >= $1 AND e.source IN ('portal', 'nginx')
		AND (($2 = '' AND $3 = '') OR EXISTS (
			SELECT 1 FROM repositories r
			WHERE r.is_public AND r.status = 'active'
			  AND (e.file_path = COALESCE(NULLIF(r.path_prefix,''),r.slug)
			       OR starts_with(e.file_path, COALESCE(NULLIF(r.path_prefix,''),r.slug) || '/'))
			  AND ($2 = '' OR r.id::text = $2)
			  AND ($3 = '' OR lower(r.distribution) = lower($3))
		))`
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE completed_estimate),
		       COALESCE(SUM(bytes_sent), 0)
		FROM download_events_raw e
		WHERE `+where, since, filter.RepositoryID, filter.Distribution).Scan(&result.DownloadRequests, &result.CompletedDownloads, &result.BandwidthBytes)
	if err != nil {
		return Summary{}, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT e.file_path, COUNT(*) AS requests, COALESCE(SUM(e.bytes_sent), 0) AS bytes_sent
		FROM download_events_raw e
		WHERE `+where+`
		GROUP BY e.file_path
		ORDER BY requests DESC, file_path ASC
		LIMIT 10
	`, since, filter.RepositoryID, filter.Distribution)
	if err != nil {
		return Summary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var file TopFile
		if err := rows.Scan(&file.Path, &file.Requests, &file.Bytes); err != nil {
			return Summary{}, err
		}
		result.PopularFiles = append(result.PopularFiles, file)
	}
	return result, rows.Err()
}

func (s *Store) Timeseries(ctx context.Context, period string, since time.Time, filter StatsFilter) ([]TimeseriesPoint, error) {
	unit := "hour"
	if period != "24h" {
		unit = "day"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT date_trunc($1, e.occurred_at) AS bucket_start,
		       COUNT(*) AS requests,
	       COUNT(*) FILTER (WHERE e.completed_estimate) AS completed,
	       COALESCE(SUM(e.bytes_sent), 0) AS bytes_sent
		FROM download_events_raw e
		WHERE e.occurred_at >= $2 AND e.source IN ('portal', 'nginx')
		  AND (($3 = '' AND $4 = '') OR EXISTS (
			SELECT 1 FROM repositories r
			WHERE r.is_public AND r.status = 'active'
			  AND (e.file_path = COALESCE(NULLIF(r.path_prefix,''),r.slug)
			       OR starts_with(e.file_path, COALESCE(NULLIF(r.path_prefix,''),r.slug) || '/'))
			  AND ($3 = '' OR r.id::text = $3)
			  AND ($4 = '' OR lower(r.distribution) = lower($4))
		))
		GROUP BY bucket_start
		ORDER BY bucket_start
	`, unit, since, filter.RepositoryID, filter.Distribution)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]TimeseriesPoint, 0)
	for rows.Next() {
		var point TimeseriesPoint
		if err := rows.Scan(&point.BucketStart, &point.DownloadRequests, &point.CompletedDownloads, &point.BytesSent); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (s *Store) Regions(ctx context.Context, period string, since time.Time, filter StatsFilter) ([]RegionAggregate, error) {
	rows, err := s.pool.Query(ctx, `
		WITH regional_counts AS (
			SELECT g.country_code, g.region_code, g.country_name, g.region_name,
			       g.latitude, g.longitude, COUNT(*)::bigint AS downloads
			FROM download_events_raw e
			JOIN geo_regions g ON g.id = e.geo_region_id
			WHERE e.occurred_at >= $1 AND e.source IN ('portal', 'nginx')
			  AND g.country_code <> 'ZZ'
			  AND (($2 = '' AND $3 = '') OR EXISTS (
				SELECT 1 FROM repositories r WHERE r.is_public AND r.status = 'active'
				  AND (e.file_path = COALESCE(NULLIF(r.path_prefix,''),r.slug)
				       OR starts_with(e.file_path, COALESCE(NULLIF(r.path_prefix,''),r.slug) || '/'))
				  AND ($2 = '' OR r.id::text = $2) AND ($3 = '' OR lower(r.distribution) = lower($3))
			  ))
			GROUP BY g.country_code, g.region_code, g.country_name, g.region_name, g.latitude, g.longitude
		), visible_regions AS (
			SELECT region_name AS region, country_name AS country, latitude, longitude, downloads
			FROM regional_counts WHERE downloads >= $4
		), suppressed_regions AS (
			SELECT 'Other'::text AS region, country_name AS country,
			       NULL::double precision AS latitude, NULL::double precision AS longitude,
			       SUM(downloads)::bigint AS downloads
			FROM regional_counts WHERE downloads < $4
			GROUP BY country_code, country_name
			HAVING SUM(downloads) >= $4
		)
		SELECT region, country, latitude, longitude, downloads FROM visible_regions
		UNION ALL
		SELECT region, country, latitude, longitude, downloads FROM suppressed_regions
		ORDER BY downloads DESC, country, region
	`, since, filter.RepositoryID, filter.Distribution, minPublicRegionDownloads)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]RegionAggregate, 0)
	for rows.Next() {
		var item RegionAggregate
		if err := rows.Scan(&item.Region, &item.Country, &item.Latitude, &item.Longitude, &item.Downloads); err != nil {
			return nil, err
		}
		item.Period = period
		items = append(items, item)
	}
	return items, rows.Err()
}

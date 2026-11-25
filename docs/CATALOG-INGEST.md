# Repository catalog ingest

Metadata is sourced from a reviewed JSON manifest maintained by the mirror
operator. The repository does not currently have a verified upstream catalog
feed or live production manifest, so this importer does not invent mirror
records or fetch arbitrary remote metadata.

## Manifest shape

```json
{
  "repositories": [
    {
      "slug": "replace-with-path-slug",
      "path_prefix": "iso/replace-with-path-slug",
      "name": "Display name",
      "distribution": "Distribution name",
      "category": "iso",
      "description": "Optional description",
      "status": "active",
      "mirror_base_url": "https://mirror.example.org/path/",
      "is_public": true,
      "releases": [
        {
          "version": "release-name",
          "released_at": "2026-01-01T00:00:00Z",
          "files": [
            {
              "path": "iso/replace-with-path-slug/release/file.iso",
              "size_bytes": 123,
              "checksum": "hex-digest",
              "checksum_algorithm": "sha256",
              "download_url": "https://mirror.example.org/path/release/file.iso"
            }
          ]
        }
      ]
    }
  ]
}
```

`slug`, `path_prefix`, name, distribution, category, status, release version,
and file URL/path are validated. `path_prefix` is a normalized relative path
under the mirror root; it defaults to `slug` for older manifests. Categories
are `iso`, `package`, or `tool`; statuses are
`active`, `hidden`, or `retired`. Checksums and algorithms must be supplied
together. File paths must be normalized relative paths, URLs must be absolute
HTTP(S), and release/file keys cannot repeat inside one manifest.

Run `go run ./cmd/catalog-import path/to/catalog.json` from `backend` with the
usual `DB_*` variables set. The whole manifest is one transaction. Re-running
upserts repository by slug, release by repository/version, and file by
release/path. Records missing from the input remain unchanged; operators must
set removed repositories to `hidden` or `retired` explicitly. The command
reports processed repository/release/file counts and the failing repository/release/path, but
does not log descriptions, checksums, URLs with credentials, or response data.
An existing slug cannot silently change its `path_prefix` during a routine
manifest import; coordinate such a path migration explicitly so existing event
mapping and aggregate history are reviewed. CI imports the checked-in snapshot
twice and confirms the repeated import is idempotent, then verifies a prefix
change is rejected without altering the stored catalog.

## Path and ownership rules

Statistics associate an event with a repository when its path equals the
repository `path_prefix` or starts with `path_prefix/`. This supports nested
mirror trees such as `/iso/debian/debian13/file.iso`: use slug `debian` and
`path_prefix: "iso/debian"`. The API keeps the public slug independent from the
filesystem path. Confirm a sample from the mirror filesystem, catalog file
path, and access log agree before publishing the repository.

The mirror operator owns the manifest source and review. A catalog update is
complete when the validated manifest is imported and representative paths are
checked against the mirror. Keep the manifest in the operator's deployment or
configuration repository; do not edit catalog rows manually in PostgreSQL.

## Current verified seed

[`../backend/catalog/pinguin-public-snapshot.json`](../backend/catalog/pinguin-public-snapshot.json)
is a seed snapshot captured from the public Pinguin mirror on 2026-11-23/24. It
contains 89 visible top-level ISO directories with observed path prefixes;
release and file metadata is populated only for the three Debian 13 files
verified in the mirror listing. Their byte sizes came from successful HTTP HEAD
responses. Checksums and release dates were not advertised, so they remain
null. Other directories deliberately have no releases or files until their
listings are reviewed. This seed is not a catalog sync job; refresh and review
it against the operator's current mirror listing before each production catalog
update.

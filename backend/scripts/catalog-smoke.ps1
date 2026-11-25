$ErrorActionPreference = 'Stop'

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 10)
$dbName = "pinguin_catalog_check_$suffix"
$slug = "catalog-smoke-$suffix"
$manifestPath = Join-Path $env:TEMP "pinguin-catalog-smoke-$suffix.json"
$dbCreated = $false

Get-Content ..\.env | ForEach-Object {
    if ($_ -match '^([^=]+)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
if ($env:DB_USER -ne $env:POSTGRES_USER) { throw 'Use the PostgreSQL owner as DB_USER for this isolated catalog test.' }

try {
    docker exec web-pinguin-postgres createdb -U $env:POSTGRES_USER $dbName
    if ($LASTEXITCODE -ne 0) { throw 'Could not create isolated catalog test database.' }
    $dbCreated = $true
    $env:DB_NAME = $dbName
    $env:DB_USER = $env:POSTGRES_USER
    $env:DB_PASSWORD = $env:POSTGRES_PASSWORD
    $env:DB_SSLMODE = 'disable'
    go run ./cmd/migrate
    if ($LASTEXITCODE -ne 0) { throw 'Catalog test database migration failed.' }

    $manifest = @{
        repositories = @(@{
            slug = $slug
            path_prefix = "iso/$slug"
            name = 'Catalog smoke repository'
            distribution = 'Smoke Linux'
            category = 'tool'
            description = 'Synthetic fixture for idempotent import verification.'
            status = 'active'
            mirror_base_url = "https://mirror.example.invalid/$slug/"
            is_public = $true
            releases = @(@{
                version = '1.0'
                released_at = [DateTime]::UtcNow.ToString('o')
                files = @(@{
                    path = "iso/$slug/1.0/tool.tar.gz"
                    size_bytes = 4096
                    checksum = ('a' * 64)
                    checksum_algorithm = 'sha256'
                    download_url = "https://mirror.example.invalid/$slug/1.0/tool.tar.gz"
                })
            })
        })
    } | ConvertTo-Json -Depth 10 -Compress
    Set-Content -LiteralPath $manifestPath -Value $manifest -Encoding utf8
    go run ./cmd/catalog-import $manifestPath
    if ($LASTEXITCODE -ne 0) { throw 'First catalog import failed.' }
    go run ./cmd/catalog-import $manifestPath
    if ($LASTEXITCODE -ne 0) { throw 'Second catalog import failed.' }

    $repoCount = docker exec web-pinguin-postgres psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM repositories WHERE slug='$slug'"
    $releaseCount = docker exec web-pinguin-postgres psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM repository_releases r JOIN repositories p ON p.id=r.repository_id WHERE p.slug='$slug'"
    $fileCount = docker exec web-pinguin-postgres psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM repository_files f JOIN repositories p ON p.id=f.repository_id WHERE p.slug='$slug'"
    if ($LASTEXITCODE -ne 0 -or [int]$repoCount -ne 1 -or [int]$releaseCount -ne 1 -or [int]$fileCount -ne 1) {
        throw "Catalog idempotency check failed: repositories=$repoCount releases=$releaseCount files=$fileCount"
    }
    Write-Output 'Catalog ingest smoke passed: repeated manifest import kept one repository, release, and file.'
}
finally {
    if ($dbCreated) { docker exec web-pinguin-postgres dropdb --if-exists -U $env:POSTGRES_USER $dbName | Out-Null }
    if (Test-Path -LiteralPath $manifestPath) { Remove-Item -LiteralPath $manifestPath -Force }
}

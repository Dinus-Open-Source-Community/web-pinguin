$ErrorActionPreference = 'Stop'

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 10)
$sourceDb = "pinguin_backup_src_$suffix"
$restoreDb = "restore_pinguin_$suffix"
$container = 'web-pinguin-postgres'
$dumpInContainer = "/tmp/pinguin-backup-smoke-$suffix.dump"
$dumpOnHost = Join-Path $env:TEMP "pinguin-backup-smoke-$suffix.dump"
$sourceCreated = $false
$restoreCreated = $false

Get-Content ..\.env | ForEach-Object {
    if ($_ -match '^([^=]+)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
if ($env:DB_USER -ne $env:POSTGRES_USER) { throw 'Use the PostgreSQL owner as DB_USER for this isolated restore test.' }

try {
    docker exec $container createdb -U $env:POSTGRES_USER $sourceDb
    if ($LASTEXITCODE -ne 0) { throw 'Could not create isolated source database.' }
    $sourceCreated = $true
    $env:DB_NAME = $sourceDb
    $env:DB_USER = $env:POSTGRES_USER
    $env:DB_PASSWORD = $env:POSTGRES_PASSWORD
    $env:DB_SSLMODE = 'disable'
    go run ./cmd/migrate
    if ($LASTEXITCODE -ne 0) { throw 'Source database migration failed.' }

    $seed = "INSERT INTO repositories(slug,name,distribution,category,status) VALUES ('restore-smoke-$suffix','restore smoke','fixture','tool','active')"
    docker exec $container psql -v ON_ERROR_STOP=1 -U $env:POSTGRES_USER -d $sourceDb -c $seed | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not seed source database.' }
    docker exec $container sh -ec "pg_dump -U '$env:POSTGRES_USER' -d '$sourceDb' -Fc -f '$dumpInContainer'"
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL dump failed.' }
    docker cp "${container}:$dumpInContainer" $dumpOnHost | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not copy isolated dump to the host temp directory.' }

    docker exec $container createdb -U $env:POSTGRES_USER $restoreDb
    if ($LASTEXITCODE -ne 0) { throw 'Could not create isolated restore database.' }
    $restoreCreated = $true
    docker cp $dumpOnHost "${container}:$dumpInContainer" | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not copy dump into the isolated database container.' }
    docker exec $container pg_restore -U $env:POSTGRES_USER -d $restoreDb --no-owner --no-privileges $dumpInContainer
    if ($LASTEXITCODE -ne 0) { throw 'Restore into isolated database failed.' }

    $migrationCount = docker exec $container psql -At -U $env:POSTGRES_USER -d $restoreDb -c 'SELECT count(*) FROM schema_migrations'
    if ($LASTEXITCODE -ne 0 -or [int]$migrationCount -ne 6) { throw "Restored migration count mismatch: $migrationCount" }
    $catalogCount = docker exec $container psql -At -U $env:POSTGRES_USER -d $restoreDb -c "SELECT count(*) FROM repositories WHERE slug='restore-smoke-$suffix'"
    if ($LASTEXITCODE -ne 0 -or [int]$catalogCount -ne 1) { throw 'Restored catalog fixture was not found.' }
    Write-Output 'PostgreSQL backup/restore smoke passed: schema and fixture verified in separate database.'
}
finally {
    docker exec $container rm -f $dumpInContainer | Out-Null
    if ($sourceCreated) { docker exec $container dropdb --if-exists -U $env:POSTGRES_USER $sourceDb | Out-Null }
    if ($restoreCreated) { docker exec $container dropdb --if-exists -U $env:POSTGRES_USER $restoreDb | Out-Null }
    if (Test-Path -LiteralPath $dumpOnHost) { Remove-Item -LiteralPath $dumpOnHost -Force }
}

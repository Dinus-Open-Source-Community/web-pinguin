$ErrorActionPreference = 'Stop'

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 10)
$dbName = "pinguin_nginx_check_$suffix"
$slug = "nginx-smoke-$suffix"
$logPath = Join-Path $env:TEMP "pinguin-nginx-smoke-$suffix.log"
$statePath = Join-Path $env:TEMP "pinguin-nginx-smoke-$suffix.state.json"
$container = 'web-pinguin-postgres'
$dbCreated = $false
$apiProcess = $null
$apiPort = 18084
$apiExe = Join-Path $env:TEMP "pinguin-nginx-api-$suffix.exe"
$apiOut = Join-Path $env:TEMP "pinguin-nginx-api-$suffix.log"
$apiErr = Join-Path $env:TEMP "pinguin-nginx-api-$suffix.err"
$workerExe = Join-Path $env:TEMP "pinguin-nginx-worker-$suffix.exe"
$workerOut = Join-Path $env:TEMP "pinguin-nginx-worker-$suffix.log"
$workerErr = Join-Path $env:TEMP "pinguin-nginx-worker-$suffix.err"
$workerProcess = $null
$listingDir = Join-Path $env:TEMP "pinguin-nginx-listing-$suffix"
$encoding = [System.Text.UTF8Encoding]::new($false)
$allOutput = [System.Collections.Generic.List[string]]::new()

Get-Content ..\.env | ForEach-Object {
    if ($_ -match '^([^=]+)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
if ($env:DB_USER -ne $env:POSTGRES_USER) { throw 'Use the PostgreSQL owner as DB_USER for this isolated Nginx test.' }

function New-LogLine([int]$Seconds, [int]$Bytes, [string]$ClientIP = '10.0.0.8') {
    $time = [DateTime]::UtcNow.AddSeconds($Seconds).ToString("yyyy-MM-dd'T'HH':'mm':'ss'Z'", [Globalization.CultureInfo]::InvariantCulture)
    return "$time 200 $Bytes /iso/$slug/1.0/file.iso $ClientIP"
}

function Import-Log([bool]$MayFail = $false) {
    $output = go run ./cmd/nginx-import 2>&1
    $exit = $LASTEXITCODE
    foreach ($line in $output) { $allOutput.Add([string]$line) }
    if ($MayFail) {
        if ($exit -eq 0) { throw 'Malformed Nginx line unexpectedly succeeded.' }
    } elseif ($exit -ne 0) {
        throw "Nginx import failed: $($output -join ' ')"
    }
    return ($output -join "`n")
}

function Event-Count {
    $value = docker exec $container psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM download_events_raw WHERE source='nginx'"
    if ($LASTEXITCODE -ne 0) { throw 'Could not query imported event count.' }
    return [int]$value
}

try {
    docker exec $container createdb -U $env:POSTGRES_USER $dbName
    if ($LASTEXITCODE -ne 0) { throw 'Could not create isolated Nginx test database.' }
    $dbCreated = $true
    $env:DB_NAME = $dbName
    $env:DB_USER = $env:POSTGRES_USER
    $env:DB_PASSWORD = $env:POSTGRES_PASSWORD
    $env:DB_SSLMODE = 'disable'
    $env:NGINX_ACCESS_LOG = $logPath
    $env:NGINX_STATE_PATH = $statePath
    $geoFixture = Join-Path $PSScriptRoot '..\internal\geolocation\testdata\GeoLite2-City-Test.mmdb'
    $env:GEOIP_DB_PATH = (Resolve-Path $geoFixture).Path
    go run ./cmd/migrate
    if ($LASTEXITCODE -ne 0) { throw 'Nginx test database migration failed.' }
    $seed = "INSERT INTO repositories(slug,path_prefix,name,distribution,category,status) VALUES ('$slug','iso/$slug','Nginx smoke repository','Smoke Linux','tool','active')"
    docker exec $container psql -v ON_ERROR_STOP=1 -U $env:POSTGRES_USER -d $dbName -c $seed | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not seed repository path fixture.' }

    [IO.File]::WriteAllText($logPath, (New-LogLine 0 8192 '81.2.69.160') + "`r`n", $encoding)
    [void](Import-Log)
    if ((Event-Count) -ne 1) { throw 'Initial Nginx line was not imported.' }
    $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    $initialOffset = [int64]$state.offset
    [void](Import-Log)
    if ((Event-Count) -ne 1) { throw 'Restart duplicated an imported Nginx event.' }

    [IO.File]::AppendAllText($logPath, (New-LogLine 1 1024) + "`r`n", $encoding)
    [void](Import-Log)
    if ((Event-Count) -ne 2) { throw 'Appended Nginx line was not imported.' }
    $oldOffset = [int64]((Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json).offset)

    $replacement = @((New-LogLine 2 2048 '81.2.69.160'), (New-LogLine 3 4096)) -join "`r`n"
    [IO.File]::WriteAllText($logPath, $replacement + "`r`n", $encoding)
    if ((Get-Item -LiteralPath $logPath).Length -lt $oldOffset) { throw 'Rotation fixture must be at least as long as the prior checkpoint.' }
    $rotationOutput = Import-Log
    if ($rotationOutput -notmatch 'restarting at byte zero' -or (Event-Count) -ne 4) { throw 'Same-size-or-larger log replacement was not detected and reimported.' }

    $partialLine = New-LogLine 4 512
    [IO.File]::AppendAllText($logPath, $partialLine.Substring(0, 20), $encoding)
    [void](Import-Log)
    $partialOffset = [int64]((Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json).offset)
    if ($partialOffset -ne (Get-Item -LiteralPath $logPath).Length - 20 -or (Event-Count) -ne 4) { throw 'Partial trailing line advanced the checkpoint or was imported.' }
    [IO.File]::AppendAllText($logPath, $partialLine.Substring(20) + "`r`n", $encoding)
    [void](Import-Log)
    if ((Event-Count) -ne 5) { throw 'Completed trailing line was not imported after restart.' }

    $beforeBadLine = [int64]((Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json).offset)
    [IO.File]::AppendAllText($logPath, "malformed`r`n", $encoding)
    $failureOutput = Import-Log $true
    $afterBad = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
    if ([int64]$afterBad.offset -ne $beforeBadLine -or [int]$afterBad.lines_failed -ne 1 -or $failureOutput -notmatch 'offset was not advanced' -or $failureOutput -notmatch 'permanent_input_error') { throw 'Malformed line did not retain its safe checkpoint or report the permanent input error class.' }

    $prefix = [IO.File]::ReadAllText($logPath, $encoding).Substring(0, [int]$beforeBadLine)
    [IO.File]::WriteAllText($logPath, $prefix + (New-LogLine 5 256) + "`r`n", $encoding)
    [void](Import-Log)
    [void](Import-Log)
    if ((Event-Count) -ne 6) { throw 'Corrected row was lost or restart duplicated Nginx events.' }

    $prefixCount = docker exec $container psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM download_events_raw WHERE source='nginx' AND starts_with(file_path,'iso/$slug/')"
    $unknownCount = docker exec $container psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM download_events_raw e JOIN geo_regions g ON g.id=e.geo_region_id WHERE e.source='nginx' AND g.country_code='ZZ' AND g.region_code='UNKNOWN'"
    $plainIPCount = docker exec $container psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM download_events_raw WHERE source='nginx' AND (source_key || file_path) LIKE '%10.0.0.8%'"
    $knownPublicGeoCount = docker exec $container psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT count(*) FROM download_events_raw e JOIN geo_regions g ON g.id=e.geo_region_id WHERE e.source='nginx' AND g.country_code='GB' AND g.region_code='ENG'"
    if ($LASTEXITCODE -ne 0 -or [int]$prefixCount -ne 6 -or [int]$unknownCount -ne 4 -or [int]$knownPublicGeoCount -ne 2 -or [int]$plainIPCount -ne 0) { throw 'Imported event mapping, public/private IP handling, or privacy check failed.' }

    $env:WORKER_INTERVAL_SECONDS = '60'
    $env:WORKER_BATCH_SIZE = '100'
    go build -o $workerExe ./cmd/worker
    if ($LASTEXITCODE -ne 0) { throw 'Could not build analytics worker for path-prefix check.' }
    $workerProcess = Start-Process -FilePath $workerExe -WorkingDirectory (Get-Location).Path -WindowStyle Hidden -PassThru -RedirectStandardOutput $workerOut -RedirectStandardError $workerErr
    $aggregateCount = 0
    for ($i = 0; $i -lt 30; $i++) {
        $aggregateCount = [int](docker exec $container psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT COALESCE(sum(request_count),0) FROM download_aggregates WHERE file_path_prefix='iso/$slug'")
        if ($aggregateCount -eq 6) { break }
        Start-Sleep -Seconds 1
    }
    if ($aggregateCount -ne 6) { throw 'Worker did not aggregate all nested-prefix events under the repository path.' }
    Stop-Process -Id $workerProcess.Id -Force
    $workerProcess = $null

    New-Item -ItemType Directory -Path $listingDir | Out-Null
    $env:LISTING_DIR = $listingDir
    $env:API_ADDR = "127.0.0.1:$apiPort"
    go build -o $apiExe .
    if ($LASTEXITCODE -ne 0) { throw 'Could not build API for Nginx statistics check.' }
    $apiProcess = Start-Process -FilePath $apiExe -WorkingDirectory (Get-Location).Path -WindowStyle Hidden -PassThru -RedirectStandardOutput $apiOut -RedirectStandardError $apiErr
    $base = "http://127.0.0.1:$apiPort"
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $response = Invoke-WebRequest -Uri "$base/health/live" -TimeoutSec 2
            if ($response.StatusCode -eq 200) { $ready = $true; break }
        } catch { Start-Sleep -Seconds 1 }
    }
    if (-not $ready) { throw 'API did not become live for Nginx statistics check.' }
    $repositoryList = (Invoke-WebRequest -Uri "$base/api/v1/repositories?q=$slug&page=1&page_size=10" -TimeoutSec 10).Content | ConvertFrom-Json
    $catalogItem = $repositoryList.data | Where-Object { $_.slug -eq $slug } | Select-Object -First 1
    if (-not $catalogItem -or $catalogItem.path_prefix -ne "iso/$slug") { throw 'Catalog API did not expose the nested repository path prefix.' }
    $repoID = docker exec $container psql -At -U $env:POSTGRES_USER -d $dbName -c "SELECT id FROM repositories WHERE slug='$slug'"
    $filter = "repository=$repoID&distribution=Smoke%20Linux"
    $summary = (Invoke-WebRequest -Uri "$base/api/v1/stats/summary?period=24h&$filter" -TimeoutSec 10).Content | ConvertFrom-Json
    $timeseries = (Invoke-WebRequest -Uri "$base/api/v1/stats/timeseries?period=24h&$filter" -TimeoutSec 10).Content | ConvertFrom-Json
    $requestsInSeries = ($timeseries.data | Measure-Object -Property download_requests -Sum).Sum
    if ($summary.data.download_requests -ne 6 -or $summary.data.bandwidth_bytes -ne 16128 -or $requestsInSeries -ne 6) { throw 'Nginx-imported events are missing from repository/distribution statistics.' }

    $outputText = $allOutput -join "`n"
    if ($outputText.Contains('10.0.0.8') -or $outputText.Contains('81.2.69.160')) { throw 'Importer logs exposed the client IP.' }
    Write-Output 'Nginx importer/API smoke passed: GeoIP lookup, private-IP privacy, restart/rotation, partial/malformed recovery, nested path-prefix mapping, and repository-filtered summary/timeseries.'
}
finally {
    if ($workerProcess -and -not $workerProcess.HasExited) { Stop-Process -Id $workerProcess.Id -Force }
    if ($apiProcess -and -not $apiProcess.HasExited) { Stop-Process -Id $apiProcess.Id -Force }
    if ($dbCreated) { docker exec $container dropdb --if-exists -U $env:POSTGRES_USER $dbName | Out-Null }
    foreach ($path in @($logPath, $statePath, "$statePath.tmp")) {
        if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
    }
    foreach ($path in @($apiExe, $apiOut, $apiErr, $workerExe, $workerOut, $workerErr)) { if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force } }
    if (Test-Path -LiteralPath $listingDir) { Remove-Item -LiteralPath $listingDir -Recurse -Force }
}

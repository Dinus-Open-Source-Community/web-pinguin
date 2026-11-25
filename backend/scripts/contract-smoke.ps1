param([switch]$UseConfiguredJwtSecret)
$ErrorActionPreference = 'Stop'

$stamp = Get-Date -Format 'yyyyMMddHHmmss'
$dbName = "pinguin_runtime_check_$stamp"
$token = $null
$previous = $null
$exe = Join-Path $env:TEMP "pinguin-contract-api-$stamp.exe"
$log = Join-Path $env:TEMP "pinguin-contract-api-$stamp.log"
$errLog = Join-Path $env:TEMP "pinguin-contract-api-$stamp.err"
$apiProcess = $null
$dbCreated = $false

Get-Content ..\.env | ForEach-Object {
    if ($_ -match '^([^=]+)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}

try {
    docker exec web-pinguin-postgres createdb -U $env:POSTGRES_USER $dbName
    if ($LASTEXITCODE -ne 0) { throw 'Could not create isolated API test database.' }
    $dbCreated = $true
    $env:DB_NAME = $dbName
    $env:DB_USER = $env:POSTGRES_USER
    $env:DB_PASSWORD = $env:POSTGRES_PASSWORD
    $env:DB_SSLMODE = 'disable'

    go run ./cmd/migrate
    if ($LASTEXITCODE -ne 0) { throw 'Migration failed in isolated API test database.' }
    go build -o $exe .
    if ($LASTEXITCODE -ne 0) { throw 'API build failed.' }

    $env:LISTING_DIR = (Resolve-Path ..\mirror-sample).Path
    if (-not $UseConfiguredJwtSecret) {
        $env:JWT_SECRET = 'runtime-current-signing-secret-0123456789abcdef0123456789'
    } elseif ([string]::IsNullOrWhiteSpace($env:JWT_SECRET) -or [Text.Encoding]::UTF8.GetByteCount($env:JWT_SECRET) -lt 32) {
        throw 'Configured JWT_SECRET must contain at least 32 bytes.'
    }
    $env:JWT_SECRET_PREVIOUS = 'runtime-previous-signing-secret-0123456789abcdef0123456789'
    $env:JWT_ISSUER = 'web-pinguin'
    $env:JWT_AUDIENCE = 'web-pinguin-admin'
    $token = (go run ./cmd/admin-jwt -subject runtime-current).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Could not issue current JWT for smoke test.' }
    $previous = (go run ./cmd/admin-jwt -subject runtime-previous -use-previous).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Could not issue previous JWT for smoke test.' }
    $managerToken = (go run ./cmd/admin-jwt -subject runtime-manager -roles survey_manager).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Could not issue survey-manager JWT for smoke test.' }
    $readerToken = (go run ./cmd/admin-jwt -subject runtime-reader -roles response_reader).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Could not issue response-reader JWT for smoke test.' }
    $fulfillmentToken = (go run ./cmd/admin-jwt -subject runtime-fulfillment -roles fulfillment_manager).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Could not issue fulfillment-manager JWT for smoke test.' }
    $auditToken = (go run ./cmd/admin-jwt -subject runtime-auditor -roles audit_reader).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'Could not issue audit-reader JWT for smoke test.' }
    $env:API_ADDR = '127.0.0.1:18081'
    $apiProcess = Start-Process -FilePath $exe -WorkingDirectory (Get-Location).Path -WindowStyle Hidden -PassThru -RedirectStandardOutput $log -RedirectStandardError $errLog
    $base = 'http://127.0.0.1:18081'
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $response = Invoke-WebRequest -Uri "$base/health/live" -TimeoutSec 2
            if ($response.StatusCode -eq 200) { $ready = $true; break }
        } catch { Start-Sleep -Seconds 1 }
    }
    if (-not $ready) { throw 'API did not become live.' }

    function CallApi([string]$Method, [string]$Path, [hashtable]$Headers = @{}, [string]$Body = $null) {
        $params = @{ Method = $Method; Uri = ($base + $Path); Headers = $Headers; SkipHttpErrorCheck = $true; TimeoutSec = 10 }
        if ($null -ne $Body) { $params.Body = $Body; $params.ContentType = 'application/json' }
        Invoke-WebRequest @params
    }

    $cases = @(
        @{ Name = 'readiness'; Response = (CallApi GET '/health/ready'); Want = 200 },
        @{ Name = 'public catalog'; Response = (CallApi GET '/api/v1/repositories?page=1&page_size=5'); Want = 200 },
        @{ Name = 'invalid period'; Response = (CallApi GET '/api/v1/stats/summary?period=invalid'); Want = 400 },
        @{ Name = 'missing repository'; Response = (CallApi GET '/api/v1/repositories/00000000-0000-4000-8000-000000000000'); Want = 404 },
        @{ Name = 'admin without token'; Response = (CallApi GET '/api/v1/admin/surveys'); Want = 401 },
        @{ Name = 'worker without token'; Response = (CallApi GET '/api/v1/admin/workers/analytics'); Want = 401 },
        @{ Name = 'worker with current token'; Response = (CallApi GET '/api/v1/admin/workers/analytics' @{ Authorization = "Bearer $token" }); Want = 200 },
        @{ Name = 'worker with previous token'; Response = (CallApi GET '/api/v1/admin/workers/analytics' @{ Authorization = "Bearer $previous" }); Want = 200 }
        @{ Name = 'admin survey listing with manager role'; Response = (CallApi GET '/api/v1/admin/surveys' @{ Authorization = "Bearer $managerToken" }); Want = 200 }
        @{ Name = 'survey create denied to response reader'; Response = (CallApi POST '/api/v1/admin/surveys' @{ Authorization = "Bearer $readerToken" } '{}'); Want = 403 }
        @{ Name = 'fulfillment update denied to response reader'; Response = (CallApi PATCH '/api/v1/admin/survey-options/00000000-0000-4000-8000-000000000001/fulfillment' @{ Authorization = "Bearer $readerToken" } '{}'); Want = 403 }
    )
    foreach ($case in $cases) {
        if ($case.Response.StatusCode -ne $case.Want) { throw "$($case.Name): expected $($case.Want), received $($case.Response.StatusCode)" }
    }
    $invalidDraft = CallApi POST '/api/v1/admin/surveys' @{ Authorization = "Bearer $token" } '{}'
    if ($invalidDraft.StatusCode -ne 422) { throw "invalid admin body expected 422, got $($invalidDraft.StatusCode)" }
    $start = [DateTime]::UtcNow.AddMinutes(-2).ToString('o')
    $end = [DateTime]::UtcNow.AddDays(1).ToString('o')
    $draft = @{ title = 'Runtime contract smoke'; starts_at = $start; ends_at = $end; questions = @(@{ label = 'Mirror preference'; type = 'short_text'; required = $true }) } | ConvertTo-Json -Depth 6 -Compress
    $created = CallApi POST '/api/v1/admin/surveys' @{ Authorization = "Bearer $token" } $draft
    if ($created.StatusCode -ne 201) { throw "valid survey creation expected 201, got $($created.StatusCode): $($created.Content); fixture=$draft" }
    $surveyId = ($created.Content | ConvertFrom-Json).data.id
    $published = CallApi POST "/api/v1/admin/surveys/$surveyId/publish" @{ Authorization = "Bearer $token" }
    if ($published.StatusCode -ne 200) { throw "publish expected 200, got $($published.StatusCode)" }
    $active = CallApi GET '/api/v1/surveys/active'
    if ($active.StatusCode -ne 200) { throw 'active survey request failed.' }
    $questionId = ($active.Content | ConvertFrom-Json).data.questions[0].id
    $submission = @{ answers = @(@{ question_id = $questionId; text = 'Debian' }) } | ConvertTo-Json -Depth 5 -Compress
    $idem = "runtime-smoke-$stamp-000001"
    $submitted = CallApi POST "/api/v1/surveys/$surveyId/responses" @{ 'Idempotency-Key' = $idem } $submission
    $replayed = CallApi POST "/api/v1/surveys/$surveyId/responses" @{ 'Idempotency-Key' = $idem } $submission
    $conflictBody = @{ answers = @(@{ question_id = $questionId; text = 'Fedora' }) } | ConvertTo-Json -Depth 5 -Compress
    $conflict = CallApi POST "/api/v1/surveys/$surveyId/responses" @{ 'Idempotency-Key' = $idem } $conflictBody
    $missingSurvey = CallApi POST '/api/v1/surveys/00000000-0000-4000-8000-000000000000/responses' @{ 'Idempotency-Key' = "runtime-missing-$stamp-0001" } '{"answers":[{"question_id":"00000000-0000-4000-8000-000000000001","text":"x"}]}'
    foreach ($check in @(@{ Name = 'submit'; Response = $submitted; Want = 201 }, @{ Name = 'replay'; Response = $replayed; Want = 201 }, @{ Name = 'idempotency conflict'; Response = $conflict; Want = 409 }, @{ Name = 'missing survey'; Response = $missingSurvey; Want = 404 })) {
        if ($check.Response.StatusCode -ne $check.Want) { throw "$($check.Name): expected $($check.Want), received $($check.Response.StatusCode)" }
    }

    $responseList = CallApi GET "/api/v1/admin/surveys/$surveyId/responses" @{ Authorization = "Bearer $token" }
    if ($responseList.StatusCode -ne 200) { throw 'admin response list failed.' }
    $readerResponseList = CallApi GET "/api/v1/admin/surveys/$surveyId/responses" @{ Authorization = "Bearer $readerToken" }
    if ($readerResponseList.StatusCode -ne 200) { throw 'response-reader role was denied response access.' }
    $readerAudit = CallApi GET '/api/v1/admin/audit-logs' @{ Authorization = "Bearer $readerToken" }
    if ($readerAudit.StatusCode -ne 403) { throw 'response-reader role unexpectedly accessed audit logs.' }
    $auditorAudit = CallApi GET '/api/v1/admin/audit-logs' @{ Authorization = "Bearer $auditToken" }
    if ($auditorAudit.StatusCode -ne 200) { throw 'audit-reader role was denied audit access.' }
    $fulfillmentInvalid = CallApi PATCH '/api/v1/admin/survey-options/00000000-0000-4000-8000-000000000001/fulfillment' @{ Authorization = "Bearer $fulfillmentToken" } '{}'
    if ($fulfillmentInvalid.StatusCode -ne 422) { throw "fulfillment-manager role did not reach fulfillment validation: got $($fulfillmentInvalid.StatusCode)" }
    $audit = CallApi GET '/api/v1/admin/audit-logs?actor_id=runtime-current&action=read_survey_responses' @{ Authorization = "Bearer $token" }
    if ($audit.StatusCode -ne 200 -or (($audit.Content | ConvertFrom-Json).pagination.total -lt 1)) { throw 'admin identity/action audit record was not written.' }
    Write-Output 'Runtime API contract smoke passed: catalog list/detail prefixes, auth rotation, public/admin flows, validation, idempotency, not-found, audit identity.'
}
finally {
    if ($apiProcess -and -not $apiProcess.HasExited) { Stop-Process -Id $apiProcess.Id -Force }
    if ($dbCreated) { docker exec web-pinguin-postgres dropdb --if-exists -U $env:POSTGRES_USER $dbName | Out-Null }
}

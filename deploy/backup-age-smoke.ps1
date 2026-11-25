param([string]$AgeToolDirectory = '')

$ErrorActionPreference = 'Stop'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$envFile = Join-Path $PSScriptRoot '.env.production.example'
$composeFile = Join-Path $PSScriptRoot 'compose.production.yaml'
$gitDirectory = Split-Path -Parent (Get-Command git.exe).Source
$bashExe = [IO.Path]::GetFullPath((Join-Path $gitDirectory '..\bin\bash.exe'))
if (-not (Test-Path -LiteralPath $bashExe)) { throw 'Git Bash is required to run the deployment shell scripts.' }
$ageCommand = if ($AgeToolDirectory) { Join-Path $AgeToolDirectory 'age.exe' } else { (Get-Command age.exe).Source }
$ageKeygen = if ($AgeToolDirectory) { Join-Path $AgeToolDirectory 'age-keygen.exe' } else { (Get-Command age-keygen.exe).Source }
if (-not (Test-Path -LiteralPath $ageCommand) -or -not (Test-Path -LiteralPath $ageKeygen)) { throw 'Install age and age-keygen, or pass -AgeToolDirectory.' }

$suffix = [Guid]::NewGuid().ToString('N').Substring(0, 10)
$projectName = "pinguin_backup_smoke_$suffix"
$restoreDb = "restore_age_$suffix"
$deployRoot = [IO.Path]::GetFullPath($PSScriptRoot)
$taskTemp = Join-Path $deployRoot ".backup-age-smoke-$suffix"
$backupDir = Join-Path $taskTemp 'backups'
$identity = Join-Path $taskTemp 'backup.agekey'
$backupDirRelative = "deploy/.backup-age-smoke-$suffix/backups"
$identityRelative = "deploy/.backup-age-smoke-$suffix/backup.agekey"
$dbStarted = $false
$restoreCreated = $false
$container = ''

$envLines = Get-Content -LiteralPath $envFile
$userLine = $envLines | Where-Object { $_ -match '^POSTGRES_USER=' } | Select-Object -Last 1
$databaseLine = $envLines | Where-Object { $_ -match '^POSTGRES_DB=' } | Select-Object -Last 1
if (-not $userLine -or -not $databaseLine) { throw 'Production env example is missing PostgreSQL database settings.' }
$dbUser = $userLine.Substring('POSTGRES_USER='.Length)
$dbName = $databaseLine.Substring('POSTGRES_DB='.Length)

New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
$ageDirectory = Split-Path -Parent $ageCommand
$env:Path = "$ageDirectory;$env:Path"
$env:COMPOSE_PROJECT_NAME = $projectName
& $ageKeygen -o $identity *> $null
if ($LASTEXITCODE -ne 0) { throw 'Could not generate an ephemeral age identity.' }
$env:BACKUP_AGE_RECIPIENT = (& $ageKeygen -y $identity).Trim()
if ($LASTEXITCODE -ne 0 -or -not $env:BACKUP_AGE_RECIPIENT.StartsWith('age1')) { throw 'Could not derive the ephemeral age recipient.' }
$env:BACKUP_AGE_IDENTITY = $identityRelative

Push-Location $repoRoot
try {
    docker compose --env-file $envFile -f $composeFile up -d postgres
    if ($LASTEXITCODE -ne 0) { throw 'Could not start isolated production PostgreSQL.' }
    $dbStarted = $true
    $container = (docker compose --env-file $envFile -f $composeFile ps -q postgres).Trim()
    if (-not $container) { throw 'Could not locate isolated PostgreSQL container.' }
    $healthy = $false
    for ($i = 0; $i -lt 30; $i++) {
        if ((docker inspect $container --format '{{.State.Health.Status}}') -eq 'healthy') { $healthy = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $healthy) { throw 'Isolated PostgreSQL did not become healthy.' }

    $seed = "CREATE TABLE backup_smoke_marker(id integer PRIMARY KEY, payload text NOT NULL); INSERT INTO backup_smoke_marker VALUES (1, 'age-encrypted-restore-ok');"
    docker exec $container psql -v ON_ERROR_STOP=1 -U $dbUser -d $dbName -c $seed | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Could not seed the backup smoke marker.' }

    $backupOutput = & $bashExe deploy/backup.sh deploy/.env.production.example $backupDirRelative 2>&1
    if ($LASTEXITCODE -ne 0) { throw "Encrypted backup command failed: $($backupOutput -join ' ')" }
    $backupLine = $backupOutput | Where-Object { $_ -match '^encrypted backup written: ' } | Select-Object -Last 1
    if (-not $backupLine) { throw 'Backup command did not report its output file.' }
    $backupPath = $backupLine -replace '^encrypted backup written: ', ''
    if (-not (Test-Path -LiteralPath $backupPath) -or (Get-Item -LiteralPath $backupPath).Length -lt 100) { throw 'Encrypted backup file is missing or unexpectedly small.' }

    & $bashExe deploy/restore-test.sh $backupPath $restoreDb deploy/.env.production.example
    if ($LASTEXITCODE -ne 0) { throw 'Encrypted restore command failed.' }
    $restoreCreated = $true
    $actual = (docker exec $container psql -At -U $dbUser -d $restoreDb -c 'SELECT payload FROM backup_smoke_marker WHERE id=1').Trim()
    if ($LASTEXITCODE -ne 0 -or $actual -ne 'age-encrypted-restore-ok') { throw 'Restored database marker did not match the source.' }
    Write-Output 'Age-encrypted backup and isolated PostgreSQL restore smoke passed.'
}
finally {
    if ($restoreCreated -and $container) { docker exec $container dropdb --if-exists -U $dbUser $restoreDb | Out-Null }
    if ($dbStarted) { docker compose --env-file $envFile -f $composeFile down -v --remove-orphans | Out-Null }
    if ([IO.Path]::GetFullPath($taskTemp).StartsWith($deployRoot + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
        Remove-Item -LiteralPath $taskTemp -Recurse -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:BACKUP_AGE_RECIPIENT -ErrorAction SilentlyContinue
    Remove-Item Env:BACKUP_AGE_IDENTITY -ErrorAction SilentlyContinue
    Remove-Item Env:COMPOSE_PROJECT_NAME -ErrorAction SilentlyContinue
}

<#
.SYNOPSIS
  KleinAI - bring up local dev environment.

.DESCRIPTION
  - Start docker compose dev stack: MySQL(13306) + Redis(16379)
  - Wait for MySQL healthy
  - Spawn 4 backend cmd windows (api / admin / openai / worker)
  - Print frontend dev hints

.EXAMPLE
  pwsh ./scripts/dev-up.ps1
  powershell.exe -ExecutionPolicy Bypass -File ./scripts/dev-up.ps1
#>

[CmdletBinding()]
param(
  [switch]$NoBackend
)

$ErrorActionPreference = 'Stop'
$ROOT = Split-Path -Parent $PSScriptRoot

Write-Host '==============================================' -ForegroundColor Cyan
Write-Host ' KleinAI dev environment' -ForegroundColor Cyan
Write-Host '==============================================' -ForegroundColor Cyan

# ---------- 1. docker compose ----------
Write-Host ''
Write-Host '[1/4] starting MySQL + Redis containers ...' -ForegroundColor Yellow
Push-Location (Join-Path $ROOT 'deploy')
try {
  docker compose -f docker-compose.dev.yml up -d
  if ($LASTEXITCODE -ne 0) { throw 'docker compose up failed' }
} finally {
  Pop-Location
}

# ---------- 2. wait mysql ----------
Write-Host ''
Write-Host '[2/4] waiting for MySQL to become healthy ...' -ForegroundColor Yellow
$ready = $false
for ($i = 1; $i -le 60; $i++) {
  $state = docker inspect --format '{{json .State.Health.Status}}' klein-mysql-dev 2>$null
  if ($state -match 'healthy') {
    Write-Host ('  -> healthy after ' + $i + 's') -ForegroundColor Green
    $ready = $true
    break
  }
  Start-Sleep -Seconds 1
}
if (-not $ready) {
  Write-Warning 'MySQL did not become healthy in 60s. Run: docker logs klein-mysql-dev'
}

# ---------- 3. backend env ----------
$envFile = Join-Path $ROOT 'backend/.env.local'
if (-not (Test-Path $envFile)) {
  Write-Host ''
  Write-Host '[3/4] copying backend/.env.example -> backend/.env.local ...' -ForegroundColor Yellow
  Copy-Item (Join-Path $ROOT 'backend/.env.example') $envFile
  Write-Host '  -> created backend/.env.local (edit later if needed)' -ForegroundColor Green
} else {
  Write-Host ''
  Write-Host '[3/4] backend/.env.local already exists, keep as is.' -ForegroundColor Yellow
}

# ---------- 4. backend cmd ----------
if (-not $NoBackend) {
  Write-Host ''
  Write-Host '[4/4] launching 4 backend cmd processes (each in its own window) ...' -ForegroundColor Yellow

  $shellExe = 'powershell.exe'
  if (Get-Command pwsh -ErrorAction SilentlyContinue) { $shellExe = 'pwsh' }

  $cmds = @(
    @{ Name = 'api';    Path = './cmd/api' },
    @{ Name = 'admin';  Path = './cmd/admin' },
    @{ Name = 'openai'; Path = './cmd/openai' },
    @{ Name = 'worker'; Path = './cmd/worker' }
  )

  $backendPath = (Join-Path $ROOT 'backend').Replace('\','/')

  foreach ($c in $cmds) {
    $title = 'klein-' + $c.Name
    $bodyLines = @(
      ('$Host.UI.RawUI.WindowTitle = "' + $title + '"'),
      ('Set-Location "' + $backendPath + '"'),
      'Get-Content .env.local | ForEach-Object {',
      '  if ($_ -match "^[^#].*?=") {',
      '    $kv = $_ -split "=", 2',
      '    [Environment]::SetEnvironmentVariable($kv[0].Trim(), $kv[1].Trim())',
      '  }',
      '}',
      ('Write-Host ">>> klein ' + $c.Name + '" -ForegroundColor Cyan'),
      ('go run ' + $c.Path)
    )
    $body = $bodyLines -join "`n"
    Start-Process $shellExe -ArgumentList @('-NoLogo','-NoExit','-ExecutionPolicy','Bypass','-Command', $body) | Out-Null
    Start-Sleep -Milliseconds 600
  }

  Write-Host '  -> 4 backend windows spawned' -ForegroundColor Green
} else {
  Write-Host ''
  Write-Host '[4/4] -NoBackend specified, skipping backend launch.' -ForegroundColor DarkGray
}

# ---------- 5. frontend hint ----------
Write-Host ''
Write-Host '----------------------------------------------' -ForegroundColor Cyan
Write-Host ' Next steps - frontend dev servers' -ForegroundColor Cyan
Write-Host '----------------------------------------------' -ForegroundColor Cyan
Write-Host @'

  user app (Vite 5173):
    cd frontend
    pnpm install                          # first time only
    pnpm --filter @kleinai/user dev

  admin app (Vite 5174):
    cd frontend
    pnpm --filter @kleinai/admin dev

  Open in browser:
    user      : http://localhost:5173
    admin     : http://localhost:5174     (default admin / admin123)
    openai v1 : http://localhost:17200/v1/images/generations

  Stop dependency containers:
    pwsh ./scripts/dev-down.ps1

'@ -ForegroundColor White

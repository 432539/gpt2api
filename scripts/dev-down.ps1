<#
.SYNOPSIS
  Stop KleinAI local dev dependency containers (MySQL / Redis).

.DESCRIPTION
  - Default: docker compose down, keeps volumes
  - With -Volumes: also removes data volumes (DB will re-init next up)

.EXAMPLE
  pwsh ./scripts/dev-down.ps1
  pwsh ./scripts/dev-down.ps1 -Volumes
#>

[CmdletBinding()]
param(
  [switch]$Volumes
)

$ErrorActionPreference = 'Stop'
$ROOT = Split-Path -Parent $PSScriptRoot

Push-Location (Join-Path $ROOT 'deploy')
try {
  if ($Volumes) {
    Write-Host 'stopping containers and removing volumes ...' -ForegroundColor Yellow
    docker compose -f docker-compose.dev.yml down -v
  } else {
    Write-Host 'stopping containers (volumes preserved) ...' -ForegroundColor Yellow
    docker compose -f docker-compose.dev.yml down
  }
} finally {
  Pop-Location
}
Write-Host 'done.' -ForegroundColor Green

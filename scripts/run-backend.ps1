<#
.SYNOPSIS
  Helper: load backend/.env.local then run one go cmd.

.EXAMPLE
  pwsh ./scripts/run-backend.ps1 ./cmd/api
#>
param(
  [Parameter(Mandatory = $true)][string]$CmdPath
)
$ErrorActionPreference = 'Stop'
$ROOT = Split-Path -Parent $PSScriptRoot
Set-Location (Join-Path $ROOT 'backend')
$envFile = Join-Path $ROOT 'backend/.env.local'
if (-not (Test-Path $envFile)) { throw "missing $envFile" }
Get-Content $envFile | ForEach-Object {
  $line = $_.Trim()
  if (-not $line -or $line.StartsWith('#')) { return }
  $idx = $line.IndexOf('=')
  if ($idx -lt 1) { return }
  $k = $line.Substring(0, $idx).Trim()
  $v = $line.Substring($idx + 1).Trim()
  $hash = $v.IndexOf(' #')
  if ($hash -gt 0) { $v = $v.Substring(0, $hash).Trim() }
  [Environment]::SetEnvironmentVariable($k, $v)
}
Write-Host ">>> launching $CmdPath" -ForegroundColor Cyan
go run $CmdPath

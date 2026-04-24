# Windows 本地预构建脚本
# 用法:
#   powershell -NoProfile -File deploy/build-local.ps1
#   powershell -NoProfile -File deploy/build-local.ps1 -Arch arm64
#   $env:TARGETARCH='amd64'; powershell -NoProfile -File deploy/build-local.ps1
#   powershell -NoProfile -File deploy/build-local.ps1 -Force

param(
    [switch]$Force,
    [string]$Arch = $env:TARGETARCH
)

$ErrorActionPreference = 'Stop'
# PowerShell 7:关掉 "native 命令 stderr 自动触发终结" 的坑
if ($PSVersionTable.PSVersion.Major -ge 7) {
    $PSNativeCommandUseErrorActionPreference = $false
}

$root = Resolve-Path "$PSScriptRoot/.."
Set-Location $root

function Resolve-TargetArch {
    param([string]$Value)

    if ([string]::IsNullOrWhiteSpace($Value)) {
        $Value = (go env GOHOSTARCH).Trim()
    }

    switch ($Value.ToLowerInvariant()) {
        'amd64' { return 'amd64' }
        'x86_64' { return 'amd64' }
        'x64' { return 'amd64' }
        'arm64' { return 'arm64' }
        'aarch64' { return 'arm64' }
        default { throw "unsupported arch: $Value. use -Arch amd64|arm64 or set TARGETARCH." }
    }
}

$targetArch = Resolve-TargetArch $Arch

Write-Host "[build-local] repo  = $root"
Write-Host "[build-local] target= linux/$targetArch"
Write-Host "[build-local] step1 = cross-build gpt2api (linux/$targetArch)"
$env:GOOS = "linux"
$env:GOARCH = $targetArch
$env:CGO_ENABLED = "0"
New-Item -ItemType Directory -Force deploy/bin | Out-Null
go build -buildvcs=false -ldflags "-s -w" -o deploy/bin/gpt2api ./cmd/server
if ($LASTEXITCODE -ne 0) { throw "gpt2api build failed" }
[System.IO.File]::WriteAllText((Join-Path $root "deploy/bin/.gpt2api_arch"), $targetArch)

$goosePath = Join-Path $root "deploy/bin/goose"
$gooseArchFile = Join-Path $root "deploy/bin/.goose_arch"
$needGooseBuild = $Force -or -not (Test-Path $goosePath)
if (-not $needGooseBuild) {
    if (-not (Test-Path $gooseArchFile)) {
        $needGooseBuild = $true
    } else {
        $existingArch = (Get-Content $gooseArchFile -Raw).Trim()
        if ($existingArch -ne $targetArch) {
            $needGooseBuild = $true
        }
    }
}

if ($needGooseBuild) {
    Write-Host "[build-local] step2 = cross-build goose (linux/$targetArch, tmp module)"
    $tmp = Join-Path $env:TEMP "gpt2api-goose-src"
    if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
    New-Item -ItemType Directory -Force $tmp | Out-Null
    Push-Location $tmp
    try {
        cmd /c "go mod init goose-wrapper >nul 2>&1"
        cmd /c "go get github.com/pressly/goose/v3/cmd/goose@v3.20.0 >nul 2>&1"
        go build -buildvcs=false -ldflags "-s -w" -o $goosePath github.com/pressly/goose/v3/cmd/goose
        if ($LASTEXITCODE -ne 0) { throw "goose build failed" }
        [System.IO.File]::WriteAllText($gooseArchFile, $targetArch)
    } finally {
        Pop-Location
        Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
    }
} else {
    Write-Host "[build-local] step2 = skip goose (linux/$targetArch exists). use -Force to rebuild"
}

Write-Host "[build-local] step3 = npm run build (web)"
Push-Location (Join-Path $root "web")
try {
    $needNodeInstall = -not (Test-Path node_modules)
    if (-not $needNodeInstall) {
        $nodeModulesTime = (Get-Item node_modules).LastWriteTimeUtc
        if ((Get-Item package.json).LastWriteTimeUtc -gt $nodeModulesTime) {
            $needNodeInstall = $true
        } elseif ((Test-Path package-lock.json) -and (Get-Item package-lock.json).LastWriteTimeUtc -gt $nodeModulesTime) {
            $needNodeInstall = $true
        }
    }
    if ($needNodeInstall) {
        if (Test-Path package-lock.json) {
            Write-Host "[build-local] step3a = npm ci (deps changed or node_modules missing)"
            npm ci --no-audit --no-fund --loglevel=error
            if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
        } else {
            Write-Host "[build-local] step3a = npm install (node_modules missing)"
            npm install --no-audit --no-fund --loglevel=error
            if ($LASTEXITCODE -ne 0) { throw "npm install failed" }
        }
    }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }
} finally {
    Pop-Location
}

Write-Host "[build-local] done. artifacts:"
Get-Item deploy/bin/gpt2api, deploy/bin/goose, web/dist/index.html | Format-Table -AutoSize

#!/usr/bin/env bash
# Linux 本地预构建脚本(服务器上直接用 / WSL / macOS 均可)
#
# 用法:
#   bash deploy/build-local.sh                  # 默认编译当前宿主对应的 linux/<arch>
#   bash deploy/build-local.sh --arch arm64    # 指定目标架构
#   TARGETARCH=amd64 bash deploy/build-local.sh
#   bash deploy/build-local.sh --force         # 强制重建 goose
#
# 产物:
#   deploy/bin/gpt2api        linux/<target arch> 可执行(后端)
#   deploy/bin/goose          linux/<target arch> 可执行(迁移工具)
#   web/dist/                 前端 Vite 产物
#
# 这套产物 + deploy/Dockerfile 就可以离线构建镜像,无需容器再访问外网。

set -euo pipefail

FORCE=0
TARGETARCH="${TARGETARCH:-}"

normalize_arch() {
    case "${1:-}" in
        amd64|x86_64) echo "amd64" ;;
        arm64|aarch64) echo "arm64" ;;
        *)
            echo "[build-local] unsupported arch: ${1:-<empty>} (use amd64 or arm64)" >&2
            exit 1
            ;;
    esac
}

while (($# > 0)); do
    case "$1" in
        -f|--force)
            FORCE=1
            shift
            ;;
        --arch)
            if (($# < 2)); then
                echo "[build-local] --arch requires a value" >&2
                exit 1
            fi
            TARGETARCH="$2"
            shift 2
            ;;
        --arch=*)
            TARGETARCH="${1#*=}"
            shift
            ;;
        *)
            echo "[build-local] unknown argument: $1" >&2
            exit 1
            ;;
    esac
done

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "[build-local] repo  = $ROOT"
if [ -n "$TARGETARCH" ]; then
    TARGETARCH="$(normalize_arch "$TARGETARCH")"
else
    TARGETARCH="$(normalize_arch "$(uname -m)")"
fi
echo "[build-local] target= linux/${TARGETARCH}"

# ---- step1: 交叉编译 gpt2api ----
echo "[build-local] step1 = cross-build gpt2api (linux/${TARGETARCH})"
mkdir -p deploy/bin
GOOS=linux GOARCH="${TARGETARCH}" CGO_ENABLED=0 \
    go build -buildvcs=false -ldflags "-s -w" -o deploy/bin/gpt2api ./cmd/server
printf '%s\n' "${TARGETARCH}" > deploy/bin/.gpt2api_arch

# ---- step2: 编译 goose ----
GOOSE="$ROOT/deploy/bin/goose"
GOOSE_ARCH_FILE="$ROOT/deploy/bin/.goose_arch"
GOOSE_NEEDS_BUILD=0
if [ "$FORCE" = "1" ] || [ ! -x "$GOOSE" ]; then
    GOOSE_NEEDS_BUILD=1
elif [ ! -f "$GOOSE_ARCH_FILE" ] || [ "$(cat "$GOOSE_ARCH_FILE" 2>/dev/null || true)" != "$TARGETARCH" ]; then
    GOOSE_NEEDS_BUILD=1
fi

if [ "$GOOSE_NEEDS_BUILD" = "1" ]; then
    echo "[build-local] step2 = cross-build goose (linux/${TARGETARCH}, tmp module)"
    TMP="$(mktemp -d)"
    trap 'rm -rf "$TMP"' EXIT
    pushd "$TMP" >/dev/null
    go mod init goose-wrapper >/dev/null 2>&1
    go get github.com/pressly/goose/v3/cmd/goose@v3.20.0 >/dev/null 2>&1
    GOOS=linux GOARCH="${TARGETARCH}" CGO_ENABLED=0 \
        go build -buildvcs=false -ldflags "-s -w" -o "$GOOSE" github.com/pressly/goose/v3/cmd/goose
    popd >/dev/null
    printf '%s\n' "${TARGETARCH}" > "$GOOSE_ARCH_FILE"
else
    echo "[build-local] step2 = skip goose (linux/${TARGETARCH} exists). use --force to rebuild"
fi

# ---- step3: 前端 ----
echo "[build-local] step3 = npm run build (web)"
pushd web >/dev/null
if [ ! -d node_modules ] || [ package-lock.json -nt node_modules ] || [ package.json -nt node_modules ]; then
    if [ -f package-lock.json ]; then
        echo "[build-local] step3a = npm ci (deps changed or node_modules missing)"
        npm ci --no-audit --no-fund --loglevel=error
    else
        echo "[build-local] step3a = npm install (node_modules missing)"
        npm install --no-audit --no-fund --loglevel=error
    fi
fi
npm run build
popd >/dev/null

echo "[build-local] done. artifacts:"
ls -lh deploy/bin/gpt2api deploy/bin/goose web/dist/index.html

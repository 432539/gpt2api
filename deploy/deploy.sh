#!/usr/bin/env bash

set -Eeuo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

COMPOSE_FILE="deploy/docker-compose.yml"
ENV_FILE="deploy/.env"
ENV_EXAMPLE_FILE="deploy/.env.example"
CONFIG_FILE="configs/config.yaml"
CONFIG_EXAMPLE_FILE="configs/config.example.yaml"
SERVER_CONTAINER="gpt2api-server"
MYSQL_VOLUME="deploy_mysql_data"

DO_PULL=1
FORCE_GOOSE=0
SKIP_HEALTHCHECK=0
TARGETARCH="${TARGETARCH:-}"
BASE_URL="${APP_BASE_URL:-}"
STEP_NO=0
CURRENT_STEP=""
CURRENT_STEP_STARTED=0
SCRIPT_STARTED=0
CREATED_ENV=0
CREATED_CONFIG=0

usage() {
  cat <<'EOF'
用法:
  bash deploy/deploy.sh
  bash deploy/deploy.sh --no-pull
  bash deploy/deploy.sh --arch arm64
  bash deploy/deploy.sh --force-goose
  bash deploy/deploy.sh --base-url https://api.example.com

行为:
  1. 首次部署时自动创建 deploy/.env 和 configs/config.yaml
  2. 若当前分支已配置 upstream,执行 git pull --ff-only(可用 --no-pull 跳过)
  3. 运行 deploy/build-local.sh 生成后端 / goose / 前端产物
  4. docker compose build server
  5. docker compose up -d
  6. 等待 gpt2api-server 进入 running/healthy,失败时自动打印最近日志

说明:
  - 不会执行 docker compose down / down -v,不会删除 MySQL/Redis/备份/图片卷数据
  - 首次生成的 deploy/.env 会自动写入随机密码和密钥
  - 可通过 --base-url 或环境变量 APP_BASE_URL 设置 configs/config.yaml 的 app.base_url
EOF
}

timestamp() {
  date '+%F %T'
}

elapsed_s() {
  local started="$1"
  echo $((SECONDS - started))
}

quote_cmd() {
  local out=""
  local arg
  for arg in "$@"; do
    if [[ -n "$out" ]]; then
      out+=" "
    fi
    out+="$(printf '%q' "$arg")"
  done
  printf '%s' "$out"
}

log() {
  printf '[%s][deploy] %s\n' "$(timestamp)" "$*"
}

warn() {
  printf '[%s][deploy][WARN] %s\n' "$(timestamp)" "$*" >&2
}

die() {
  printf '[%s][deploy][ERR] %s\n' "$(timestamp)" "$*" >&2
  exit 1
}

step() {
  STEP_NO=$((STEP_NO + 1))
  CURRENT_STEP="$*"
  CURRENT_STEP_STARTED=$SECONDS
  printf '\n[%s][deploy][step %02d] %s\n' "$(timestamp)" "$STEP_NO" "$*"
}

step_done() {
  if [[ -z "$CURRENT_STEP" ]]; then
    return
  fi
  log "step ${STEP_NO} done: ${CURRENT_STEP} ($(elapsed_s "$CURRENT_STEP_STARTED")s)"
}

run_cmd() {
  local desc="$1"
  shift
  local started="$SECONDS"
  log "run: ${desc}"
  log "cmd: $(quote_cmd "$@")"
  "$@"
  log "done: ${desc} ($(elapsed_s "$started")s)"
}

on_err() {
  local exit_code="$1"
  local line_no="$2"
  local cmd="$3"
  printf '\n[%s][deploy][ERR] command failed (exit=%s, line=%s): %s\n' "$(timestamp)" "$exit_code" "$line_no" "$cmd" >&2
  if [[ -n "$CURRENT_STEP" ]]; then
    printf '[%s][deploy][ERR] current step: %s (%ss)\n' "$(timestamp)" "$CURRENT_STEP" "$(elapsed_s "$CURRENT_STEP_STARTED")" >&2
  fi
  printf '[%s][deploy][ERR] compose status:\n' "$(timestamp)" >&2
  docker compose -f "$COMPOSE_FILE" ps >&2 || true
  if docker inspect "$SERVER_CONTAINER" >/dev/null 2>&1; then
    printf '[%s][deploy][ERR] recent server logs:\n' "$(timestamp)" >&2
    docker compose -f "$COMPOSE_FILE" logs --tail=120 server >&2 || true
  fi
  exit "$exit_code"
}
trap 'on_err "$?" "$LINENO" "$BASH_COMMAND"' ERR

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "missing command: $1"
  fi
}

set_env_value() {
  local file="$1"
  local key="$2"
  local value="$3"
  local tmp
  tmp="$(mktemp)"
  awk -v key="$key" -v value="$value" '
    BEGIN { done = 0 }
    $0 ~ ("^" key "=") {
      print key "=" value
      done = 1
      next
    }
    { print }
    END {
      if (!done) {
        print key "=" value
      }
    }
  ' "$file" > "$tmp"
  mv "$tmp" "$file"
}

set_app_yaml_value() {
  local file="$1"
  local key="$2"
  local value="$3"
  local tmp
  tmp="$(mktemp)"
  awk -v want_key="$key" -v want_value="$value" '
    BEGIN { in_app = 0 }
    /^[^[:space:]]/ {
      in_app = ($0 ~ /^app:/)
    }
    in_app && $0 ~ ("^[[:space:]]+" want_key ":") {
      indent = substr($0, 1, match($0, /[^[:space:]]/) - 1)
      print indent want_key ": \"" want_value "\""
      next
    }
    { print }
  ' "$file" > "$tmp"
  mv "$tmp" "$file"
}

random_hex_32() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  od -An -N 32 -tx1 /dev/urandom | tr -d ' \n'
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 48 | tr -d '\n/+=' | cut -c1-48
    return
  fi
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48
}

random_password() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 24 | tr -d '\n/+=' | cut -c1-24
    return
  fi
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 24
}

bootstrap_env_if_missing() {
  if [[ -f "$ENV_FILE" ]]; then
    log "reuse existing $ENV_FILE"
    return
  fi
  [[ -f "$ENV_EXAMPLE_FILE" ]] || die "missing env template: $ENV_EXAMPLE_FILE"

  if docker volume inspect "$MYSQL_VOLUME" >/dev/null 2>&1; then
    die "$ENV_FILE is missing, but docker volume $MYSQL_VOLUME already exists. restore the old .env first to avoid MySQL credential mismatch"
  fi

  cp "$ENV_EXAMPLE_FILE" "$ENV_FILE"
  set_env_value "$ENV_FILE" "MYSQL_ROOT_PASSWORD" "$(random_password)"
  set_env_value "$ENV_FILE" "MYSQL_PASSWORD" "$(random_password)"
  set_env_value "$ENV_FILE" "JWT_SECRET" "$(random_secret)"
  set_env_value "$ENV_FILE" "CRYPTO_AES_KEY" "$(random_hex_32)"
  CREATED_ENV=1
  log "created $ENV_FILE with generated secrets"
}

bootstrap_config_if_missing() {
  if [[ -f "$CONFIG_FILE" ]]; then
    log "reuse existing $CONFIG_FILE"
  else
    [[ -f "$CONFIG_EXAMPLE_FILE" ]] || die "missing config template: $CONFIG_EXAMPLE_FILE"
    cp "$CONFIG_EXAMPLE_FILE" "$CONFIG_FILE"
    set_app_yaml_value "$CONFIG_FILE" "env" "prod"
    CREATED_CONFIG=1
    log "created $CONFIG_FILE from example"
  fi

  if [[ -n "$BASE_URL" ]]; then
    set_app_yaml_value "$CONFIG_FILE" "base_url" "$BASE_URL"
    log "set app.base_url=$BASE_URL"
  fi
}

ensure_clean_tree_for_pull() {
  local dirty
  dirty="$(git status --porcelain --untracked-files=no)"
  if [[ -n "$dirty" ]]; then
    printf '[deploy][ERR] working tree has tracked changes, refuse to git pull:\n%s\n' "$dirty" >&2
    printf '[deploy][ERR] commit/stash/revert them first, or rerun with --no-pull.\n' >&2
    exit 1
  fi
}

maybe_git_pull() {
  if [[ "$DO_PULL" != "1" ]]; then
    log "skip git pull"
    return
  fi

  ensure_clean_tree_for_pull

  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    die "current directory is not a git repository"
  fi

  if ! git rev-parse --abbrev-ref --symbolic-full-name '@{u}' >/dev/null 2>&1; then
    warn "current branch has no upstream, skip git pull"
    return
  fi

  local branch
  branch="$(git branch --show-current)"
  if [[ -z "$branch" ]]; then
    warn "detached HEAD detected, skip git pull (use --no-pull if this is intentional)"
    return
  fi

  run_cmd "git pull --ff-only (${branch})" git pull --ff-only
}

wait_for_server() {
  local timeout_sec="${HEALTH_TIMEOUT_SEC:-240}"
  local waited=0
  local last_seen=""

  if [[ "$SKIP_HEALTHCHECK" == "1" ]]; then
    log "skip health check"
    return
  fi

  log "waiting for $SERVER_CONTAINER to become healthy (timeout=${timeout_sec}s)"
  while (( waited < timeout_sec )); do
    if ! docker inspect "$SERVER_CONTAINER" >/dev/null 2>&1; then
      sleep 1
      waited=$((waited + 1))
      continue
    fi

    local state health
    state="$(docker inspect -f '{{.State.Status}}' "$SERVER_CONTAINER" 2>/dev/null || true)"
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$SERVER_CONTAINER" 2>/dev/null || true)"
    local current="${state}/${health}"

    if [[ "$current" != "$last_seen" ]]; then
      log "server state -> ${current}"
      last_seen="$current"
    fi

    case "$current" in
      running/healthy|running/none)
        log "$SERVER_CONTAINER is ${current}"
        return
        ;;
      exited/*|dead/*|*/unhealthy)
        printf '[%s][deploy][ERR] server state=%s health=%s\n' "$(timestamp)" "$state" "$health" >&2
        docker compose -f "$COMPOSE_FILE" logs --tail=120 server >&2 || true
        exit 1
        ;;
    esac

    if (( waited == 0 || waited % 15 == 0 )); then
      log "health wait progress: ${waited}s/${timeout_sec}s"
    fi
    sleep 1
    waited=$((waited + 1))
  done

  printf '[%s][deploy][ERR] server did not become healthy within %ss\n' "$(timestamp)" "$timeout_sec" >&2
  docker compose -f "$COMPOSE_FILE" logs --tail=120 server >&2 || true
  exit 1
}

print_summary() {
  local http_port="8080"
  if [[ -f "$ENV_FILE" ]]; then
    local parsed
    parsed="$(awk -F= '$1=="HTTP_PORT"{print $2}' "$ENV_FILE" | tail -n1)"
    if [[ -n "$parsed" ]]; then
      http_port="$parsed"
    fi
  fi

  printf '\n[%s][deploy] finished successfully (%ss)\n' "$(timestamp)" "$(elapsed_s "$SCRIPT_STARTED")"
  printf '[%s][deploy] repo: %s\n' "$(timestamp)" "$ROOT"
  printf '[%s][deploy] env: %s\n' "$(timestamp)" "$ENV_FILE"
  printf '[%s][deploy] config: %s\n' "$(timestamp)" "$CONFIG_FILE"
  printf '[%s][deploy] server: http://127.0.0.1:%s\n' "$(timestamp)" "$http_port"
  if [[ -n "$BASE_URL" ]]; then
    printf '[%s][deploy] app.base_url: %s\n' "$(timestamp)" "$BASE_URL"
  else
    printf '[%s][deploy] note: app.base_url keeps current value in %s; use --base-url if you need an external URL\n' "$(timestamp)" "$CONFIG_FILE"
  fi
  if [[ "$CREATED_ENV" == "1" ]]; then
    printf '[%s][deploy] note: first deploy generated %s, back it up before changing the host\n' "$(timestamp)" "$ENV_FILE"
  fi
  if [[ "$CREATED_CONFIG" == "1" ]]; then
    printf '[%s][deploy] note: first deploy generated %s\n' "$(timestamp)" "$CONFIG_FILE"
  fi
  printf '[%s][deploy] logs: docker compose -f %s logs -f server\n' "$(timestamp)" "$COMPOSE_FILE"
}

while (($# > 0)); do
  case "$1" in
    --no-pull)
      DO_PULL=0
      shift
      ;;
    --force-goose)
      FORCE_GOOSE=1
      shift
      ;;
    --skip-healthcheck)
      SKIP_HEALTHCHECK=1
      shift
      ;;
    --base-url)
      if (($# < 2)); then
        die "--base-url requires a value"
      fi
      BASE_URL="$2"
      shift 2
      ;;
    --base-url=*)
      BASE_URL="${1#*=}"
      shift
      ;;
    --arch)
      if (($# < 2)); then
        die "--arch requires a value"
      fi
      TARGETARCH="$2"
      shift 2
      ;;
    --arch=*)
      TARGETARCH="${1#*=}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

require_cmd git
require_cmd docker
require_cmd bash

if ! docker compose version >/dev/null 2>&1; then
  die "docker compose v2 is required"
fi

SCRIPT_STARTED=$SECONDS
log "start deploy flow"
log "repo=$ROOT"
if [[ -n "$TARGETARCH" ]]; then
  log "requested target arch=$TARGETARCH"
fi
if [[ -n "$BASE_URL" ]]; then
  log "requested base_url=$BASE_URL"
fi

step "bootstrap local config files"
bootstrap_env_if_missing
bootstrap_config_if_missing
step_done

step "synchronize git repository"
maybe_git_pull
step_done

step "build local artifacts"
build_cmd=(bash deploy/build-local.sh)
if [[ -n "$TARGETARCH" ]]; then
  build_cmd+=(--arch "$TARGETARCH")
fi
if [[ "$FORCE_GOOSE" == "1" ]]; then
  build_cmd+=(--force)
fi
run_cmd "build local artifacts" "${build_cmd[@]}"
step_done

step "build docker image"
run_cmd "docker compose build server" docker compose -f "$COMPOSE_FILE" build server
step_done

step "start or update containers"
run_cmd "docker compose up -d" docker compose -f "$COMPOSE_FILE" up -d
step_done

step "wait for server health"
wait_for_server
step_done

step "show compose status"
run_cmd "docker compose ps" docker compose -f "$COMPOSE_FILE" ps
step_done

print_summary

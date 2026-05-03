#!/usr/bin/env bash
# =====================================================
# KleinAI · MySQL 初始化脚本
#
# MySQL 8.0 官方镜像首次启动时会按字典序执行 /docker-entrypoint-initdb.d 下
# 的 *.sh / *.sql。我们的 backend/migrations/*.sql 是 goose 风格（同时包含
# Up 与 Down 段），如果直接交给镜像执行会先 CREATE 后 DROP。
# 此脚本只抽取 “-- +goose Up” 与 “-- +goose Down” 之间的 SQL 喂给 mysql。
#
# 容器内挂载约定：
#   /migrations           -> backend/migrations 只读
#   /docker-entrypoint-initdb.d/01-run-migrations.sh -> 本脚本
# =====================================================

set -euo pipefail

MIGDIR="${MIGDIR:-/migrations}"
MYSQL_PWOPT=()
if [[ -n "${MYSQL_ROOT_PASSWORD:-}" ]]; then
  MYSQL_PWOPT=(-p"${MYSQL_ROOT_PASSWORD}")
fi

if [[ ! -d "$MIGDIR" ]]; then
  echo "[klein-init] no migrations dir at $MIGDIR, skip."
  exit 0
fi

shopt -s nullglob
files=( "$MIGDIR"/*.sql )
shopt -u nullglob

if [[ ${#files[@]} -eq 0 ]]; then
  echo "[klein-init] no .sql files in $MIGDIR, skip."
  exit 0
fi

# 按文件名排序确保顺序
IFS=$'\n' sorted=( $(printf '%s\n' "${files[@]}" | sort) )
unset IFS

for f in "${sorted[@]}"; do
  echo "[klein-init] applying $f ..."
  awk '
    /^[[:space:]]*--[[:space:]]*\+goose[[:space:]]+Up([[:space:]]|$)/ {flag=1; next}
    /^[[:space:]]*--[[:space:]]*\+goose[[:space:]]+Down([[:space:]]|$)/ {flag=0; next}
    /^[[:space:]]*--[[:space:]]*\+goose[[:space:]]+StatementBegin([[:space:]]|$)/ {next}
    /^[[:space:]]*--[[:space:]]*\+goose[[:space:]]+StatementEnd([[:space:]]|$)/ {next}
    flag {print}
  ' "$f" | mysql --default-character-set=utf8mb4 -uroot "${MYSQL_PWOPT[@]}" "${MYSQL_DATABASE}"
done

echo "[klein-init] all migrations applied."

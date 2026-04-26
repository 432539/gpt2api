#!/usr/bin/env bash
# 一键更新并重启 server 服务。
#
# 用法:
#   bash deploy/redeploy-server.sh
#
# 注意:deploy/build-local.sh 会每次重跑前端 npm run build。

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "[redeploy-server] git pull"
git pull

echo "[redeploy-server] build local artifacts"
bash deploy/build-local.sh

echo "[redeploy-server] docker-compose build server"
cd deploy
docker-compose build server

echo "[redeploy-server] docker-compose up -d server"
docker-compose up -d server

echo "[redeploy-server] done"

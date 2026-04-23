# gpt2api 容器化部署

一键启动 = `docker compose up -d`。Server 启动时自动:

1. 等 MySQL 健康
2. 跑 `goose up` 应用所有迁移(包含用户表、账号池、审计、备份元数据等)
3. 启动 HTTP 服务(`:8080`)

## ⚠️ 架构说明:宿主预编译 + 容器运行

本仓库的 `Dockerfile` 是**零外网依赖的"预构建 + 运行时"镜像**(规避国内拉 `proxy.golang.org` / `npm registry` 卡死)。  
镜像里**不会**帮你 `go build` / `npm install`,而是直接 `COPY` 宿主机上已经编译好的三个产物:

| 产物 | 路径 |
|------|------|
| 后端(linux/<target arch>) | `deploy/bin/gpt2api` |
| 迁移工具(linux/<target arch>) | `deploy/bin/goose` |
| 前端 Vite 产物 | `web/dist/` |

所以**第一次部署 / 代码更新后,都要先在宿主机跑一次预编译脚本**,再 `docker compose build server`。

## 快速开始

### 0. 宿主机准备

需要提前装好:**Go 1.22+**、**Node 18+ / 20 LTS**、**Docker 24+**、**docker compose v2**。

> 建议配镜像加速:  
> `go env -w GOPROXY=https://goproxy.cn,direct`  
> `npm config set registry https://registry.npmmirror.com`

### 1. 预编译(必做一次)

一条命令搞定后端 + goose + 前端三个产物。默认目标架构跟随当前宿主机,也可显式指定:

```bash
# Linux / macOS / WSL
bash deploy/build-local.sh
TARGETARCH=arm64 bash deploy/build-local.sh
TARGETARCH=amd64 bash deploy/build-local.sh

# Windows PowerShell
powershell -NoProfile -File deploy\build-local.ps1
powershell -NoProfile -File deploy\build-local.ps1 -Arch arm64
```

结束后检查:

```bash
ls -lh deploy/bin/gpt2api deploy/bin/goose web/dist/index.html
```

### 2. 配置与启动

```bash
cp configs/config.example.yaml configs/config.yaml
cd deploy
cp .env.example .env           # 修改 JWT_SECRET / CRYPTO_AES_KEY / MySQL 密码
docker compose build server    # 把刚才的产物 COPY 进镜像
docker compose up -d
docker compose logs -f server  # 观察迁移 + 启动日志
```

`server` 启动前会检查 `configs/config.yaml`。如果缺失,容器会打印提示后退出,不会循环重启。

> **⚠️ 没有默认账号 / 密码。** 启动完成后打开 `http://<服务器IP>:8080/register` 注册,
> **第一个注册的账号自动成为 admin**;之后的注册都是普通用户。建议首位 admin 登录后去
> **管理后台 → 系统设置**关闭"允许开放注册"。详见仓库根 `README.md`「5. 首次登录」。

### 3. 日常更新

| 场景 | 做什么 |
|------|--------|
| 只改了前端 | `cd web && npm run build` → `cd ../deploy && docker compose build server && docker compose up -d server` |
| 只改了后端 | `bash deploy/build-local.sh` → `cd deploy && docker compose build server && docker compose up -d server` |
| `git pull` 新版 | `bash deploy/build-local.sh` → `docker compose build server && docker compose up -d server` |
| 只改了 `.env` | `docker compose up -d`(环境变量变化 compose 会自动感知并重建容器) |
| 想秒重启 | `docker compose restart server` |

默认暴露端口:


| 服务     | 端口     | 说明                   |
| ------ | ------ | -------------------- |
| server | `8080` | OpenAI 兼容网关 + 后台 API |
| mysql  | `3306` | 业务数据库                |
| redis  | `6379` | 锁 / 限流 / 缓存          |


## 目录与数据卷

- `mysql_data`:MySQL 物理数据
- `redis_data`:Redis AOF
- `backups`:`/app/data/backups` —— 数据库备份文件(.sql.gz)落盘目录
- `./logs`:宿主机 `deploy/logs` —— server 日志

数据库备份和宿主机数据是两条独立路径:

- 管理员在后台"数据备份"里点"立即备份"会把 `mysqldump` 压缩写入 `backups` 卷;
- `backups` 卷也可以挂回宿主机目录来做 rsync 异地冷备。

## 安全红线

以下必须在 **.env** 中显式覆盖(生产禁用默认值):

- `JWT_SECRET`:至少 32 字符随机串
- `CRYPTO_AES_KEY`:**严格** 64 位 hex(32 字节 AES-256 key)
- `MYSQL_ROOT_PASSWORD` / `MYSQL_PASSWORD`

后端对高危操作的保护:


| 操作        | 权限常量            | 额外要求                                                     |
| --------- | --------------- | -------------------------------------------------------- |
| 列出/下载备份   | `system:backup` | -                                                        |
| 创建备份      | `system:backup` | -                                                        |
| 删除备份      | `system:backup` | `X-Admin-Confirm: <password>`                            |
| 上传备份      | `system:backup` | `X-Admin-Confirm: <password>`                            |
| **恢复数据库** | `system:backup` | `backup.allow_restore=true`(默认 false)+ `X-Admin-Confirm` |
| 调整用户积分    | `user:credit`   | 自动落审计                                                    |


凡是 `/api/admin/`* 的写操作(POST/PUT/PATCH/DELETE)都会被 `audit.Middleware` 自动记录到 `admin_audit_logs` 表,管理员可在"审计日志"页查看。

## 恢复数据库的标准流程

因为 `restore` 会直接覆盖现库,**默认关闭**。启用方式:

1. 在 `.env` 中 `BACKUP_ALLOW_RESTORE=true`
2. `docker compose up -d server`(重启生效)
3. 在后台点"恢复",输入管理员密码二次确认
4. 完成后把 `.env` 改回 `false` 再重启,锁回常态

## 常用运维命令

```bash
# 手动触发一次迁移(平时容器启动时会自动跑)
docker compose exec server goose -dir /app/sql/migrations mysql \
  "$GPT2API_MYSQL_DSN" up

# 查看当前迁移状态
docker compose exec server goose -dir /app/sql/migrations mysql \
  "$GPT2API_MYSQL_DSN" status

# 进入 MySQL
docker compose exec mysql mysql -ugpt2api -p gpt2api

# 冷备份(API 之外的兜底方式)
docker compose exec server mysqldump -hmysql -ugpt2api -p \
  --single-transaction --quick gpt2api | gzip > gpt2api-$(date +%F).sql.gz
```

## 单节点 vs 多节点

当前 compose 配置针对单机部署。后续要做多副本:

- `server` 可直接 `docker compose up -d --scale server=3`(需前面加 nginx/traefik)
- `backups` 卷改成共享存储(NFS / S3 fuse),否则每个副本只能看到自己创建的备份
- Redis 分布式锁已天然支持多副本,MySQL 和 JWT 密钥需统一

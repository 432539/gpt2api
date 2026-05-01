# syntax=docker/dockerfile:1.6
#
# "预构建 + 运行时" 镜像 —— 零外网依赖版本(国内 / 内网环境友好)
# --------------------------------------------------------------
# 构建前请先在宿主机生成以下产物(见 deploy/build-local.ps1 / build-local.sh):
#   deploy/bin/gpt2api           Linux/amd64 可执行
#   deploy/bin/goose             Linux/amd64 可执行
#   web/dist/                    前端 Vite 产物
#
# 这样容器内除了 alpine 的 apk 装几个小工具,不需要任何外网访问。
# 在网络通畅的 CI 环境里,你也可以用旧版的两阶段 Dockerfile。
# --------------------------------------------------------------

FROM alpine:3.20

# 走 ustc HTTP 源,规避企业 HTTPS 代理 MITM 导致的证书校验失败
RUN sed -i 's@https\?://dl-cdn.alpinelinux.org@http://mirrors.ustc.edu.cn@g' /etc/apk/repositories \
    && apk add --no-cache ca-certificates tzdata curl bash mariadb-client \
    && update-ca-certificates \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

# 后端 + goose 二进制(宿主交叉编译产物)
COPY deploy/bin/gpt2api /app/gpt2api
COPY deploy/bin/goose   /usr/local/bin/goose
RUN chmod +x /app/gpt2api /usr/local/bin/goose

# 前端构建产物 —— 被 Go server 以 SPA 方式托管
COPY web/dist /app/web/dist

# Migrations、默认配置、entrypoint
COPY sql     /app/sql
COPY configs /app/configs
COPY deploy/entrypoint.sh /app/entrypoint.sh
RUN sed -i 's/\r$//' /app/entrypoint.sh \
    && chmod +x /app/entrypoint.sh \
    && mkdir -p /app/data/backups /app/logs

EXPOSE 18080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD curl -fsS http://localhost:18080/healthz || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/gpt2api", "-c", "/app/configs/config.yaml"]

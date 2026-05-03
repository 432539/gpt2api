# syntax=docker/dockerfile:1.7
# 单阶段 install + build：避免分离层时 COPY 覆盖 pnpm 创建的符号链接 node_modules。
FROM node:20-alpine AS build
WORKDIR /repo

RUN corepack enable

COPY pnpm-workspace.yaml pnpm-lock.yaml* package.json ./
COPY tsconfig.base.json ./
COPY packages packages
COPY apps/admin apps/admin

RUN pnpm install --frozen-lockfile=false
RUN pnpm --filter @kleinai/admin build

FROM nginx:1.27-alpine AS runtime
COPY --from=build /repo/apps/admin/dist /usr/share/nginx/html
COPY apps/admin/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

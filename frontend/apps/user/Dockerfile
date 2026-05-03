# syntax=docker/dockerfile:1.7
# 单阶段 install + build：避免分离层时 COPY 覆盖 pnpm 创建的符号链接 node_modules。
FROM node:20-alpine AS build
WORKDIR /repo

RUN corepack enable

# 先 COPY 全部 workspace 元信息 + 共享包 + 本 app 源码，再 install + build
COPY pnpm-workspace.yaml pnpm-lock.yaml* package.json ./
COPY tsconfig.base.json ./
COPY packages packages
COPY apps/user apps/user

RUN pnpm install --frozen-lockfile=false
RUN pnpm --filter @kleinai/user build

FROM nginx:1.27-alpine AS runtime
COPY --from=build /repo/apps/user/dist /usr/share/nginx/html
COPY apps/user/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]

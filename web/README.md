# GPT2API 控制台(前端)

基于 Vue 3 + TypeScript + Vite + Element Plus 的管理台前端,为 `cmd/server` 提供:

- 登录 / 注册 / 个人中心(Dashboard、API Keys)
- 管理后台(用户、用户分组、审计日志、数据备份/恢复)
- 其余模块(账号池、代理池、模型、充值、用量统计、全局 Keys、系统设置)已预留占位页

## 快速开始

```bash
cd web
npm install        # 或 pnpm install / yarn
npm run dev        # http://localhost:5173
```

开发服务器通过 Vite 代理 `/api`、`/v1`、`/healthz` 到后端(默认 `http://localhost:18080`)。
改成别的后端:在 `web/.env.development` 里调整 `VITE_API_BASE`。

## 生产构建

```bash
npm run build       # 产物在 web/dist/
npm run preview     # 本地静态预览
```

生产常见部署方式:把 `web/dist/` 交给 nginx,前端走同源,
nginx 把 `/api/` 和 `/v1/` 反代到 Go 后端即可。`VITE_API_BASE` 留空表示同源。

## 目录约定

```
src/
  api/         后端 HTTP 客户端封装(auth/apikey/admin/backup…)
  router/      路由表 + 权限守卫(meta.perm)
  stores/      Pinia:user store(持久化 token + 角色 + 权限 + 菜单)
  layouts/     BasicLayout(侧边栏/顶栏) + BlankLayout(登录等)
  views/
    auth/      登录 / 注册
    personal/  个人中心页
    admin/     管理员页
  utils/       金额/字节/时间格式化
  styles/      全局 scss
  components/  公共组件(Placeholder 等)
```

## 权限模型

- 登录后 `GET /api/me` 取 `user/role/permissions`,`GET /api/me/menu` 取菜单树。
- 路由级:`route.meta.perm = 'foo:bar' | ['a','b']`,守卫里调用 `store.hasPerm()` 做前置校验。
- UI 级:按钮的 `:disabled` / `v-if` 也会参考 `store.hasPerm()`。
- 真正的权限校验在后端 `middleware.RequirePerm`,前端仅做体验优化,勿当安全边界。

## 敏感操作(二次确认)

下列操作会要求用户在浏览器里再输入一次管理员密码(通过 `X-Admin-Confirm` 头发送):

- 重置他人密码 `POST /api/admin/users/:id/reset-password`
- 删除用户 `DELETE /api/admin/users/:id`
- 调账 `POST /api/admin/users/:id/credits/adjust`
- 删除 / 恢复 / 上传备份 `/api/admin/system/backup/*`

## 路线图

- [ ] 个人:Usage / Billing / Images
- [ ] Admin:账号池、代理池、模型、充值订单、用量统计、全局 Keys、系统设置
- [ ] i18n(当前只提供简体中文)
- [ ] 黑暗模式切换(Element Plus dark 已引入 css vars)

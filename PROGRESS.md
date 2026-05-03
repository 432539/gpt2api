# gpt2api 开发进度看板

> 最近更新：2026-04-27（UX 调优后）
> 主文档：`[README.md](./README.md)` · 规范：`[docs/](./docs/)` · 常驻 AI 规则：`.cursor/rules/`
>
> **项目定位**：基于 GPT / GROK 双账号池的高并发 AIGC 平台，OpenAI 协议兼容；前端 React + Tailwind，后端 Go + MySQL + Redis。
> **皮肤规范**：默认主题 = 「克莱因蓝（Klein Blue · IKB `#002FA7`）」，仅作为视觉皮肤通过 `packages/theme` token 集中管理，可在 `tokens.css` 切换为其它色板而不影响业务代码。代码内部 module/包名 `@kleinai/`* 保留作为既有命名空间，不外露给最终用户。

---

## ⚡ 本地起服务（Sprint 9 已具备完整可观察形态）

提供两种启动模式，按需选择：

### 模式 A · 全容器（推荐：零本地依赖、最贴近线上）

> MySQL / Redis / 4 个 Go 后端 / 2 个前端静态站全部跑容器，前端容器内 nginx 统一反代到后端。

```powershell
# 首次构建镜像（≈ 5–8 分钟，分别打 backend / user-web / admin-web 三个镜像）
cd deploy
$env:KLEIN_DEV_MYSQL_PORT='23306'   # 仅当 13306 被 Hyper-V 占用时设置
docker compose -f docker-compose.dev-full.yml up -d --build

# 之后日常启动 / 关停
docker compose -f docker-compose.dev-full.yml up -d
docker compose -f docker-compose.dev-full.yml down            # 仅停容器
docker compose -f docker-compose.dev-full.yml down -v         # 同时清数据卷
```


| 入口            | 地址                                                               | 默认账号                 |
| ------------- | ---------------------------------------------------------------- | -------------------- |
| 用户端           | [http://localhost:17080](http://localhost:17080)                 | 注册即用                 |
| 管理后台          | [http://localhost:17088](http://localhost:17088)                 | `admin` / `admin123` |
| OpenAI 兼容（直连） | [http://localhost:17200/v1](http://localhost:17200/v1)           | 用户 KEY 管理生成 sk-...   |
| OpenAI 兼容（反代） | [http://localhost:17080/v1](http://localhost:17080/v1)           | 同上（推荐前端调用）           |
| 用户 API（直连）    | [http://localhost:17180/healthz](http://localhost:17180/healthz) | -                    |
| 后台 API（直连）    | [http://localhost:17188/healthz](http://localhost:17188/healthz) | -                    |


### 模式 B · 半容器（适合改 Go / 改 React 边写边热更新）

> 仅 MySQL / Redis 跑容器，4 个 Go 后端 + 2 个 Vite 跑主机，省去镜像构建时间。

```powershell
pwsh ./scripts/dev-up.ps1
# 该脚本会：
#   1. docker compose -f deploy/docker-compose.dev.yml up -d
#   2. 等 MySQL 健康
#   3. 复制 backend/.env.example → backend/.env.local（首次）
#   4. 在 4 个新窗口拉起 api / admin / openai / worker

# 起前端（首次需 pnpm install）
cd frontend
pnpm install
pnpm --filter @kleinai/user  dev   # → http://localhost:5173
pnpm --filter @kleinai/admin dev   # → http://localhost:5174

# 关停
pwsh ./scripts/dev-down.ps1
```

> 两种模式默认都用 `KLEIN_PROVIDER_GPT/GROK=mock`，无需真实 OpenAI / GROK 凭证即可走通生成全流程。
> 切真实通道：模式 A 改 `deploy/docker-compose.dev-full.yml` 里的 `KLEIN_PROVIDER_*=real`；模式 B 改 `backend/.env.local`。
> 真实凭证一律走 **管理后台 → Token 管理** 入库（AES-256-GCM 落盘）。

**Windows 上 13306 / 16379 端口被 Hyper-V 占用怎么办？**

```powershell
# 查看 TCP 排除范围
netsh interface ipv4 show excludedportrange protocol=tcp
# 临时释放（需管理员，重启后失效）
net stop winnat
net start winnat
# 或：模式 A 直接 $env:KLEIN_DEV_MYSQL_PORT='23306' 后再 up
# 或：模式 B 改 deploy/docker-compose.dev.yml + backend/.env.local 的 DSN
```

---

## 总览


| 模块             | 状态     | 负责  | 备注                                                   |
| -------------- | ------ | --- | ---------------------------------------------------- |
| **规范文档**       | ✅ Done | -   | 6 篇规范 + README                                       |
| **AI 常驻规则**    | ✅ Done | -   | `.cursor/rules/` 4 份规则                               |
| **后端脚手架**      | ✅ Done | -   | 4 个 cmd 二进制 + healthz / readyz                       |
| **前端脚手架**      | ✅ Done | -   | pnpm monorepo + 用户端 / 后台 骨架                          |
| **部署脚手架**      | ✅ Done | -   | docker-compose + 3 份 nginx + Dockerfile              |
| **账号体系**       | ✅ Done | -   | 注册 / 登录 / refresh / me / 改密                          |
| **账号池核心**      | ✅ Done | -   | 增删改查 + 批量导入 + RR 调度 + AES 加密                         |
| **API Key 管理** | ✅ Done | -   | 用户 CRUD + OpenAI 兼容鉴权                                |
| **计费引擎**       | ✅ Done | -   | 钱包 / 预扣 / 结算 / 退款 + CDK 兑换                           |
| **生成调度**       | ✅ Done | -   | 真实 GPT / GROK provider + AES 解密凭证 + env 切换 mock/real |
| **前后端联调**      | ✅ Done | -   | user 全部页面接入真实 API；admin 主流程接入                        |
| **管理后台联调**     | ✅ Done | -   | 登录 / 仪表盘 / 账号池 CRUD + 批量导入 / CDK 批次                  |


图例：✅ 完成 · 🚧 进行中 · ⏳ 待开始 · ⛔ 阻塞 · 🐛 待修复

---

## Sprint 0 · 规范与基础（已完成）

- 编写 6 份开发规范（`docs/01 ~ 06`）
- 项目根 README
- Cursor AI 常驻规则 `.cursor/rules/`
  - `00-core.mdc`（始终生效）
  - `10-backend.mdc`（backend/**）
  - `20-frontend.mdc`（frontend/**）
  - `30-deploy.mdc`（deploy/**、Dockerfile）
- PROGRESS 看板

---

## Sprint 1 · 后端脚手架（已完成）

> 目标：4 个 cmd 二进制能起服务、返回 healthz；MySQL / Redis 连接通；核心表迁移完成。

- 仓库结构 `backend/`：cmd / internal / pkg / configs / migrations / scripts
- `go.mod` + 依赖锁定
- `Makefile`：build / run-api / run-admin / run-openai / run-worker / migrate-up / migrate-down / lint / test
- `.env.example` + `configs/config.yaml`
- 基础包 `pkg/`：config / logger / database / snowflake / jwtx / crypto / errcode / response / ratelimit / httpc / version
- 中间件 `internal/middleware/`：recovery / requestid / access_log / cors / auth / ratelimit / security
- 入口 `cmd/`：api / admin / openai / worker
- 数据库迁移 `migrations/` 10 个文件覆盖核心域

---

## Sprint 2 · 前端脚手架（已完成）

- `frontend/` pnpm workspace + tsconfig.base + eslint / prettier
- `packages/theme`：tokens.css + tailwind.preset.ts + animations
- `apps/user`：Vite + React Router + AuthLayout / AppLayout + 登录 / 注册 / 创作 / 历史 / 计费 / KEY / 邀请 / 设置
- `apps/admin`：AdminLayout + 后台登录 + 仪表盘 + Token 管理骨架

---

## Sprint 3 · 部署脚手架（已完成）

- `deploy/docker-compose.yml`（基础）
- `deploy/env/.env.example`
- `deploy/nginx/`：`user.conf` / `admin.conf` / `openai.conf`
- `backend/Dockerfile`（多阶段、distroless）
- `frontend/apps/user/Dockerfile` + `frontend/apps/admin/Dockerfile`

---

## Sprint 4 · 账号体系 + 账号池 MVP（已完成）

- 用户：注册 / 登录 / 刷新 / me / 改密
- 账号池：
  - 单条 CRUD + 启用 / 停用 / 解除熔断
  - 批量导入（每行一条；支持 `name@@cred` / `cred@base_url` / `cred`）
  - 调度器 RoundRobin / WeightedRR + 30s 缓存
  - 凭证 AES-256-GCM 加密存储
  - 分组管理 / 健康检查 worker（待补）
- 管理后台：账号池列表 / 详情 / 批量导入 / 池状态接口

---

## Sprint 5 · API Key + OpenAI 兼容鉴权（已完成）

- `api_key` 模型 + repo（hash + salt + last4）
- 用户端 CRUD：list / create（明文仅返回一次）/ toggle / delete
- `AuthAPIKey` 中间件：`Authorization: Bearer sk-klein-xxx`
- OpenAI 兼容服务挂入鉴权 + scope 校验

---

## Sprint 6 · 计费引擎（已完成）

- `wallet_log` + `consume_record` + `refund_record`
- BillingService：PreDeduct / Settle / FailRefund / GrantPoints
- CDK 服务：批次生成 + 用户兑换（事务 + 余量 + per_user_limit）
- 用户端：钱包流水 / CDK 兑换 接口
- 管理后台：CDK 批次创建接口
- 充值订单（支付宝 / 微信 / Stripe）—— 后续接入
- 优惠码 / 邀请返点 —— 后续接入

---

## Sprint 7 · 生成调度（已完成）

- `generation_task` / `generation_result` 模型 + repo
- `provider.Provider` 接口 + `mock` 实现
- `GenerationService`：幂等 + 预扣 + 池调度 + 失败退款
- 用户端 `/api/v1/gen/{image,video}` + 任务详情 + 历史
- OpenAI 兼容 `/v1/{images,videos}/generations`（同步等待）
- 真实 GPT / GROK 适配（Sprint 9 已完成，见下方）
- WebSocket / SSE 进度推送 —— Sprint 10

---

## Sprint 8 · 前后端联调（已完成 stub）

> 目标：用户端真实跑通 注册 → 登录 → 兑换 CDK → 创建生成 → 查看历史 → 管理 KEY 全链路。

- `apps/user/src/lib/api.ts` axios 客户端（baseURL / token / 401 / 错误码）
- `apps/user/src/lib/services.ts` 领域 API 封装
- `apps/user/src/lib/types.ts` + `format.ts` 类型与展示工具
- `stores/auth.ts` zustand + `stores/toast.ts` + `components/Toaster`
- `routes/RequireAuth` 路由守卫 + 401 自动跳登录
- 登录 / 注册页对接 `/auth/`*（zod 表单校验 + 自动跳转）
- AppLayout 顶栏对接 `/users/me`（实时余额、退出登录）
- 创作中心 · 图像 对接 `/gen/image` + 轮询任务 + 余额刷新
- 创作中心 · 视频 对接 `/gen/video` + 轮询任务 + 余额刷新
- 生成历史对接 `/gen/history`（图/视频筛选、分页加载）
- KEY 管理对接 `/keys/`*（创建一次性明文展示、停启用、删除）
- 余额明细对接 `/billing/logs` + `/billing/cdk/redeem`
- 设置页对接 `/users/password` + 资料展示 + 主题切换
- 邀请页展示真实邀请码 + 一键复制
- 调用说明页基于 `VITE_OPENAI_BASE_URL` 生成示例 + 一键复制
- `vite.config.ts` 增加 `/api` → 17180、`/v1` → 17200 代理

---

## Sprint 9.6 · UI/UX Pro 规范化（已完成）

> 目标：把已联调的页面从「能用」升级到「整套规范」。引入「设计 token + 共享组件层」架构，前后台高频页面全量替换为统一字号、间距、按钮、卡片、表格、空状态、徽标、对话框组件；本地 `index.css` 仅留站点级覆盖。

- **Design Tokens（`packages/theme/src/tokens.css`）**：8pt 间距栅格 + `clamp()` 流式字号（display/h1-h4/body/small/tiny）+ `tracking-`* / `weight-*` 排印变量 + 控件高度变量 `ctl-h-{xs,sm,md,lg,xl}` + 控件 padding 变量；阴影增加 `shadow-4 / shadow-inset / focus-ring / focus-ring-danger`；动画 `--ease-* / --duration-*` 标准化；语义颜色补 `success-soft / warning-soft / danger-soft / info-soft`。
- **共享组件层（`packages/theme/src/components.css`，~720 行）**：
  - 排印：`text-display / text-h1..h4 / text-body / text-small / text-tiny / text-overline / text-mono / gradient-text`
  - 按钮：`btn` + `btn-primary|secondary|outline|ghost|danger|danger-ghost|link` × `btn-xs|sm|md|lg|xl` + `btn-icon|btn-block`，旧 `.btn-klein` 作为 alias 保留
  - 表单：`field / field-label / field-hint / field-error / input / select / textarea / input-affix / checkbox / radio`，`.input-klein` alias
  - 卡片：`card / card-flat / card-elevated / card-glass / card-gradient / card-tinted / card-section / dialog-surface`，`.glass-card` alias
  - 数据展示：`stat-grid / stat-tile(-accent) / stat-label / stat-value / stat-delta`、`data-table` 全表样式、`list-row`
  - 状态/标记：`badge(-success|warning|danger|klein|solid|outline) / chip(-active|outline) / kbd / progress / tabs / tab`
  - 空态/骨架：`empty-state(-icon|title|desc) / skeleton(-text)`
  - 页面骨架：`page / page-narrow / page-wide / page-header / page-title / page-subtitle / section / section-header / section-title`
- **Tailwind preset**：暴露所有新 token 为原子类（`bg-info-soft`、`text-tiny`、`font-extra`、`tracking-wide`、`shadow-4`、`duration-base`、`ease-out` 等）。
- **入口收敛**：在 `apps/{user,admin}/src/index.css` 顶部 `@import '@kleinai/theme/components.css'`，确保 `@layer components` 与 `@apply` 在同一 PostCSS 上下文；本地 `index.css` 只保留 `body line-height` 与 `.creative-pane / .admin-pane` 等页面级覆盖。
- **页面重构**（用户端）：`LoginPage / RegisterPage / CreateImagePage / CreateVideoPage / HistoryPage / BillingPage / KeysPage / SettingsPage / DocsPage / InvitePage / AppLayout / LoginGate` 全量切换 `btn / btn-{variant} / field / input / card / page-* / badge / progress / tabs / kbd / empty-state`。
- **页面重构**（管理端）：`Dashboard / TokenAccounts / CDK / Login / AdminLayout / _placeholder` 同步升级；`TokenAccountsPage` 表格切到 `data-table`，状态徽标切到 `badge badge-{success|warning|danger}`；`_placeholder` 改为带图标的 `empty-state`，所有占位页（用户管理 / 充值消费 / 优惠码 / 系统配置 / 请求日志）拥有一致空态外观。
- **构建验证**：`pnpm typecheck` & `pnpm build` 全绿；docker `user-web` / `admin-web` 镜像在 `docker-compose.dev-full.yml` 上重新构建并热替换，仅 `28KB` (admin) / `38KB` (user) gzipped CSS。

---

## Sprint 9.5 · UX & 品牌微调（已完成）

> 目标：把项目从「Klein Blue 主题站」收敛为「gpt2api · AIGC 平台」，主题降级为默认皮肤；首页开放浏览，生成动作未登录时弹浮层。

- **品牌降级**：用户可见文案统一改为 `gpt2api`，`Logo` 标识改为 `gpt2api`（`g2a` 角标）。代码 module path（`@kleinai/`*、Go module）保留不变，避免 churn。
- **首页开放**：`/`、`/create/image`、`/create/video`、`/docs` 不再要求登录，未登录用户可直接体验生成 UI；受保护路由（历史 / 余额 / KEY / 邀请 / 设置）外层挂 `<RequireAuth>`，未登录访问会回到首页并自动弹登录浮层。
- **登录浮层**：新增 `<LoginGate />` 全局浮层 + `useLoginGateStore` 状态机 + `useEnsureLoggedIn(action)` Hook。生成按钮、受保护导航、401 拦截均通过浮层完成「断点续做」，无需中断当前页面状态。
- **响应式**：
  - 用户端侧栏宽度改 `clamp(220px,18vw,260px)`，避免 1024px 上吃宽度过多。
  - 创作页三栏改为 `lg`（≥1024px）双栏 + `2xl`（≥1536px）三栏；中等屏幕将「当前任务进度」合并到结果区头部。
  - 后台 `<AdminLayout>` 补齐移动端抽屉 + 顶栏汉堡按钮，header 加 `truncate` / `flex-shrink-0`，避免昵称撑爆。
- **空白页修复**：`apps/admin` 的 `BrowserRouter basename="/admin"` 与 nginx 的根挂载路径冲突 → 改成无 basename；401 跳转改为 `/login`。
- **PROGRESS / README 改写**：明确「克莱因蓝是默认皮肤而非项目身份」；默认账号 `admin / admin123`、用户端注册即用。

---

## Sprint 9 · 真实 Provider + 后台联调（已完成）

> 目标：替换 mock provider，跑通真实 GPT / GROK 调用；管理后台前端联调账号池 / CDK。

### Provider 真实化

- `provider.Request` 加入 `Credential` / `BaseURL`，避免 provider 持有 AES
- `GenerationService` 注入 AES，调用前解密 `account.credential_enc`
- `provider/gpt`：OpenAI 兼容 `/v1/images/generations`，同步返回
  - URL / b64 双兼容；4xx/5xx 失败时自动熔断账号
- `provider/grok`：通用「异步任务 + 轮询」协议
  - 同步直返 / 异步 `task_id` 自动适配
  - 内置 12 min 超时 + 3s→10s 指数 backoff
- `provider/factory`：env 驱动 `KLEIN_PROVIDER_GPT/GROK = mock|real`，零代码切换
- `.env.example` 增加 provider 模式与 base url 配置
- `go build ./... && go vet ./...` 全绿

### 管理后台前端

- `lib/api`（独立 token KEY = `klein:admin:token`）+ `lib/types` + `lib/format` + `lib/services`
- `stores/auth` + `stores/toast` + `components/Toaster`
- `routes/RequireAuth` 路由守卫；401 自动清 token + 跳 `/admin/login`
- 登录页：zod + react-hook-form + `/admin/api/v1/auth/login`
- AdminLayout 顶栏：当前管理员信息 + 角色 + 退出登录
- **仪表盘**：实时拉 `accounts/stats` + 列表 total，渲染 GPT / GROK 池水位
- **Token 管理**：列表（筛选 + 关键字 + 分页）+ 状态切换 + 删除
- **Token 管理**：新增账号 Dialog（明文凭证，提交后端加密）
- **Token 管理**：批量导入 Dialog（粘贴文本，每行一条，三种格式自动识别）
- **CDK 批次**：批次号 / 名称 / 单码点数 / 数量 / per_user_limit / 过期 — 提交并展示结果
- `pnpm --filter @kleinai/admin typecheck` + `build` 全绿

### 仍开口

- 真实 webhook 回调 + 写 `generation_result`（异步 worker，Sprint 10）
- 管理后台：CDK 列表 + 导出 CSV（下一轮）
- 管理后台：用户管理（封禁 / 加点 / 修改套餐，下一轮，需补后端 API）
- 管理后台：充值消费 / 优惠码 / 请求日志（下一轮，目前为占位页）

---

## Sprint 9.5 · 账号池高级能力（已完成）

> 目标：把已写好的 `AccountTestService / OpenAIOAuthService / SystemConfigService / ProxyService` 接入 router，并在管理后台做完代理管理 + 系统配置两块拼图。

### 后端

- `router.MountAdmin` 装配补齐：
  - `POST /admin/api/v1/accounts/:id/test`、`POST /accounts/:id/refresh`、`POST /accounts/batch-refresh`
  - 整组 `proxies`：`GET / POST / PUT / DELETE / POST /:id/test`
  - 整组 `system`：`GET /system/settings`、`PUT /system/settings`
  - `accountAdmin.SetTestService(testSvc)` 回填，使 Test/Refresh 走得通
- 复用既有服务（无新增逻辑）：
  - `AccountTestService.Test`：GPT/GROK `GET /v1/models` 探活
  - `AccountTestService.RefreshOAuth`：`auth.openai.com/oauth/token` refresh_token grant，写回 `access_token_enc / access_token_expires_at / last_refresh_at`
  - `AccountTestService.maybeRefresh`：access_token 距过期阈值内自动刷新
  - `AccountTestService.TestProxy`：通过代理探测 `https://www.google.com/generate_204` 测延迟
  - `SystemConfigService`：30s 内存缓存 + 类型化便捷方法（`GlobalProxyEnabled / RefreshBeforeHours / OpenAIClientID / OpenAITokenURL`）

### 后台前端

- `lib/types`：补 `AccountItem` 上 OAuth/Test 字段；新增 `AccountTestResp / AccountRefreshResp / AccountBatchRefreshResp / ProxyItem / ProxyCreateBody / ProxyUpdateBody / ProxyTestResp / SystemSettings`
- `lib/services`：补 `accountsApi.test / refresh / batchRefresh`；新增 `proxiesApi`、`systemApi`
- **Token 管理页**升级：
  - 顶栏新增「批量刷新 OAuth」按钮（按当前 provider 过滤）
  - 表格新列「OAuth / 最近测试」：RT / AT 徽标 + access_token 倒计时 + last_test 状态/延迟/相对时间
  - 操作列新增「测试连通」按钮（所有账号）+「刷新 access_token」按钮（仅 OAuth 账号）
- **代理管理**新页（`/proxies`）：列表（启用/禁用 tabs + 关键字 + 分页）+ 新增/编辑 Dialog + 启停 + 删除 + 测试连通
- **系统配置**新页（`/config`）替代占位页：
  - 「全局代理」分区：开关 + 下拉选择已启用的代理
  - 「OAuth 调度」分区：刷新窗口（小时）/ OpenAI client_id / Token Endpoint
  - 「完整配置（只读）」JSON 视图便于诊断
- 侧边栏新增「代理管理」入口

### 验收

- `go vet ./... && go build ./...` 全绿
- `pnpm --filter @kleinai/admin typecheck` 全绿
- 容器 `klein-admin-dev` 重建后 GIN 启动日志已注册全部新路由
- `curl /admin/api/v1/{accounts,proxies,system/settings}` 返回 401（已挂中间件）

### 9.5 修订（hotfix · 2026-04-27 晚）

> 用户在管理后台试新增 / 批量导入账号时点出 5 个真实问题，本轮一并修齐：

- **数据库 schema 漂移**：`migrations/20260427130011_init_proxy_oauth.sql` 中 `oauth_meta` 列 `AFTER` 与 `COMMENT` 顺序倒置，MySQL 8.0 拒绝执行；旧库（已建在 9.5 之前）的 mysql 容器 `docker-entrypoint-initdb.d` 不会重跑，导致 `account` 表缺 `proxy_id / oauth_meta / access_token_enc / refresh_token_enc / access_token_expires_at / last_refresh_at / last_test_*` 共 10 列 + 缺 5 条 `system_config` 默认值。已修正迁移并对运行中库手动补齐。
- **批量导入 OAuth 行为不一致**：`AccountAdminService.BatchImport` 漏写 `refresh_token_enc`，与单条 `Create` 不同；现一并写入，使后续 `RefreshOAuth` 行为对齐。
- **DTO 缺 `proxy_id`**：`AccountBatchImportReq` 加 `ProxyID *uint64`，让批量导入也能直接绑代理。
- **新增账号 / 批量导入 UI 字段不全**：
  - `CreateDialog` 增加：绑定代理下拉（取 `proxies.list status=1`）、`rpm_limit / tpm_limit / daily_quota / monthly_quota` 折叠区块、按 `auth_type` 切换的 placeholder + hint（API Key / Cookie / OAuth `refresh_token` 各自文案）
  - `ImportDialog` 增加：默认绑定代理下拉、按 `auth_type` 切换的多行示例（API Key/Cookie/OAuth）
  - `base_url` 字段统一经 `normalizeBaseURL()` 自动补 `https://`，规避后端 `binding:"omitempty,url"` 校验对 `api.openai.com` 这类裸域的 400
- **端到端验证**：登录后 `POST /accounts`（OAuth + 限速字段）、`POST /accounts/import`（OAuth × 3 行）、`GET /accounts?keyword=e2e` 全部 200，`has_refresh_token=true` 在所有 OAuth 行上正确回传。

---

## 🟡 剩余未开发清单（截至 2026-04-27 23:00）

> 已落地的功能可在本地容器中观察。以下为后续 Sprint 待补部分。

### 后端


| 模块          | 子项                            | 优先级 | 备注                              |
| ----------- | ----------------------------- | --- | ------------------------------- |
| Worker      | 改造为 asynq 真实异步消费              | P1  | 现在是 inline goroutine，单机够用，多副本需要 |
| 进度推送        | WebSocket / SSE 把 task 状态推到前端 | P1  | 现在用 1.5s 轮询                     |
| Webhook     | provider 异步回调端点               | P1  | 配合 grok 真异步任务                   |
| 用户管理        | 列表 / 封禁 / 加点 / 改套餐 API        | P1  | 后台前端已留位                         |
| 充值订单        | 微信 / 支付宝 / Stripe 通道          | P1  | 现在只能 CDK                        |
| 优惠码         | promo_code 表 + CRUD + 校验      | P2  | CDK 已通；优惠码用于折扣                  |
| 请求日志        | 持久化 + 后台查询 API                | P2  | 目前只有 access log 文件              |
| 邀请返点        | 首充返点 + 终身分润落账                 | P2  | 表已建，逻辑未串                        |
| 健康检查 worker | 自动探测账号池 + 解熔断                 | P2  | 现在只有手动                          |


### 后台前端（占位页待对接）


| 页面                  | 状态               |
| ------------------- | ---------------- |
| 用户管理（列表 / 编辑 / 封禁）  | ⏳ 待对接（需上方后端 API） |
| 充值消费记录              | ⏳ 待对接            |
| 优惠码                 | ⏳ 待对接            |
| 兑换码 CDK 列表 + CSV 导出 | ⏳ 待对接（创建已通）      |
| 系统配置                | ✅ Sprint 9.5 已上线（代理 / OAuth / 完整 KV 视图） |
| 代理管理                | ✅ Sprint 9.5 已上线（CRUD + 测试 + 全局开关） |
| 请求日志                | ⏳ 待对接            |


### 部署 / 运维


| 项目                         | 状态                  |
| -------------------------- | ------------------- |
| 自签 / Let's Encrypt 证书脚本    | ⏳ 生产 nginx.conf 已留位 |
| K8s manifests / Helm chart | ⏳ Sprint 10         |
| Prometheus + Grafana 接入    | ⏳ Sprint 10         |
| OpenTelemetry trace 接入     | ⏳ Sprint 10         |


---

## Sprint 10 · 上线准备

- 性能压测（k6）
- 安全演练（越权 / 注入 / 限流绕过）
- 监控 / 告警接入
- 灰度发布脚本
- Runbook 演练

---

## 决策记录（ADR-Lite）


| 编号  | 决策                                                                        | 时间         | 备注                           |
| --- | ------------------------------------------------------------------------- | ---------- | ---------------------------- |
| 001 | 默认皮肤采用克莱因蓝 IKB `#002FA7` + 电光蓝 `#1E3DFF` 高光（仅视觉，可换）                       | 2026-04-27 | 替代之前的紫色方案；非项目身份              |
| 002 | 4 个二进制独立部署                                                                | 2026-04-27 | api/admin/openai/worker 解耦扩缩 |
| 003 | 端口段 17000-17999；MySQL 13306 / Redis 16379                                 | 2026-04-27 | 避开常用端口                       |
| 004 | 点数最小单位 0.01，DB int64 *100 存储                                              | 2026-04-27 | 避免浮点精度                       |
| 005 | 用户 API Key 仅创建时返回明文                                                       | 2026-04-27 | DB 仅存 SHA256+salt+last4      |
| 006 | provider 不持有 AES，credential 由 GenerationService 解密后注入 Request             | 2026-04-27 | 简化 provider 实现，集中密钥使用        |
| 007 | provider 默认 `mock`，env `KLEIN_PROVIDER_GPT/GROK=real` 切真实通道               | 2026-04-27 | 本地 / CI 友好，生产显式启用            |
| 008 | 后台 token 与用户 token 分别落 localStorage（`klein:token` vs `klein:admin:token`） | 2026-04-27 | 同源同浏览器隔离会话                   |
| 009 | 前端首页对未登录用户开放，生成等关键动作通过 `<LoginGate>` 浮层断点续做                               | 2026-04-27 | 避免「打开就是登录页」的硬墙体验             |
| 010 | 用户可见品牌名 = `gpt2api`；代码 module/CSS 类（`@kleinai/`*、`klein-*`）保留为内部命名空间      | 2026-04-27 | 把克莱因蓝降级为默认皮肤而非身份             |


---

## 风险与待办

- ⚠️ Grok 视频接口协议尚未对齐，需要在 Sprint 6 前完成调研，确认是否走第三方代理
- ⚠️ 支付通道优先支持 微信 + 支付宝；境外支付（Stripe）作为 Sprint 7 末尾扩展
- ⚠️ 账号池凭证加密密钥的运维流程（KMS / 手动）需在上线前敲定


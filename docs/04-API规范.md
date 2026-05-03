# 04 · API 规范

> 三套独立 API：
> - **用户端 API**：`https://api.klein.example` → `:17180`，前缀 `/api/v1`
> - **管理后台 API**：`https://admin-api.klein.example` → `:17188`，前缀 `/admin/api/v1`
> - **OpenAI 兼容 API**：`https://openai.klein.example` → `:17200`，前缀 `/v1`

---

## 1. 通用约定

### 1.1 请求

- HTTPS 强制；`Content-Type: application/json; charset=utf-8`（文件上传除外）
- 必带头：
  - `X-Request-Id`：UUID v4 / ULID（前端生成，后端透传）
  - `Accept-Language`：`zh-CN` / `en-US`（默认 zh-CN）
  - `User-Agent`：客户端标识 `KleinAI-Web/1.0.0`
- 鉴权头三选一：
  - 用户端：`Authorization: Bearer <accessToken>`
  - 后台：同上 + `X-Admin-Token`
  - OpenAI 兼容：`Authorization: Bearer sk-klein-xxxxx`
- 写操作必带 `Idempotency-Key: <UUIDv4>`（创建生成任务、充值下单、CDK 兑换等）
- 时间统一 ISO 8601：`2026-04-27T13:30:00.123+08:00`
- 分页：`?page=1&page_size=20`，最大 `page_size=100`

### 1.2 响应

```json
{
  "code": 0,
  "msg": "ok",
  "data": { },
  "trace_id": "01HX..."
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | int | 0 表示成功；非 0 看《02-后端规范》错误码 |
| `msg` | string | 中文提示，可直接展示 |
| `data` | any | 业务负载 |
| `trace_id` | string | 链路 ID，便于排障 |

分页响应统一：

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "list": [],
    "total": 123,
    "page": 1,
    "page_size": 20
  }
}
```

### 1.3 HTTP 状态码与业务码关系

- HTTP 200：业务成功（即便 `code != 0`，也用 200，**除特例外**）
- HTTP 401：仅用于 Token 失效 / 未登录
- HTTP 403：仅用于权限不足
- HTTP 429：限流
- HTTP 5xx：服务器/网关问题

### 1.4 文件上传

- 路径 `/api/v1/upload`
- `multipart/form-data`，单文件 ≤ 20MB（图片）/ ≤ 200MB（视频参考素材）
- 返回 `{ url, hash, size, mime }`，URL 走 CDN

### 1.5 WebSocket

- 入口：`wss://api.klein.example/ws?token=<accessToken>` （`:17280`）
- 心跳：客户端每 25s 发 `{"op":"ping"}`，服务端 `{"op":"pong"}`
- 任务进度推送：

```json
{
  "op": "task.progress",
  "task_id": "01HX...",
  "status": "running",
  "progress": 35,
  "ts": 1714200000123
}
```

```json
{
  "op": "task.done",
  "task_id": "01HX...",
  "status": "success",
  "results": [{ "url": "...", "thumb_url": "...", "duration_ms": 8000 }]
}
```

---

## 2. 用户端 API（`/api/v1`）

### 2.1 鉴权 / 账户

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/register` | 注册（手机号 / 邮箱） |
| POST | `/auth/login` | 登录（密码） |
| POST | `/auth/login/sms` | 短信验证码登录 |
| POST | `/auth/captcha/sms` | 发送短信验证码 |
| POST | `/auth/refresh` | 刷新 Token |
| POST | `/auth/logout` | 登出（吊销 refresh） |
| POST | `/auth/password/reset` | 重置密码 |
| GET  | `/me` | 当前用户基本信息 + 余额 + 套餐 |
| PATCH| `/me` | 修改昵称 / 头像 / 偏好 |
| POST | `/me/password` | 修改密码（需旧密码） |
| GET  | `/me/devices` | 在线设备列表 |
| POST | `/me/devices/:id/revoke` | 强制下线某设备 |

#### `/auth/login` 示例

请求：
```json
POST /api/v1/auth/login
{
  "account": "user@example.com",
  "password": "******",
  "captcha_id": "xxx",
  "captcha_code": "1234"
}
```

响应：
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "access_token": "ey...",
    "refresh_token": "ey...",
    "expires_in": 7200,
    "user": {
      "uuid": "...",
      "username": "Panda9527",
      "avatar": "...",
      "points": 25600,
      "plan_code": "pro",
      "plan_expire_at": "2026-12-31T23:59:59+08:00"
    }
  }
}
```

### 2.2 创作中心（生图 / 生视频）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/models` | 模型市场列表（按 kind 筛选） |
| GET  | `/models/:code` | 模型详情 |
| POST | `/gen/image` | 文生图 / 图生图（需 Idempotency-Key） |
| POST | `/gen/video` | 文生视频 / 图生视频（需 Idempotency-Key） |
| GET  | `/gen/tasks/:task_id` | 任务详情（含进度与结果） |
| POST | `/gen/tasks/:task_id/cancel` | 取消任务（仅 pending/running） |
| GET  | `/gen/tasks` | 历史任务（生成历史） |
| POST | `/gen/results/:id/upscale` | 放大 |
| POST | `/gen/results/:id/variation` | 再次生成（变体） |
| POST | `/gen/results/:id/favorite` | 收藏 / 取消收藏 |
| POST | `/upload` | 通用上传（参考图等） |
| GET  | `/prompt/history` | 提示词历史 |
| GET  | `/inspirations` | 灵感广场（推荐 / 全部 / 插画 / 摄影 / 3D / 二次元） |

#### `/gen/image` 示例

```json
POST /api/v1/gen/image
Idempotency-Key: 9b1d... 
{
  "model_code": "img-v3",
  "mode": "t2i",
  "prompt": "一只可爱的猫咪，戴着宇航员头盔...",
  "neg_prompt": "",
  "ratio": "4:3",
  "count": 4,
  "seed": null,
  "ref_url": null
}
```

响应（异步）：
```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "task_id": "01HX...",
    "status": "pending",
    "estimate_seconds": 8,
    "cost_points": 1600
  }
}
```

后续通过 WebSocket 或 `GET /gen/tasks/:task_id` 拿结果。

#### `/gen/video` 关键参数

```json
{
  "model_code": "vid-v1",
  "mode": "t2v",
  "prompt": "...",
  "ratio": "16:9",
  "duration": 8,
  "fps": 24,
  "ref_image_url": null
}
```

### 2.3 KEY 管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/keys` | 列表 |
| POST | `/keys` | 创建（**仅创建时返回明文 Key**） |
| PATCH| `/keys/:id` | 改名 / 改作用域 / 改限额 |
| POST | `/keys/:id/disable` | 停用 |
| POST | `/keys/:id/enable` | 启用 |
| DELETE | `/keys/:id` | 删除 |
| GET  | `/keys/:id/stats` | 用量统计（QPS / 调用数 / 消耗点数） |

### 2.4 余额 / 充值 / 兑换

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/wallet` | 余额 + 冻结点数 + 套餐 |
| GET  | `/wallet/logs` | 余额明细（流水） |
| GET  | `/recharge/products` | 充值套餐列表 |
| POST | `/recharge/orders` | 创建充值订单（返回支付参数） |
| GET  | `/recharge/orders/:order_no` | 查询订单状态 |
| POST | `/recharge/orders/:order_no/cancel` | 取消订单 |
| GET  | `/recharge/records` | 充值记录 |
| GET  | `/consume/records` | 消费记录 |
| POST | `/promo/preview` | 预览优惠码效果（金额折算） |
| POST | `/redeem` | 兑换 CDK |

#### CDK 兑换示例

```json
POST /api/v1/redeem
{ "code": "KLEIN-PRO-2026Q1-XXXXXXXX" }
```

```json
{
  "code": 0,
  "msg": "兑换成功",
  "data": {
    "reward": { "points": 10000, "plan": "pro", "days": 30 },
    "wallet": { "points": 35600 }
  }
}
```

### 2.5 邀请中心

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/invite/info` | 邀请码 / 海报 / 规则 |
| GET  | `/invite/stats` | 邀请人数 / 累计返点 |
| GET  | `/invite/list` | 邀请明细（被邀请人 + 状态） |
| GET  | `/invite/rewards` | 返点流水 |

### 2.6 公告 / 帮助

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/announcements` | 公告列表（按时间 + 状态） |
| GET  | `/announcements/:id` | 公告详情 |
| GET  | `/help/docs` | 帮助文档分类 |
| GET  | `/help/docs/:slug` | 帮助文档 |

---

## 3. 管理后台 API（`/admin/api/v1`）

> 全部接口需 `AuthAdminMiddleware` + RBAC 权限校验。
> 写操作记录 `admin_audit_log`。

### 3.1 鉴权

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/login` | 后台登录（密码 + 图形验证码 + 二次校验） |
| POST | `/auth/logout` | 登出 |
| POST | `/auth/refresh` | 刷新 |
| GET  | `/auth/me` | 当前管理员 + 角色 + 权限点 |
| GET  | `/roles` | 角色列表 |
| POST | `/roles` | 新增角色 |
| PATCH| `/roles/:id` | 修改角色 |
| DELETE| `/roles/:id` | 删除角色 |
| GET  | `/permissions` | 权限点字典 |

### 3.2 Token 管理（即"账号池"管理）

> 一线运营最常用模块；批量导入/导出/启停/熔断恢复。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/accounts` | 列表（筛选 provider / status / group） |
| POST | `/accounts` | 新增单个账号 |
| POST | `/accounts/import` | **批量导入**（CSV / TXT，每行一个 token） |
| POST | `/accounts/export` | 批量导出（脱敏） |
| GET  | `/accounts/:id` | 详情（含使用统计） |
| PATCH| `/accounts/:id` | 编辑 |
| POST | `/accounts/:id/test` | 健康测试（实时调用一次最低成本接口） |
| POST | `/accounts/:id/disable` | 停用 |
| POST | `/accounts/:id/enable` | 启用 |
| POST | `/accounts/:id/reset-cooldown` | 解除熔断 |
| POST | `/accounts/batch/disable` | 批量停用 |
| POST | `/accounts/batch/delete` | 批量删除 |
| GET  | `/accounts/:id/stats` | 调用统计（成功率、QPS、p95、累计点数） |
| GET  | `/account-groups` | 分组列表 |
| POST | `/account-groups` | 新增分组 |
| PATCH| `/account-groups/:id` | 编辑（含调度策略） |
| POST | `/account-groups/:id/members` | 添加成员（批量 account_id） |
| DELETE| `/account-groups/:id/members/:account_id` | 移除成员 |

#### 批量导入格式（CSV）

```
provider,name,auth_type,credential,base_url,weight,daily_quota,group_code,remark
gpt,gpt-001,api_key,sk-xxx,https://api.openai.com,10,0,gpt-image-default,
grok,grok-001,cookie,"<long cookie>",https://x.ai,10,0,grok-video-default,
```

### 3.3 用户管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/users` | 列表（搜索：手机/邮箱/UUID/邀请码） |
| GET  | `/users/:id` | 详情 |
| PATCH| `/users/:id` | 修改昵称 / 套餐 / 状态 |
| POST | `/users/:id/disable` | 封禁 |
| POST | `/users/:id/enable` | 解封 |
| POST | `/users/:id/points/grant` | 手动赠送点数 |
| POST | `/users/:id/points/deduct` | 手动扣点 |
| POST | `/users/:id/reset-password` | 重置密码（短信 / 邮件下发临时密码） |
| GET  | `/users/:id/wallet/logs` | 钱包流水 |
| GET  | `/users/:id/tasks` | 任务历史 |
| GET  | `/users/:id/keys` | 该用户的 API Key |

### 3.4 充值 / 消费记录

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/recharge/records` | 充值记录（按时间 / 渠道 / 用户筛选） |
| GET  | `/recharge/records/:id` | 详情 |
| POST | `/recharge/records/:id/refund` | 退款（需理由 + 二次校验） |
| GET  | `/consume/records` | 消费记录 |
| GET  | `/consume/records/:id` | 详情 |
| GET  | `/wallet/logs` | 总账流水（跨用户） |
| GET  | `/stats/revenue` | 收入统计（日/周/月） |
| GET  | `/stats/consume` | 消费统计 |

### 3.5 优惠码

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/promo-codes` | 列表 |
| POST | `/promo-codes` | 创建 |
| PATCH| `/promo-codes/:id` | 编辑 |
| POST | `/promo-codes/:id/disable` | 停用 |
| GET  | `/promo-codes/:id/uses` | 使用记录 |

### 3.6 兑换码 CDK

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/redeem-batches` | 批次列表 |
| POST | `/redeem-batches` | 新建批次（指定数量、奖励、有效期，自动生成 CDK） |
| GET  | `/redeem-batches/:id` | 批次详情（剩余、已用） |
| POST | `/redeem-batches/:id/disable` | 停用整批 |
| GET  | `/redeem-batches/:id/codes` | 单个 CDK 列表 |
| GET  | `/redeem-batches/:id/export` | 导出 CSV（**仅未使用 CDK**） |
| POST | `/redeem-codes/:id/disable` | 单条作废 |

### 3.7 系统配置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/configs` | 全部 key 列表 |
| GET  | `/configs/:key` | 取单个 |
| PUT  | `/configs/:key` | 修改 |
| GET  | `/dicts` | 字典分组 |
| POST | `/dicts` | 新增 |
| PATCH| `/dicts/:id` | 编辑 |
| DELETE| `/dicts/:id` | 删除 |
| GET  | `/announcements` | 公告管理 |
| POST | `/announcements` | 新增 |
| PATCH| `/announcements/:id` | 编辑 |
| DELETE| `/announcements/:id` | 删除 |
| GET  | `/models` | 模型管理（生图 / 生视频） |
| POST | `/models` | 新增模型 |
| PATCH| `/models/:id` | 编辑（单价、绑定分组、套餐限制） |
| GET  | `/plans` | 套餐管理 |
| POST | `/plans` | 新增 |
| PATCH| `/plans/:id` | 编辑 |

### 3.8 请求日志

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/logs/requests` | 请求日志（多维筛选：用户 / Key / 路径 / 状态 / 时间） |
| GET  | `/logs/requests/:trace_id` | 单条详情 |
| GET  | `/logs/pool-calls` | 账号池调用日志 |
| GET  | `/logs/audit` | 后台操作审计日志 |
| GET  | `/logs/tasks` | 生成任务日志 |
| POST | `/logs/export` | 导出（异步任务，邮件回执下载链接） |

### 3.9 概览看板

| 方法 | 路径 | 说明 |
|------|------|------|
| GET  | `/dashboard/summary` | 今日 GMV / 用户数 / 任务数 / 成功率 |
| GET  | `/dashboard/charts/revenue` | 收入折线 |
| GET  | `/dashboard/charts/usage` | 用量折线 |
| GET  | `/dashboard/pool/health` | 账号池整体健康 |

---

## 4. OpenAI 兼容 API（`/v1`）

> 让用户用 `openai-python` / 任意 SDK，仅替换 `base_url` 和 `api_key` 即可调用。
> 鉴权：`Authorization: Bearer sk-klein-xxxxx`（用户在 KEY 管理页生成）。

### 4.1 图像生成（OpenAI Images API 兼容）

```
POST /v1/images/generations
```

请求体（兼容 OpenAI）：
```json
{
  "model": "img-v3",
  "prompt": "...",
  "n": 4,
  "size": "1024x768",
  "quality": "hd",
  "response_format": "url"
}
```

响应：
```json
{
  "created": 1714200000,
  "data": [
    { "url": "https://cdn..../1.png", "revised_prompt": "..." },
    { "url": "https://cdn..../2.png" }
  ]
}
```

错误响应（OpenAI 风格）：
```json
{
  "error": {
    "type": "rate_limit_exceeded",
    "code": 429301,
    "message": "操作过于频繁",
    "trace_id": "..."
  }
}
```

### 4.2 图像编辑 / 变体（兼容）

```
POST /v1/images/edits          (multipart: image, mask, prompt)
POST /v1/images/variations     (multipart: image)
```

### 4.3 视频生成（兼容 OpenAI Sora 风格协议 + Grok 自有协议）

主路由：

```
POST /v1/videos/generations
```

请求：
```json
{
  "model": "vid-v1",
  "prompt": "...",
  "size": "1280x720",
  "duration": 8,
  "ref_image_url": "..."
}
```

响应（异步）：
```json
{
  "id": "video_01HX...",
  "object": "video.generation",
  "status": "queued",
  "created": 1714200000
}
```

```
GET  /v1/videos/generations/{id}        # 查询任务（pending/running/completed/failed）
DELETE /v1/videos/generations/{id}      # 取消
```

完成后：
```json
{
  "id": "video_01HX...",
  "object": "video.generation",
  "status": "completed",
  "video_url": "https://cdn..../v.mp4",
  "thumbnail_url": "https://cdn..../t.jpg",
  "duration_ms": 8000,
  "size": "1280x720",
  "created": 1714200000,
  "completed": 1714200060
}
```

### 4.4 模型与计费查询

```
GET  /v1/models                     # 兼容 OpenAI，返回 KleinAI 全模型
GET  /v1/dashboard/billing/credit_grants    # 余额查询（返回点数）
```

### 4.5 限流与错误

- 429 时同时返回 `Retry-After` 与 `X-RateLimit-*` 头
- 错误码映射 `502201 → upstream_unavailable`（兼容字段 `error.type`）
- 任意错误响应均携带 `trace_id`

---

## 5. 接口安全细则

1. **越权**：所有 `/api/v1/**` 接口必须显式带 `user_id` 过滤；`/admin/**` 必须 `permission` 校验。
2. **签名（开放 API 可选强制）**：
   ```
   X-Klein-Ts: 1714200000
   X-Klein-Sign: HMAC_SHA256(secret, ts + method + path + sha256(body))
   ```
3. **回放防御**：5 分钟时间窗 + Redis 记录 nonce。
4. **CORS**：白名单仅前端域名；管理后台单独配置。
5. **CSRF**：用户端用 Bearer Token 天然免疫；后台同源策略 + SameSite=Lax 双保险。
6. **风控**：登录 / 注册 / 充值 / 兑换接口走 IP+UID 双限流，必要时拉起验证码。
7. **数据脱敏**：日志、响应内的手机号 / 邮箱 / Key / 第三方 token 一律脱敏。

---

## 6. 文档与联调

- Swagger：`/swagger/index.html`，仅 dev/staging 开放，生产关闭；同时通过 `swag init` 生成 `docs/swagger.json`。
- Apifox 项目：建议建三个空间分别对应三套 API。
- Postman 集合：每个 PR 必须更新 `docs/api/postman_collection.json`（如启用）。
- Mock：dev 环境 `?mock=1` 返回固定示例数据，便于前端先行联调。

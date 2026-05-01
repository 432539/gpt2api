# scripts

运维/自测辅助脚本集合。

> **⚠️ 注意** 本目录下所有 `--admin-email` / `--admin-pass` / `--user-email` / `--user-pass` 以及
> 形如 `admin@smoke.test`、`Admin123456`、`User123456` 这样的字面量,都是**冒烟脚本自己创建测试账号时用的临时凭证**,
> 它们 **不是** 部署后的默认管理员账号密码。
>
> GPT2API 本身不内置任何默认账号 —— **首位访问 `/register` 的用户自动成为 admin**(见根目录 README.md 的部署说明)。

## smoke.mjs · e2e 冒烟

对已启动的后端(本地 `go run` 或 `docker compose up -d`)做一轮端到端闭环自检。

### 覆盖用例


| #   | 用例                       | 说明                                                      |
| --- | ------------------------ | ------------------------------------------------------- |
| 1   | `/healthz`               | 后端可达                                                    |
| 2   | 首位用户自动 admin             | 若 users 表为空,register 的第一个账号自动拿到 admin 角色                |
| 3   | 普通用户注册 / 登录              |                                                         |
| 4   | `/api/me`、`/api/me/menu` | 断言 admin/user 各自的 role、permissions、menu 非空              |
| 5   | API Keys CRUD            | 用户视角 create / list / patch(禁用)/ delete                  |
| 6   | 越权校验                     | user token 访问 `/api/admin/*` 应 401/403;匿名访问 admin 应 401 |
| 7   | Admin 用户 / 分组列表          |                                                         |
| 8   | 调账 `+delta`              | 正确密码过,错误密码被拒(403),流水可查,用户余额同步                           |
| 9   | 审计日志                     | 含 `users.credit.adjust` 等动作                             |
| 10  | 备份链路                     | 创建 → 列表包含 → 下载 → 删除(二次密码);宿主缺 `mysqldump` 时跳过           |


### 用法

前置条件:Node ≥ 18(原生 fetch / FormData),后端已启动。

```bash
cd scripts
npm run smoke
```

或直接指定参数:

```bash
node scripts/smoke.mjs \
  --base http://localhost:18080 \
  --admin-email admin@smoke.test \
  --admin-pass  Admin123456 \
  --user-email  user@smoke.test \
  --user-pass   User123456
```

- `--keep true` 保留脚本创建的 Key、备份文件(便于后续手动验证)
- 环境变量 `GPT2API_BASE` 可覆盖 `--base`

### 退出码

- `0` 全部通过
- `1` 至少一条 FAIL
- `2` 脚本级异常(如 `/healthz` 不可达)

### 复跑行为

脚本是幂等的——已经存在的账号走登录路径,已经存在的 key 不影响新建。但它假设 "首位用户 = admin" 那步只在空库时成立,所以:

- 对全新库:能完整跑通
- 对已经跑过的库:要么复用相同 admin 账号(`--admin-email` 指向那个),要么清空 users 表再跑

### 与 CI 配合

GitHub Actions 示例骨架:

```yaml
- name: docker compose up
  run: docker compose -f deploy/docker-compose.yml up -d --wait

- name: wait backend
  run: curl --retry 30 --retry-delay 2 --retry-connrefused http://localhost:18080/healthz

- name: smoke
  run: node scripts/smoke.mjs --base http://localhost:18080
```

#!/usr/bin/env node
/**
 * GPT2API · e2e 冒烟脚本
 *
 * 覆盖:
 *   1. /healthz
 *   2. 首位用户注册 -> 自动 admin
 *   3. 普通用户注册(user2)
 *   4. admin + user2 登录、/api/me、/api/me/menu
 *   5. API Keys CRUD(user2 视角)
 *   6. 越权校验:user2 访问 /api/admin/* -> 401/403
 *   7. Admin 用户列表 / 分组列表
 *   8. Admin 调账(X-Admin-Confirm 二次密码)+ 流水校验
 *   9. 审计日志包含本次关键动作
 *  10. 备份:立即创建 -> 列表 -> 下载(存盘) -> 删除(二次密码)
 *      mysqldump 不可用时自动跳过该组用例,不判失败
 *
 * 使用:
 *   node scripts/smoke.mjs \
 *     --base http://localhost:18080 \
 *     --admin-email   admin@smoke.test \
 *     --admin-pass    Admin123456 \
 *     --user-email    user@smoke.test \
 *     --user-pass     User123456
 *
 * 或直接 `npm run smoke` (在 web/package.json 也注册了)。
 *
 * ⚠️ 上面的 admin@smoke.test / Admin123456 / user@smoke.test / User123456
 *    只是本脚本自己要创建的"测试账号"的默认参数,**不是** GPT2API 部署后的默认凭证。
 *    正式部署没有任何内置账号,首位注册者自动成为 admin(见根 README.md)。
 *
 * 要求:
 *   - Node >= 18(用到全局 fetch / FormData / Blob / AbortController)
 *   - 后端 gpt2api 已启动,MySQL / Redis 连接正常
 *   - 为了实现"首位用户 = admin",脚本期望 users 表为空;若不空,仍会尝试登录既有 admin。
 */

import { argv, env, exit } from 'node:process'
import { writeFile } from 'node:fs/promises'

// ---------- 参数 ----------
const args = parseArgs(argv.slice(2))
const BASE = stripSlash(args['base'] || env.GPT2API_BASE || 'http://localhost:18080')
const ADMIN_EMAIL = args['admin-email'] || `admin+${Date.now()}@smoke.test`
const ADMIN_PASS  = args['admin-pass']  || 'Admin123456'
const USER_EMAIL  = args['user-email']  || `user+${Date.now()}@smoke.test`
const USER_PASS   = args['user-pass']   || 'User123456'
const KEEP_USER   = args['keep'] === 'true'

let pass = 0
let fail = 0
let skipped = 0
const results = []

function parseArgs(arr) {
  const out = {}
  for (let i = 0; i < arr.length; i++) {
    const a = arr[i]
    if (a.startsWith('--')) {
      const k = a.slice(2)
      const v = arr[i + 1] && !arr[i + 1].startsWith('--') ? arr[++i] : 'true'
      out[k] = v
    }
  }
  return out
}

function stripSlash(u) { return u.replace(/\/+$/, '') }

const CYAN = '\x1b[36m'; const RED = '\x1b[31m'; const GREEN = '\x1b[32m'
const YELLOW = '\x1b[33m'; const DIM = '\x1b[2m'; const RESET = '\x1b[0m'

function step(title) { console.log(`\n${CYAN}▶ ${title}${RESET}`) }
function ok(msg)     { pass++; results.push(['PASS', msg]); console.log(`  ${GREEN}✓${RESET} ${msg}`) }
function bad(msg, extra) {
  fail++; results.push(['FAIL', msg, extra])
  console.log(`  ${RED}✗ ${msg}${RESET}`)
  if (extra) console.log(`    ${DIM}${extra}${RESET}`)
}
function skip(msg, why) {
  skipped++; results.push(['SKIP', msg, why])
  console.log(`  ${YELLOW}○ ${msg}${RESET}  ${DIM}(${why})${RESET}`)
}

/**
 * 统一 HTTP 调用。返回 { status, body } 或抛。
 *   - body 是 JSON / Buffer(根据 Content-Type)
 *   - 不会基于业务 code 报错;由调用方自行校验。
 */
async function call(method, path, { token, body, headers = {}, raw = false } = {}) {
  const url = path.startsWith('http') ? path : BASE + path
  const h = { ...headers }
  if (token) h['Authorization'] = `Bearer ${token}`
  let payload
  if (body instanceof FormData) {
    payload = body
  } else if (body !== undefined) {
    h['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }
  const res = await fetch(url, { method, headers: h, body: payload })
  const ct = res.headers.get('content-type') || ''
  let data
  if (raw) {
    data = Buffer.from(await res.arrayBuffer())
  } else if (ct.includes('application/json')) {
    data = await res.json()
  } else {
    data = await res.text()
  }
  return { status: res.status, body: data, headers: res.headers }
}

function isEnvelope(v) { return v && typeof v === 'object' && 'code' in v && 'data' in v }
function unwrap(r)     { return isEnvelope(r.body) ? r.body.data : r.body }
function err(r)        { return isEnvelope(r.body) ? r.body.message : typeof r.body === 'string' ? r.body : JSON.stringify(r.body) }

// ========== 1. 健康 ==========
async function checkHealth() {
  step('1. 健康检查')
  try {
    const r = await call('GET', '/healthz')
    if (r.status === 200) ok(`/healthz 200 OK`)
    else bad(`/healthz 非 200 (${r.status})`, err(r))
  } catch (e) {
    bad('/healthz 不可达', e.message)
    throw new Error('后端未启动,终止')
  }
}

// ========== 2. 注册 + 登录 ==========
async function registerOrLogin(email, password) {
  // 后端 409 语义返回 HTTP 200 + code=40900(业务错误),只有 401/403/500 才用 HTTP 状态。
  const reg = await call('POST', '/api/auth/register', { body: { email, password, nickname: email.split('@')[0] } })
  const code = reg.body?.code
  if (code === 0) return { created: true, user: reg.body.data }
  if (code === 40900) return { created: false, user: null }
  throw new Error(`register 失败 status=${reg.status} code=${code}: ${err(reg)}`)
}

async function login(email, password) {
  const r = await call('POST', '/api/auth/login', { body: { email, password } })
  if (r.status !== 200 || r.body.code !== 0) {
    throw new Error(`login 失败 ${r.status}: ${err(r)}`)
  }
  return r.body.data
}

let adminToken = ''
let adminId = 0
let userToken = ''
let userId = 0

async function setupAccounts() {
  step('2. 账号 bootstrap(首位=admin,其次普通用户)')
  const a = await registerOrLogin(ADMIN_EMAIL, ADMIN_PASS)
  if (a.created) ok(`admin 注册成功: ${ADMIN_EMAIL}`)
  else ok(`admin 已存在, 复用: ${ADMIN_EMAIL}`)

  const adminLogin = await login(ADMIN_EMAIL, ADMIN_PASS)
  adminToken = adminLogin.token.access_token
  adminId = adminLogin.user.id
  if (adminLogin.user.role !== 'admin') {
    bad(`首位用户 role != admin (得到 ${adminLogin.user.role});若是复用老数据库,可手动升级该用户再重试`)
    throw new Error('admin bootstrap 失败')
  }
  ok(`admin 登录 OK, id=${adminId}, role=admin`)

  const u = await registerOrLogin(USER_EMAIL, USER_PASS)
  if (u.created) ok(`普通用户注册成功: ${USER_EMAIL}`)
  else ok(`普通用户已存在, 复用: ${USER_EMAIL}`)

  const userLogin = await login(USER_EMAIL, USER_PASS)
  userToken = userLogin.token.access_token
  userId = userLogin.user.id
  if (userLogin.user.role !== 'user') {
    bad(`普通用户 role != user (得到 ${userLogin.user.role})`)
  } else {
    ok(`普通用户登录 OK, id=${userId}, role=user`)
  }
}

// ========== 3. /me /menu ==========
async function checkMe() {
  step('3. /api/me 与 /api/me/menu')
  for (const [name, token] of [['admin', adminToken], ['user', userToken]]) {
    const me = await call('GET', '/api/me', { token })
    const d = unwrap(me)
    if (me.status === 200 && d?.user?.email) ok(`${name} /me 返回正常 (perms=${d.permissions?.length ?? 0})`)
    else bad(`${name} /me 失败 ${me.status}`, err(me))

    const menu = await call('GET', '/api/me/menu', { token })
    const m = unwrap(menu)
    if (menu.status === 200 && Array.isArray(m?.menu) && m.menu.length > 0) {
      ok(`${name} /me/menu 返回 ${m.menu.length} 个顶级菜单`)
    } else {
      bad(`${name} /me/menu 异常`, err(menu))
    }
  }
}

// ========== 4. Key CRUD ==========
let createdKeyId = 0
let createdKeyPlain = ''
async function checkKeyCRUD() {
  step('4. API Keys CRUD(普通用户视角)')

  const c = await call('POST', '/api/keys', { token: userToken, body: {
    name: 'smoke-key', quota_limit: 0, rpm: 60, tpm: 60000,
  }})
  const cd = unwrap(c)
  if (c.status === 200 && cd?.key?.startsWith('sk-')) {
    createdKeyId = cd.record.id
    createdKeyPlain = cd.key
    ok(`创建 Key 成功, id=${createdKeyId}, 前缀=${cd.record.key_prefix}`)
  } else {
    bad('创建 Key 失败', err(c))
    return
  }

  const l = await call('GET', '/api/keys', { token: userToken })
  const ld = unwrap(l)
  if (l.status === 200 && Array.isArray(ld?.list) && ld.list.length >= 1) {
    ok(`列表 Keys 返回 ${ld.list.length} 条`)
  } else {
    bad('列表 Keys 异常', err(l))
  }

  const u = await call('PATCH', `/api/keys/${createdKeyId}`, { token: userToken, body: { enabled: false }})
  if (u.status === 200 && u.body.code === 0) ok('禁用 Key 成功')
  else bad('禁用 Key 失败', err(u))

  // 清理(若未 --keep=true)
  if (!KEEP_USER) {
    const del = await call('DELETE', `/api/keys/${createdKeyId}`, { token: userToken })
    if (del.status === 200) ok('删除 Key 成功')
    else bad('删除 Key 失败', err(del))
  }
}

// ========== 5. 越权 ==========
async function checkForbidden() {
  step('5. 越权:普通用户访问 admin 接口')
  const paths = [
    ['GET',  '/api/admin/users'],
    ['GET',  '/api/admin/groups'],
    ['GET',  '/api/admin/audit/logs'],
    ['GET',  '/api/admin/system/backup'],
  ]
  for (const [m, p] of paths) {
    const r = await call(m, p, { token: userToken })
    if (r.status === 401 || r.status === 403) ok(`${m} ${p} -> ${r.status} (禁止访问 ✓)`)
    else bad(`${m} ${p} 越权未被拦截 (得到 ${r.status})`)
  }
  // 匿名访问
  const anon = await call('GET', '/api/admin/users')
  if (anon.status === 401) ok('匿名访问 /api/admin/users -> 401 ✓')
  else bad('匿名访问 admin 接口未被拦截', `got ${anon.status}`)
}

// ========== 6. Admin 用户/分组 ==========
async function checkAdminUsers() {
  step('6. Admin 用户列表 / 分组')
  const ul = await call('GET', '/api/admin/users?limit=50', { token: adminToken })
  const ud = unwrap(ul)
  if (ul.status === 200 && Array.isArray(ud?.items) && ud.items.length >= 2) {
    ok(`用户列表返回 ${ud.items.length} 条(total=${ud.total})`)
  } else {
    bad('用户列表异常', err(ul))
  }

  const gl = await call('GET', '/api/admin/groups', { token: adminToken })
  const gd = unwrap(gl)
  if (gl.status === 200 && Array.isArray(gd?.items)) {
    ok(`分组列表返回 ${gd.items.length} 条`)
  } else {
    bad('分组列表异常', err(gl))
  }
}

// ========== 7. Admin 调账 + 流水 ==========
async function checkCreditAdjust() {
  step('7. Admin 调账(加 10000 厘)+ 校验流水')
  const delta = 10000
  const adj = await call('POST', `/api/admin/users/${userId}/credits/adjust`, {
    token: adminToken,
    headers: { 'X-Admin-Confirm': ADMIN_PASS },
    body: { delta, remark: 'smoke test topup', ref_id: `smoke-${Date.now()}` },
  })
  if (adj.status === 200 && adj.body.code === 0) {
    ok(`调账成功,+${delta}`)
  } else {
    bad('调账失败', err(adj))
    return
  }

  // 错误的二次密码应当被拒
  const wrong = await call('POST', `/api/admin/users/${userId}/credits/adjust`, {
    token: adminToken,
    headers: { 'X-Admin-Confirm': 'wrong-password' },
    body: { delta: 100, remark: 'should be rejected' },
  })
  if (wrong.status === 401 || wrong.status === 403) ok('错误 X-Admin-Confirm -> 被拒绝 ✓')
  else bad('错误 X-Admin-Confirm 未被拒绝', `got ${wrong.status}`)

  // 流水
  const logs = await call('GET', `/api/admin/users/${userId}/credit-logs?limit=10`, { token: adminToken })
  const ld = unwrap(logs)
  if (logs.status === 200 && Array.isArray(ld?.items) && ld.items.some((x) => x.type === 'admin_adjust' && x.amount === delta)) {
    ok(`流水中找到 admin_adjust 记录 (+${delta})`)
  } else {
    bad('流水校验失败', err(logs))
  }

  // me 余额变化
  const me = await call('GET', '/api/me', { token: userToken })
  const mu = unwrap(me)
  if (me.status === 200 && mu?.user?.credit_balance >= delta) {
    ok(`用户余额含本次入账 (余额=${mu.user.credit_balance})`)
  } else {
    bad('用户余额未反映调账', JSON.stringify(mu?.user ?? {}))
  }
}

// ========== 8. 审计 ==========
async function checkAudit() {
  step('8. 审计日志包含关键操作')
  const r = await call('GET', '/api/admin/audit/logs?limit=50', { token: adminToken })
  const d = unwrap(r)
  if (r.status !== 200 || !Array.isArray(d?.items)) {
    bad('审计日志接口异常', err(r)); return
  }
  const actions = new Set(d.items.map((x) => x.action))
  // 后端对每个 admin 写操作都会自动派生一条 action,同时 handler 再显式 Record 一条。
  // 我们只要能找到"调账"相关的任意一条即可。
  const anyMatch = [...actions].some((x) => /credit.*adjust|users\.credit/.test(x))
  if (anyMatch) ok(`审计包含调账记录 (samples: ${[...actions].slice(0, 5).join(', ')})`)
  else bad('审计未包含调账动作', `已见:${[...actions].slice(0, 10).join(', ')}`)
}

// ========== 9. 备份 ==========
async function checkBackup() {
  step('9. 数据库备份(需要后端容器/宿主装有 mysqldump)')
  const create = await call('POST', '/api/admin/system/backup', {
    token: adminToken, body: { include_data: true },
  })
  if (create.status !== 200 || create.body.code !== 0) {
    const e = err(create)
    if (/mysqldump|exec|not found|no such file/i.test(e)) {
      skip('创建备份', `环境缺 mysqldump 或配置不可用:${e}`)
      return
    }
    bad('创建备份失败', e); return
  }
  const cd = unwrap(create)
  const backupId = cd.backup_id
  ok(`创建备份成功, id=${backupId}, size=${cd.size_bytes ?? '?'}B`)

  const list = await call('GET', '/api/admin/system/backup', { token: adminToken })
  const ld = unwrap(list)
  if (list.status === 200 && Array.isArray(ld?.items) && ld.items.some((x) => x.backup_id === backupId)) {
    ok('备份列表包含刚创建的 ID')
  } else {
    bad('备份列表异常', err(list))
  }

  // 下载
  const dl = await call('GET', `/api/admin/system/backup/${backupId}/download`, { token: adminToken, raw: true })
  if (dl.status === 200 && dl.body.length > 0) {
    const outPath = `smoke-backup-${backupId}.sql.gz`
    try { await writeFile(outPath, dl.body); ok(`下载备份成功, 写入 ${outPath} (${dl.body.length} B)`) }
    catch (e) { bad('写入下载文件失败', e.message) }
  } else {
    bad('下载备份失败', `status=${dl.status}`)
  }

  // 删除(高危:带二次密码)
  const del = await call('DELETE', `/api/admin/system/backup/${backupId}`, {
    token: adminToken, headers: { 'X-Admin-Confirm': ADMIN_PASS },
  })
  if (del.status === 200 && del.body.code === 0) ok('删除备份成功')
  else bad('删除备份失败', err(del))
}

// ========== 主流程 ==========
async function main() {
  console.log(`${DIM}=== GPT2API smoke ===${RESET}`)
  console.log(`base:  ${BASE}`)
  console.log(`admin: ${ADMIN_EMAIL}`)
  console.log(`user:  ${USER_EMAIL}`)

  await checkHealth()
  await setupAccounts()
  await checkMe()
  await checkKeyCRUD()
  await checkForbidden()
  await checkAdminUsers()
  await checkCreditAdjust()
  await checkAudit()
  await checkBackup()

  console.log(`\n${DIM}========================================${RESET}`)
  console.log(`${GREEN}PASS${RESET}: ${pass}   ${RED}FAIL${RESET}: ${fail}   ${YELLOW}SKIP${RESET}: ${skipped}`)
  if (fail > 0) {
    console.log(`\n${RED}失败明细:${RESET}`)
    for (const [tag, msg, extra] of results) {
      if (tag === 'FAIL') console.log(`  · ${msg}${extra ? ` — ${extra}` : ''}`)
    }
    exit(1)
  }
  exit(0)
}

main().catch((e) => {
  console.error(`\n${RED}脚本异常退出:${RESET} ${e.message}`)
  exit(2)
})

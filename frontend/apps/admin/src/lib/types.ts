// 后台管理 - 与后端 dto / response 对齐的前端类型。
// 注意：所有 *_points / points 字段单位为「点 *100」，展示请除以 100。

export interface ApiBody<T> {
  code: number;
  msg: string;
  data?: T;
  trace_id?: string;
}

export interface PageData<T> {
  list: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface AdminLoginResp {
  id: number;
  username: string;
  nickname: string;
  role_id: number;
  token: {
    access_token: string;
    refresh_token: string;
    token_type: string;
    access_expire_in: number;
    refresh_expire_in: number;
  };
}

export interface AdminMe {
  id: number;
  username: string;
  nickname: string;
  email?: string;
  role_id: number;
  role_code: string;
  role_name: string;
}

/** 账号池条目 */
export interface AdminUserItem {
  id: number;
  uuid: string;
  email?: string;
  phone?: string;
  username?: string;
  avatar?: string;
  points: number;
  frozen_points: number;
  total_recharge: number;
  plan_code: string;
  plan_expire_at?: number;
  inviter_id?: number;
  invite_code: string;
  status: 0 | 1 | number;
  register_ip?: string;
  last_login_at?: number;
  last_login_ip?: string;
  created_at: number;
  updated_at: number;
}

export interface AdminUserCreateBody {
  account: string;
  password: string;
  username?: string;
  points?: number;
  status?: 0 | 1;
}

export interface AdminUserUpdateBody {
  email?: string | null;
  phone?: string | null;
  username?: string | null;
  avatar?: string | null;
  password?: string;
  status?: 0 | 1;
  plan_code?: string;
  plan_expire_at?: number | null;
}

export interface AdminUserAdjustPointsBody {
  action: 'recharge' | 'deduct';
  points: number;
  remark?: string;
}

export interface AdminUserAdjustPointsResp {
  points_before: number;
  points_after: number;
}

export interface AdminGenerationLogItem {
  task_id: string;
  created_at: number;
  user_id: number;
  user_label: string;
  api_key_id?: number;
  key_label?: string;
  kind: 'image' | 'video' | string;
  model_code: string;
  prompt: string;
  status: 0 | 1 | 2 | 3 | 4 | number;
  duration_ms?: number;
  cost_points: number;
  preview_url?: string;
  error?: string;
}

export interface AdminGenerationLogPurgeResp {
  deleted: number;
}

export interface AdminGenerationUpstreamLogItem {
  id: number;
  task_id: string;
  provider: string;
  account_id?: number;
  stage: string;
  method?: string;
  url?: string;
  status_code: number;
  duration_ms: number;
  request_excerpt?: string;
  response_excerpt?: string;
  error?: string;
  meta?: string;
  created_at: number;
}

export interface AdminWalletLogItem {
  id: number;
  created_at: number;
  user_id: number;
  user_label: string;
  direction: 1 | -1 | number;
  biz_type: string;
  biz_id: string;
  points: number;
  points_before: number;
  points_after: number;
  remark?: string;
}

export interface AdminPromoItem {
  id: number;
  code: string;
  name: string;
  discount_type: 1 | 2 | 3 | number;
  discount_val: number;
  min_amount: number;
  apply_to: string;
  total_qty: number;
  used_qty: number;
  per_user_limit: number;
  start_at: number;
  end_at: number;
  status: 0 | 1 | number;
  created_at: number;
  updated_at: number;
}

export interface AdminPromoBody {
  code?: string;
  name?: string;
  discount_type?: 1 | 2 | 3;
  discount_val?: number;
  min_amount?: number;
  apply_to?: string;
  total_qty?: number;
  per_user_limit?: number;
  start_at?: number;
  end_at?: number;
  status?: 0 | 1;
}

export interface DashboardProviderRow {
  provider: string;
  total: number;
  enabled: number;
  available: number;
  broken: number;
  test_ok: number;
  quota_remaining: number;
  quota_total: number;
  quota_used: number;
  success_count: number;
  error_count: number;
}

export interface DashboardRecentTask {
  task_id: string;
  created_at: number;
  user_label: string;
  kind: 'image' | 'video' | string;
  model_code: string;
  count: number;
  status: number;
  cost_points: number;
}

export interface DashboardTrendPoint {
  date: string;
  generated: number;
  cost_points: number;
}

export interface DashboardOverviewResp {
  generated_today: number;
  generated_total: number;
  image_today: number;
  image_total: number;
  video_today: number;
  video_total: number;
  text_tokens_today: number;
  text_tokens_total: number;
  cost_points_today: number;
  cost_points_total: number;
  wallet_spend_today: number;
  wallet_spend_total: number;
  users_total: number;
  users_today: number;
  active_users_today: number;
  success_rate_today: number;
  account_providers: DashboardProviderRow[];
  recent_generations: DashboardRecentTask[];
  trend: DashboardTrendPoint[];
}

export interface AccountItem {
  id: number;
  provider: 'gpt' | 'grok' | string;
  name: string;
  auth_type: 'api_key' | 'cookie' | 'oauth' | string;
  credential_mask: string;
  base_url?: string;
  proxy_id?: number;
  weight: number;
  rpm_limit: number;
  tpm_limit: number;
  daily_quota: number;
  monthly_quota: number;
  /** -1 软删 / 0 禁用 / 1 启用 / 2 熔断 */
  status: -1 | 0 | 1 | 2 | number;
  cooldown_until?: number;
  last_used_at?: number;
  last_error?: string;
  error_count: number;
  success_count: number;
  remark?: string;
  /** OAuth 状态 */
  has_refresh_token?: boolean;
  has_access_token?: boolean;
  access_token_expire_at?: number;
  last_refresh_at?: number;
  /** 最近一次连通性测试 */
  last_test_at?: number;
  /** 0 未测 / 1 OK / 2 FAIL */
  last_test_status?: 0 | 1 | 2 | number;
  last_test_latency_ms?: number;
  last_test_error?: string;
  plan_type?: string;
  default_model?: string;
  image_quota_remaining?: number;
  image_quota_total?: number;
  image_quota_reset_at?: number;
  created_at: number;
  updated_at: number;
}

/** 账号连通性测试结果 */
export interface AccountTestResp {
  ok: boolean;
  latency_ms: number;
  error?: string;
  plan_type?: string;
  default_model?: string;
  image_quota_remaining?: number;
  image_quota_total?: number;
  image_quota_reset_at?: number;
}

/** OAuth 刷新结果 */
export interface AccountRefreshResp {
  ok: boolean;
  expires_in?: number;
  refreshed_at: number;
  has_refresh_token: boolean;
}

/** 批量刷新结果 */
export interface AccountBatchRefreshResp {
  refreshed: number;
  failed_ids: number[];
  page: number;
  page_size: number;
  total: number;
  has_more: boolean;
  next_page?: number;
}

/** 创建账号入参（明文，后端加密）；OAuth 可与 sora2ok 一致拆 AT/RT/ST/client_id。 */
export interface AccountCreateBody {
  provider: 'gpt' | 'grok';
  name: string;
  auth_type: 'api_key' | 'cookie' | 'oauth';
  /** api_key / cookie 必填；oauth 可与 access_token / refresh_token 组合 */
  credential?: string;
  access_token?: string;
  refresh_token?: string;
  session_token?: string;
  client_id?: string;
  base_url?: string;
  /** 绑定代理 ID；0/undefined = 不绑定 */
  proxy_id?: number;
  weight?: number;
  rpm_limit?: number;
  tpm_limit?: number;
  daily_quota?: number;
  monthly_quota?: number;
  remark?: string;
}

/** POST /accounts/batch-delete、/accounts/purge 响应 */
export interface AccountBulkOpResult {
  deleted: number;
}

export interface AccountPurgeBody {
  scope: 'all' | 'invalid' | 'zero_quota';
  provider?: 'gpt' | 'grok';
  confirm?: string;
}

/** 单个账号的明文凭证（管理员编辑面板回显用，解密失败为空串） */
export interface AccountSecretsResp {
  credential?: string;
  access_token?: string;
  refresh_token?: string;
  session_token?: string;
  client_id?: string;
}

export interface AccountUpdateBody {
  name?: string;
  credential?: string;
  /** OAuth 账号专用：单独替换三件套（空字符串表示清空对应列） */
  access_token?: string;
  refresh_token?: string;
  session_token?: string;
  client_id?: string;
  base_url?: string;
  /** 绑定代理 ID；0 = 不绑定 */
  proxy_id?: number;
  weight?: number;
  rpm_limit?: number;
  tpm_limit?: number;
  daily_quota?: number;
  monthly_quota?: number;
  status?: -1 | 0 | 1 | 2;
  remark?: string;
}

/** sub2api / Codex 导出 JSON 中单条账号 */
export interface Sub2APIAccountItem {
  name?: string;
  platform?: string;
  type?: string;
  priority?: number;
  concurrency?: number;
  credentials?: {
    access_token?: string;
    refresh_token?: string;
    client_id?: string;
    id_token?: string;
    email?: string;
    chatgpt_account_id?: string;
    chatgpt_user_id?: string;
    organization_id?: string;
    plan_type?: string;
  };
}

export interface AccountBatchImportBody {
  /** 默认 lines；sub2api 为 JSON 分片导入 */
  format?: 'lines' | 'sub2api';
  provider: 'gpt' | 'grok';
  /** lines 模式必填 */
  auth_type?: 'api_key' | 'cookie' | 'oauth';
  base_url?: string;
  /** 默认绑定代理 ID；0/undefined = 不绑定 */
  proxy_id?: number;
  weight?: number;
  /**
   * lines：一行一条；支持 `<name>@@<credential>` / `<credential>@<base_url>` / `<credential>`。
   */
  text?: string;
  /** sub2api：当前分片的账号列表（建议每批 ≤500） */
  accounts?: Sub2APIAccountItem[];
}

/** POST /accounts/import 响应 */
export interface AccountBatchImportResult {
  imported: number;
  skipped: number;
}

export interface PoolStatsResp {
  pool: Record<string, number>;
}
export interface CDKCreateBatchBody {
  batch_no: string;
  name: string;
  /** 单码价值（后端 *100，传 *100 后的整数） */
  points: number;
  qty: number;
  per_user_limit?: number;
  /** unix 秒；0/不传 = 永不过期 */
  expire_at?: number;
}

export interface CDKCreateBatchResp {
  id: number;
  batch_no: string;
  total_qty: number;
}

// ==================== 代理 ====================

export interface ProxyItem {
  id: number;
  name: string;
  protocol: 'http' | 'https' | 'socks5' | 'socks5h' | string;
  host: string;
  port: number;
  username?: string;
  has_password: boolean;
  /** 0 禁用 / 1 启用 */
  status: 0 | 1 | number;
  last_check_at?: number;
  /** 0 未测 / 1 OK / 2 FAIL */
  last_check_ok: 0 | 1 | 2 | number;
  last_check_ms: number;
  last_error?: string;
  remark?: string;
  created_at: number;
  updated_at: number;
}

export interface ProxyCreateBody {
  name: string;
  protocol: 'http' | 'https' | 'socks5' | 'socks5h';
  host: string;
  port: number;
  username?: string;
  password?: string;
  remark?: string;
}

export interface ProxyUpdateBody {
  name?: string;
  protocol?: 'http' | 'https' | 'socks5' | 'socks5h';
  host?: string;
  port?: number;
  username?: string;
  password?: string;
  status?: 0 | 1;
  remark?: string;
}

export interface ProxyTestResp {
  ok: boolean;
  latency_ms: number;
  error?: string;
}

// ==================== 系统配置 ====================

/** 已知 key（前端只列展示需要的，未列的也允许保存） */
export interface SystemSettings {
  /** 是否启用全局代理 */
  'proxy.global_enabled'?: boolean;
  /** 全局代理 ID（0 表示不启用） */
  'proxy.global_id'?: number;
  /** OAuth access_token 距过期 N 小时内自动刷新 */
  'oauth.refresh_before_hours'?: number;
  /** OpenAI Codex CLI client_id */
  'oauth.openai_client_id'?: string;
  /** OpenAI OAuth Token Endpoint */
  'oauth.openai_token_url'?: string;
  [key: string]: unknown;
}

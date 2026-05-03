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
export interface AccountItem {
  id: number;
  provider: 'gpt' | 'grok' | string;
  name: string;
  auth_type: 'api_key' | 'cookie' | 'oauth' | string;
  credential_mask: string;
  base_url?: string;
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
  created_at: number;
  updated_at: number;
}

/** 创建账号入参（明文 credential，后端加密） */
export interface AccountCreateBody {
  provider: 'gpt' | 'grok';
  name: string;
  auth_type: 'api_key' | 'cookie' | 'oauth';
  credential: string;
  base_url?: string;
  weight?: number;
  rpm_limit?: number;
  tpm_limit?: number;
  daily_quota?: number;
  monthly_quota?: number;
  remark?: string;
}

export interface AccountUpdateBody {
  name?: string;
  credential?: string;
  base_url?: string;
  weight?: number;
  rpm_limit?: number;
  tpm_limit?: number;
  daily_quota?: number;
  monthly_quota?: number;
  status?: -1 | 0 | 1 | 2;
  remark?: string;
}

export interface AccountBatchImportBody {
  provider: 'gpt' | 'grok';
  auth_type: 'api_key' | 'cookie' | 'oauth';
  base_url?: string;
  weight?: number;
  /** 一行一条；支持 `<name>@@<credential>` / `<credential>@<base_url>` / `<credential>` */
  text: string;
}

export interface PoolStatsResp {
  pool: Record<string, number>;
}

/** CDK 批次创建（后端：POST /admin/api/v1/cdk/batches） */
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

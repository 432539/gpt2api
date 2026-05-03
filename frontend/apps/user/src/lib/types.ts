// 与后端 dto / response 对齐的前端类型。
// 注意：所有 *_points / points / cost_points 字段单位为「点 *100」，展示时使用 fmtPoints 除以 100。

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

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  token_type: string;
  access_expire_in: number;
  refresh_expire_in: number;
}

export interface RegisterResp {
  uid: number;
  uuid: string;
  invite_code: string;
  token: TokenPair;
}

export interface LoginResp {
  uid: number;
  uuid: string;
  token: TokenPair;
}

export interface MeResp {
  uid: number;
  uuid: string;
  username?: string;
  email?: string;
  phone?: string;
  avatar?: string;
  points: number;
  frozen_points: number;
  plan_code: string;
  invite_code: string;
  created_at: number;
}

export interface APIKey {
  id: number;
  name: string;
  prefix: string;
  last4: string;
  mask: string;
  scope: string;
  rpm_limit: number;
  daily_quota: number;
  status: number;
  expire_at?: number;
  last_used_at?: number;
  created_at: number;
}

export interface APIKeyCreated {
  id: number;
  name: string;
  plain: string;
  prefix: string;
  last4: string;
  scope: string;
  created_at: number;
}

export interface APIKeyCreateBody {
  name: string;
  scope?: string;
  rpm_limit?: number;
  daily_quota?: number;
  expire_days?: number;
}

export interface WalletLog {
  id: number;
  direction: 1 | -1;
  biz_type: string;
  biz_id: string;
  points: number;
  points_before: number;
  points_after: number;
  remark?: string;
  created_at: number;
}

export interface GenerationResult {
  url: string;
  thumb_url?: string;
  width?: number;
  height?: number;
  duration_ms?: number;
}

/**
 * 任务状态：
 * 0 pending / 1 running / 2 succeeded / 3 failed / 4 refunded / 5 cancelled
 */
export type TaskStatus = 0 | 1 | 2 | 3 | 4 | 5;

export interface GenerationTask {
  task_id: string;
  kind: 'image' | 'video';
  status: TaskStatus;
  progress: number;
  model: string;
  cost_points: number;
  error?: string;
  results?: GenerationResult[];
  created_at: number;
}

export interface CreateImageBody {
  model: string;
  prompt: string;
  neg_prompt?: string;
  mode?: 't2i' | 'i2i';
  count?: number;
  ratio?: string;
  quality?: 'draft' | 'standard' | 'hd';
  ref_assets?: string[];
  params?: Record<string, unknown>;
}

export interface CreateVideoBody {
  model: string;
  prompt: string;
  mode?: 't2v' | 'i2v';
  duration?: number;
  ratio?: string;
  quality?: 'draft' | 'standard' | 'hd';
  ref_assets?: string[];
  params?: Record<string, unknown>;
}

export interface RedeemCDKResp {
  points: number;
  biz: string;
  message: string;
}

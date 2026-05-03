// 后台 API 抽象。
import { request } from './api';
import type {
  AccountBatchImportBody,
  AccountBatchImportResult,
  AccountBatchRefreshResp,
  AccountBulkOpResult,
  AccountCreateBody,
  AccountItem,
  AccountPurgeBody,
  AccountRefreshResp,
  AccountSecretsResp,
  AccountTestResp,
  AccountUpdateBody,
  AdminUserAdjustPointsBody,
  AdminUserAdjustPointsResp,
  AdminUserCreateBody,
  AdminGenerationLogItem,
  AdminGenerationLogPurgeResp,
  AdminGenerationUpstreamLogItem,
  AdminPromoBody,
  AdminPromoItem,
  AdminUserItem,
  AdminUserUpdateBody,
  AdminWalletLogItem,
  AdminLoginResp,
  AdminMe,
  CDKCreateBatchBody,
  CDKCreateBatchResp,
  DashboardOverviewResp,
  PageData,
  PoolStatsResp,
  ProxyCreateBody,
  ProxyItem,
  ProxyTestResp,
  ProxyUpdateBody,
  SystemSettings,
} from './types';

export const authApi = {
  login: (username: string, password: string) =>
    request<AdminLoginResp>({
      url: '/auth/login',
      method: 'POST',
      // 后端 dto.LoginReq 字段名为 account，前端表单仍展示「管理员账号」
      data: { account: username, password },
    }),
  me: () => request<AdminMe>({ url: '/auth/me', method: 'GET' }),
  changePassword: (body: { old_password: string; new_password: string }) =>
    request<{ ok: boolean }>({ url: '/auth/password', method: 'POST', data: body }),
};

export const dashboardApi = {
  overview: () => request<DashboardOverviewResp>({ url: '/dashboard/overview', method: 'GET' }),
};

export interface AdminUserListQuery {
  keyword?: string;
  status?: 0 | 1;
  page?: number;
  page_size?: number;
}

export const usersApi = {
  list: (q: AdminUserListQuery = {}) =>
    request<PageData<AdminUserItem>>({ url: '/users', method: 'GET', params: q }),
  create: (body: AdminUserCreateBody) =>
    request<{ id: number }>({ url: '/users', method: 'POST', data: body }),
  update: (id: number, body: AdminUserUpdateBody) =>
    request<void>({ url: `/users/${id}`, method: 'PUT', data: body }),
  adjustPoints: (id: number, body: AdminUserAdjustPointsBody) =>
    request<AdminUserAdjustPointsResp>({ url: `/users/${id}/points`, method: 'POST', data: body }),
};

export interface GenerationLogListQuery {
  keyword?: string;
  kind?: 'image' | 'video' | 'chat' | 'text';
  status?: 0 | 1 | 2 | 3 | 4;
  page?: number;
  page_size?: number;
}

export const logsApi = {
  generations: (q: GenerationLogListQuery = {}) =>
    request<PageData<AdminGenerationLogItem>>({ url: '/logs/generations', method: 'GET', params: q }),
  generationUpstream: (taskId: string) =>
    request<AdminGenerationUpstreamLogItem[]>({ url: `/logs/generations/${taskId}/upstream`, method: 'GET' }),
  purgeGenerations: (days: number) =>
    request<AdminGenerationLogPurgeResp>({ url: '/logs/generations', method: 'DELETE', data: { days } }),
};

export interface WalletLogListQuery {
  keyword?: string;
  user_id?: number;
  biz_type?: string;
  direction?: 1 | -1 | '';
  page?: number;
  page_size?: number;
}

export const billingApi = {
  walletLogs: (q: WalletLogListQuery = {}) =>
    request<PageData<AdminWalletLogItem>>({ url: '/billing/wallet-logs', method: 'GET', params: q }),
};

export interface PromoListQuery {
  keyword?: string;
  status?: 0 | 1 | '';
  discount_type?: 1 | 2 | 3 | '';
  page?: number;
  page_size?: number;
}

export const promoApi = {
  list: (q: PromoListQuery = {}) =>
    request<PageData<AdminPromoItem>>({ url: '/promo/codes', method: 'GET', params: q }),
  create: (body: AdminPromoBody) =>
    request<{ id: number }>({ url: '/promo/codes', method: 'POST', data: body }),
  update: (id: number, body: AdminPromoBody) =>
    request<void>({ url: `/promo/codes/${id}`, method: 'PUT', data: body }),
  remove: (id: number) => request<void>({ url: `/promo/codes/${id}`, method: 'DELETE' }),
};

export interface AccountListQuery {
  provider?: 'gpt' | 'grok';
  status?: -1 | 0 | 1 | 2;
  keyword?: string;
  page?: number;
  page_size?: number;
}

export const accountsApi = {
  list: (q: AccountListQuery = {}) =>
    request<PageData<AccountItem>>({
      url: '/accounts',
      method: 'GET',
      params: q,
    }),
  create: (body: AccountCreateBody) =>
    request<{ id: number }>({ url: '/accounts', method: 'POST', data: body }),
  update: (id: number, body: AccountUpdateBody) =>
    request<void>({ url: `/accounts/${id}`, method: 'PUT', data: body }),
  remove: (id: number) => request<void>({ url: `/accounts/${id}`, method: 'DELETE' }),
  batchImport: (body: AccountBatchImportBody) =>
    request<AccountBatchImportResult>({
      url: '/accounts/import',
      method: 'POST',
      data: body,
    }),
  stats: () => request<PoolStatsResp>({ url: '/accounts/stats', method: 'GET' }),
  test: (id: number) =>
    request<AccountTestResp>({ url: `/accounts/${id}/test`, method: 'POST' }),
  refresh: (id: number) =>
    request<AccountRefreshResp>({ url: `/accounts/${id}/refresh`, method: 'POST' }),
  secrets: (id: number) =>
    request<AccountSecretsResp>({ url: `/accounts/${id}/secrets`, method: 'GET' }),
  batchRefresh: (provider?: 'gpt' | 'grok' | '', page = 1, pageSize = 50) =>
    request<AccountBatchRefreshResp>({
      url: '/accounts/batch-refresh',
      method: 'POST',
      data: { provider: provider ?? '', page, page_size: pageSize },
    }),
  batchProbe: (provider?: 'gpt' | 'grok' | '', page = 1, pageSize = 20) =>
    request<{
      probed: number;
      failed_ids: number[];
      page: number;
      page_size: number;
      total: number;
      has_more: boolean;
      next_page?: number;
    }>({
      url: '/accounts/batch-probe',
      method: 'POST',
      data: { provider: provider ?? '', page, page_size: pageSize },
    }),
  batchDelete: (ids: number[]) =>
    request<AccountBulkOpResult>({
      url: '/accounts/batch-delete',
      method: 'POST',
      data: { ids },
    }),
  purge: (body: AccountPurgeBody) =>
    request<AccountBulkOpResult>({
      url: '/accounts/purge',
      method: 'POST',
      data: body,
    }),
};

export const cdkApi = {
  createBatch: (body: CDKCreateBatchBody) =>
    request<CDKCreateBatchResp>({
      url: '/cdk/batches',
      method: 'POST',
      data: body,
    }),
};

// ==================== 代理 ====================

export interface ProxyListQuery {
  status?: 0 | 1;
  keyword?: string;
  page?: number;
  page_size?: number;
}

export const proxiesApi = {
  list: (q: ProxyListQuery = {}) =>
    request<PageData<ProxyItem>>({ url: '/proxies', method: 'GET', params: q }),
  create: (body: ProxyCreateBody) =>
    request<{ id: number }>({ url: '/proxies', method: 'POST', data: body }),
  update: (id: number, body: ProxyUpdateBody) =>
    request<void>({ url: `/proxies/${id}`, method: 'PUT', data: body }),
  remove: (id: number) =>
    request<void>({ url: `/proxies/${id}`, method: 'DELETE' }),
  test: (id: number) =>
    request<ProxyTestResp>({ url: `/proxies/${id}/test`, method: 'POST' }),
};

// ==================== 系统配置 ====================

export const systemApi = {
  get: () => request<SystemSettings>({ url: '/system/settings', method: 'GET' }),
  update: (kv: Partial<SystemSettings>) =>
    request<{ updated: number }>({
      url: '/system/settings',
      method: 'PUT',
      data: kv,
    }),
  cacheStats: () =>
    request<{ root: string; files: number; bytes: number }>({ url: '/system/cache', method: 'GET' }),
  cleanCache: (body: { days?: number; all?: boolean }) =>
    request<{ deleted_files: number; deleted_bytes: number; remain_bytes: number }>({
      url: '/system/cache',
      method: 'DELETE',
      data: body,
    }),
};

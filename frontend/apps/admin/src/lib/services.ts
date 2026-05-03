// 后台 API 抽象。
import { request } from './api';
import type {
  AccountBatchImportBody,
  AccountCreateBody,
  AccountItem,
  AccountUpdateBody,
  AdminLoginResp,
  AdminMe,
  CDKCreateBatchBody,
  CDKCreateBatchResp,
  PageData,
  PoolStatsResp,
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
    request<{ imported: number }>({
      url: '/accounts/import',
      method: 'POST',
      data: body,
    }),
  stats: () => request<PoolStatsResp>({ url: '/accounts/stats', method: 'GET' }),
};

export const cdkApi = {
  createBatch: (body: CDKCreateBatchBody) =>
    request<CDKCreateBatchResp>({
      url: '/cdk/batches',
      method: 'POST',
      data: body,
    }),
};

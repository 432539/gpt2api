// 高层 API 服务：分领域封装，UI 层只看 services.* 不直接 import axios。
import { request } from './api';
import type {
  APIKey,
  APIKeyCreateBody,
  APIKeyCreated,
  CreateImageBody,
  CreateVideoBody,
  GenerationTask,
  LoginResp,
  MeResp,
  PageData,
  RedeemCDKResp,
  RegisterResp,
  TokenPair,
  WalletLog,
} from './types';

export const authApi = {
  register: (body: { account: string; password: string; invite_code?: string }) =>
    request<RegisterResp>({ method: 'POST', url: '/auth/register', data: body }),
  login: (body: { account: string; password: string }) =>
    request<LoginResp>({ method: 'POST', url: '/auth/login', data: body }),
  refresh: (refresh_token: string) =>
    request<TokenPair>({ method: 'POST', url: '/auth/refresh', data: { refresh_token } }),
  logout: () => request<null>({ method: 'POST', url: '/auth/logout' }),
  me: () => request<MeResp>({ method: 'GET', url: '/users/me' }),
  changePassword: (body: { old_password: string; new_password: string }) =>
    request<null>({ method: 'POST', url: '/users/password', data: body }),
};

export const keysApi = {
  list: async () => {
    const r = await request<{ list: APIKey[] } | APIKey[] | null>({ method: 'GET', url: '/keys' });
    if (Array.isArray(r)) return r;
    return r?.list ?? [];
  },
  create: (body: APIKeyCreateBody) =>
    request<APIKeyCreated>({ method: 'POST', url: '/keys', data: body }),
  toggle: ({ id, enable }: { id: number; enable: boolean }) =>
    request<null>({
      method: 'POST',
      url: `/keys/${id}/toggle`,
      params: { enable: enable ? 1 : 0 },
    }),
  remove: (id: number) => request<null>({ method: 'DELETE', url: `/keys/${id}` }),
};

export const billingApi = {
  logs: (page = 1, pageSize = 20) =>
    request<PageData<WalletLog>>({
      method: 'GET',
      url: '/billing/logs',
      params: { page, page_size: pageSize },
    }),
  redeemCDK: (code: string) =>
    request<RedeemCDKResp>({ method: 'POST', url: '/billing/cdk/redeem', data: { code } }),
};

export const genApi = {
  createImage: (body: CreateImageBody, idemKey?: string) =>
    request<GenerationTask>({
      method: 'POST',
      url: '/gen/image',
      data: body,
      headers: idemKey ? { 'Idempotency-Key': idemKey } : undefined,
    }),
  createVideo: (body: CreateVideoBody, idemKey?: string) =>
    request<GenerationTask>({
      method: 'POST',
      url: '/gen/video',
      data: body,
      headers: idemKey ? { 'Idempotency-Key': idemKey } : undefined,
    }),
  getTask: (taskId: string) =>
    request<GenerationTask>({ method: 'GET', url: `/gen/tasks/${taskId}` }),
  history: (params: { kind?: 'image' | 'video'; page?: number; page_size?: number } = {}) =>
    request<PageData<GenerationTask>>({
      method: 'GET',
      url: '/gen/history',
      params: {
        kind: params.kind,
        page: params.page ?? 1,
        page_size: params.page_size ?? 20,
      },
    }),
};

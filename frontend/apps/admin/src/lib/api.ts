// 后台管理 axios 客户端。与用户端独立：
//   - baseURL: /admin/api/v1
//   - token 存 localStorage(key: klein:admin:token)
//   - 401 → 清 token，跳转 /login
import axios, {
  AxiosError,
  type AxiosInstance,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig,
} from 'axios';

import type { ApiBody, AdminLoginResp } from './types';

const TOKEN_KEY = 'klein:admin:token';

export interface StoredToken {
  access: string;
  refresh: string;
  type: string;
  accessExpireAt: number;
  refreshExpireAt: number;
}

export function loadToken(): StoredToken | null {
  try {
    const raw = localStorage.getItem(TOKEN_KEY);
    if (!raw) return null;
    return JSON.parse(raw) as StoredToken;
  } catch {
    return null;
  }
}

export function saveToken(tok: AdminLoginResp['token']): StoredToken {
  const now = Date.now();
  const v: StoredToken = {
    access: tok.access_token,
    refresh: tok.refresh_token,
    type: tok.token_type || 'Bearer',
    accessExpireAt: now + tok.access_expire_in * 1000,
    refreshExpireAt: now + tok.refresh_expire_in * 1000,
  };
  localStorage.setItem(TOKEN_KEY, JSON.stringify(v));
  return v;
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  code: number;
  httpStatus?: number;
  traceId?: string;
  constructor(msg: string, code: number, opts?: { httpStatus?: number; traceId?: string }) {
    super(msg);
    this.code = code;
    this.httpStatus = opts?.httpStatus;
    this.traceId = opts?.traceId;
  }
}

const baseURL =
  (import.meta.env.VITE_ADMIN_BASE_URL as string | undefined)?.replace(/\/+$/, '') ??
  '/admin/api/v1';

export const api: AxiosInstance = axios.create({
  baseURL,
  timeout: 30_000,
  headers: { Accept: 'application/json' },
});

api.interceptors.request.use((cfg: InternalAxiosRequestConfig) => {
  const tok = loadToken();
  if (tok && cfg.headers) {
    cfg.headers.set?.('Authorization', `${tok.type} ${tok.access}`);
  }
  return cfg;
});

let unauthorizedHandler: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void) {
  unauthorizedHandler = fn;
}

api.interceptors.response.use(
  (res) => {
    const body = res.data as ApiBody<unknown>;
    if (body && typeof body === 'object' && 'code' in body && body.code !== 0) {
      throw new ApiError(body.msg || '请求失败', body.code, { traceId: body.trace_id });
    }
    return res;
  },
  (err: AxiosError<ApiBody<unknown>>) => {
    const status = err.response?.status;
    const body = err.response?.data;
    const msg = body?.msg ?? err.message ?? '网络异常';
    const code = body?.code ?? status ?? -1;
    if (status === 401) {
      clearToken();
      unauthorizedHandler?.();
    }
    return Promise.reject(
      new ApiError(msg, code, { httpStatus: status, traceId: body?.trace_id }),
    );
  },
);

/** 统一请求并解构 data，抹平 axios 返回结构 */
export async function request<T = unknown>(cfg: AxiosRequestConfig): Promise<T> {
  const res = await api.request<ApiBody<T>>(cfg);
  return (res.data?.data ?? (undefined as unknown)) as T;
}

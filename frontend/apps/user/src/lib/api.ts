// 用户端 axios 客户端：统一 baseURL、注入 Authorization、处理 401 / 业务错误码。
//
// 业务约定：
//   1) 后端响应统一为 `{code, msg, data}`；HTTP 200 但 code !== 0 视为失败。
//   2) 401 时清空 token 并跳转登录。
//   3) 失败时抛出 `ApiError`，UI 用 toast 显示。
import axios, {
  AxiosError,
  type AxiosInstance,
  type AxiosRequestConfig,
  type InternalAxiosRequestConfig,
} from 'axios';

import type { ApiBody, TokenPair } from './types';

const TOKEN_KEY = 'klein:token';

export interface StoredToken {
  access: string;
  refresh: string;
  type: string;
  accessExpireAt: number; // 毫秒时间戳
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

export function saveToken(tok: TokenPair): StoredToken {
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
  (import.meta.env.VITE_API_BASE_URL as string | undefined)?.replace(/\/+$/, '') ?? '/api/v1';

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
    const msg = body?.msg ?? friendlyHttpError(status, err.message);
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

function friendlyHttpError(status?: number, raw?: string) {
  if (status === 502 || status === 503 || status === 504) return '服务正在更新或繁忙，请稍后重试';
  if (status === 413) return '上传内容过大，请压缩后重试';
  if (status === 429) return '操作过于频繁，请稍后重试';
  if (!status && raw === 'Network Error') return '网络连接异常，请检查网络后重试';
  return raw || '网络异常';
}

/** 统一请求并解构 data，抹平 axios 返回结构 */
export async function request<T = unknown>(cfg: AxiosRequestConfig): Promise<T> {
  const res = await api.request<ApiBody<T>>(cfg);
  return (res.data?.data ?? (undefined as unknown)) as T;
}

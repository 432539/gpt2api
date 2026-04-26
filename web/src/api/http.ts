import axios, { AxiosError, type AxiosInstance, type AxiosRequestConfig } from 'axios'
import { ElMessage } from 'element-plus'

/**
 * 统一响应结构,后端 `pkg/resp` 约定:
 * {
 *   code: 0,
 *   message: "ok",
 *   data: <T>
 * }
 * 非 0 code 认为是业务错误,会被拦截器统一抛出。
 */
export interface ApiEnvelope<T = any> {
  code: number
  message: string
  data: T
}

const baseURL = import.meta.env.VITE_API_BASE || ''

export const http: AxiosInstance = axios.create({
  baseURL,
  timeout: 30_000,
})

/** access token 持久化 key(Pinia store 也复用) */
export const TOKEN_KEY = 'gpt2api.access'
export const REFRESH_KEY = 'gpt2api.refresh'

http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) {
    config.headers = config.headers || {}
    config.headers['Authorization'] = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (response) => {
    // 下载类接口直接透传 blob
    const contentTypeValue = response.headers?.['content-type']
    const contentType = Array.isArray(contentTypeValue)
      ? contentTypeValue.join(';')
      : String(contentTypeValue || '')
    if (response.config.responseType === 'blob' || contentType.startsWith('application/gzip')) {
      return response
    }
    const payload = response.data as ApiEnvelope
    if (payload && typeof payload === 'object' && 'code' in payload) {
      if (payload.code === 0) {
        return payload.data as any
      }
      const msg = payload.message || `请求失败 (code=${payload.code})`
      ElMessage.error(msg)
      return Promise.reject(new Error(msg))
    }
    return response.data
  },
  (error: AxiosError<ApiEnvelope>) => {
    const status = error.response?.status
    const msg = error.response?.data?.message || error.message || '网络错误'
    if (status === 401) {
      // 登录接口 401 = 账号密码错误,不要清 token 也不要跳转,直接给明确提示。
      // 后端返回的是英文 "invalid email or password",这里本地化为中文。
      const reqUrl = (error.config?.url || '') as string
      const isLoginEndpoint =
        reqUrl.includes('/auth/login') || reqUrl.includes('/auth/register')
      if (isLoginEndpoint) {
        const friendly =
          /invalid email or password/i.test(msg) ? '邮箱或密码错误' : msg || '登录失败'
        ElMessage.error(friendly)
      } else {
        localStorage.removeItem(TOKEN_KEY)
        localStorage.removeItem(REFRESH_KEY)
        if (!window.location.pathname.startsWith('/login')) {
          ElMessage.warning('登录已失效,请重新登录')
          window.location.href = `/login?redirect=${encodeURIComponent(window.location.pathname)}`
        } else {
          // 极端情况:已经在 /login 但又收到 401(例如 me 接口),给一条兜底 toast,
          // 避免用户看到"点了没反应"。
          ElMessage.error(msg || '登录已失效')
        }
      }
    } else if (status === 403) {
      ElMessage.error(`无权限:${msg}`)
    } else {
      ElMessage.error(msg)
    }
    return Promise.reject(error)
  },
)

/** 直接传入返回体的辅助类型工具 */
export function request<T = any>(cfg: AxiosRequestConfig): Promise<T> {
  return http.request(cfg) as unknown as Promise<T>
}

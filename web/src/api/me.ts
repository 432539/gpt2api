import { http } from './http'

// ---------- models (enabled, for 面板下拉) ----------

export interface SimpleModel {
  id: number
  slug: string
  type: 'chat' | 'image' | string
  description: string
}

export function listMyModels(): Promise<{ items: SimpleModel[]; total: number }> {
  return http.get('/api/me/models')
}

// ===============================================================
// 当前用户视角的用量 + 图片任务(供「生成面板」使用)
// ===============================================================

// ---------- usage ----------

export interface UsageItem {
  id: number
  user_id: number
  key_id: number
  model_id: number
  model_slug: string
  account_id: number
  request_id: string
  type: 'chat' | 'image' | string
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  image_count: number
  credit_cost: number
  duration_ms: number
  status: string
  error_code: string
  ip: string
  created_at: string
}

export interface UsageOverall {
  requests: number
  failures: number
  chat_requests: number
  image_images: number
  input_tokens: number
  output_tokens: number
  credit_cost: number
}

export interface UsageDaily {
  day: string
  requests: number
  failures: number
  input_tokens: number
  output_tokens: number
  image_count: number
  credit_cost: number
}

export interface UsageModelStat {
  model_id: number
  model_slug: string
  type: string
  requests: number
  failures: number
  input_tokens: number
  output_tokens: number
  image_count: number
  credit_cost: number
  avg_dur_ms: number
}

export interface MyStatsResp {
  overall: UsageOverall
  daily: UsageDaily[]
  by_model: UsageModelStat[]
}

export function listMyUsageLogs(params: {
  type?: 'chat' | 'image' | ''
  status?: string
  since?: string
  until?: string
  limit?: number
  offset?: number
} = {}): Promise<{ items: UsageItem[]; total: number; limit: number; offset: number }> {
  return http.get('/api/me/usage/logs', { params })
}

export function getMyUsageStats(params: {
  days?: number
  top_n?: number
  type?: 'chat' | 'image' | ''
  since?: string
  until?: string
} = {}): Promise<MyStatsResp> {
  return http.get('/api/me/usage/stats', { params })
}

// ---------- credit logs (个人积分流水) ----------

export interface MyCreditLog {
  id: number
  user_id: number
  key_id: number
  type: string
  amount: number
  balance_after: number
  ref_id: string
  remark: string
  created_at: string
}

export function listMyCreditLogs(params: {
  limit?: number
  offset?: number
} = {}): Promise<{ items: MyCreditLog[]; total: number; limit: number; offset: number }> {
  return http.get('/api/me/credit-logs', { params })
}

// ---------- image tasks ----------

export interface ImageTask {
  id: number
  task_id: string
  user_id: number
  model_id: number
  account_id: number
  prompt: string
  n: number
  size: string
  status: 'queued' | 'dispatched' | 'running' | 'success' | 'failed' | string
  conversation_id?: string
  error?: string
  credit_cost: number
  image_urls: string[]
  file_ids?: string[]
  created_at: string
  started_at?: string | null
  finished_at?: string | null
}

export function listMyImageTasks(params: {
  limit?: number
  offset?: number
  status?: string
  keyword?: string
  start_at?: string
  end_at?: string
} = {}): Promise<{ items: ImageTask[]; limit: number; offset: number }> {
  return http.get('/api/me/images/tasks', { params })
}

export function getMyImageTask(taskID: string): Promise<ImageTask> {
  return http.get(`/api/me/images/tasks/${taskID}`)
}

// ===============================================================
// 在线体验(Playground)—— JWT 鉴权,内部代挂 __playground__ key
// ===============================================================

// 后端返回的 OpenAI 兼容 chat chunk(stream=true 时每行 data: {...})
export interface ChatStreamDelta {
  role?: string
  content?: string
}

export interface ChatStreamChunk {
  id?: string
  model?: string
  choices?: Array<{
    index?: number
    delta?: ChatStreamDelta
    finish_reason?: string | null
  }>
}

export interface PlayChatMessage {
  role: 'system' | 'user' | 'assistant'
  content: string
}

// streamPlayChat 直接用 fetch 读 SSE,因为 axios 不擅长流式。
// 回调 onDelta 每接到一段 content 增量触发;流结束时 promise resolve。
export async function streamPlayChat(
  req: {
    model: string
    messages: PlayChatMessage[]
    temperature?: number
  },
  onDelta: (text: string) => void,
  signal?: AbortSignal,
): Promise<void> {
  const token = localStorage.getItem('gpt2api.access') || ''
  const resp = await fetch('/api/me/playground/chat', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ ...req, stream: true }),
    signal,
  })
  if (!resp.ok || !resp.body) {
    const text = await resp.text().catch(() => '')
    throw new Error(`chat ${resp.status}: ${text || resp.statusText}`)
  }
  const reader = resp.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    // SSE 按 "\n\n" 分块,每块内的 "data:" 是 JSON 或 "[DONE]"
    const blocks = buffer.split('\n\n')
    buffer = blocks.pop() || ''
    for (const block of blocks) {
      for (const line of block.split('\n')) {
        if (!line.startsWith('data:')) continue
        const data = line.slice(5).trim()
        if (!data || data === '[DONE]') continue
        try {
          const chunk: ChatStreamChunk = JSON.parse(data)
          const delta = chunk.choices?.[0]?.delta?.content
          if (delta) onDelta(delta)
        } catch {
          // 忽略解析失败的心跳
        }
      }
    }
  }
}

// 文生图 / 图生图(同步)。图生图走 reference_images 字段:
//   - /playground/image      : 走 JSON,reference_images 是 data:URL 数组
//   - /playground/image-edit : 走 multipart/form-data,直接传 File 对象
export interface PlayImageRequest {
  model: string
  prompt: string
  n?: number
  size?: string
  reference_images?: string[] // base64 data:image/png;base64,... 支持多张参考图
  // 本地 Catmull-Rom 高清放大档位:"" 原图 / "2k" 长边 2560 / "4k" 长边 3840。
  // 服务端仅保存标记,放大在图片代理 URL 首次被请求时做,PNG 输出并进程内缓存。
  upscale?: '' | '2k' | '4k'
}

export interface PlayImageData {
  url: string
  file_id?: string
  revised_prompt?: string
}

export interface PlayImageResponse {
  created: number
  task_id?: string
  data: PlayImageData[]
}

// 注意:返回是"裸 OpenAI 结构",不走我们的 ApiEnvelope,所以用 fetch。
export async function playGenerateImage(
  req: PlayImageRequest,
  signal?: AbortSignal,
): Promise<PlayImageResponse> {
  const token = localStorage.getItem('gpt2api.access') || ''
  const resp = await fetch('/api/me/playground/image', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(req),
    signal,
  })
  if (!resp.ok) {
    let detail = ''
    try {
      const body = await resp.json()
      detail = body?.error?.message || body?.message || ''
    } catch {
      /* ignore */
    }
    throw new Error(detail || `image ${resp.status}: ${resp.statusText}`)
  }
  return (await resp.json()) as PlayImageResponse
}

// playEditImage 走 /playground/image-edit,multipart/form-data,严格对齐 /v1/images/edits 规范。
// files 数组是 File 对象(来自 <input type="file"> 或拖拽),至少 1 张,最多 4 张。
export async function playEditImage(
  model: string,
  prompt: string,
  files: File[],
  opts?: { n?: number; size?: string; upscale?: '' | '2k' | '4k'; signal?: AbortSignal },
): Promise<PlayImageResponse> {
  if (!files.length) throw new Error('至少需要选择一张参考图')
  const token = localStorage.getItem('gpt2api.access') || ''
  const fd = new FormData()
  fd.append('model', model)
  fd.append('prompt', prompt)
  if (opts?.n) fd.append('n', String(opts.n))
  if (opts?.size) fd.append('size', opts.size)
  if (opts?.upscale) fd.append('upscale', opts.upscale)
  files.forEach((f, idx) => {
    // OpenAI 规范:第一张用 `image`,后续用 `image[]`;服务端两个字段都认。
    fd.append(idx === 0 ? 'image' : 'image[]', f, f.name)
  })
  const resp = await fetch('/api/me/playground/image-edit', {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
    },
    body: fd,
    signal: opts?.signal,
  })
  if (!resp.ok) {
    let detail = ''
    try {
      const body = await resp.json()
      detail = body?.error?.message || body?.message || ''
    } catch {
      /* ignore */
    }
    throw new Error(detail || `image-edit ${resp.status}: ${resp.statusText}`)
  }
  return (await resp.json()) as PlayImageResponse
}

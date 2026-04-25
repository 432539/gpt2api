<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  listMyModels,
  listMyUsageLogs,
  listMyImageTasks,
  getMyUsageStats,
  type SimpleModel,
  type UsageItem,
  type ImageTask,
  type MyStatsResp,
} from '@/api/me'
import { formatCredit, formatDateTime, formatErrorCode } from '@/utils/format'
import { ENABLE_CHAT_MODEL } from '@/config/feature'

const activeTab = ref<'chat' | 'image'>(ENABLE_CHAT_MODEL ? 'chat' : 'image')

const models = ref<SimpleModel[]>([])
const chatModels = computed(() => models.value.filter((m) => m.type === 'chat'))
const imageModels = computed(() => models.value.filter((m) => m.type === 'image'))

const selectedChatModel = ref<string>('')
const selectedImageModel = ref<string>('')

// 原点:浏览器当前地址,用于 SDK 示例的 base_url
const origin = computed(() => window.location.origin)

// ---------- 当前用户汇总 ----------
const stats = ref<MyStatsResp | null>(null)
const statsLoading = ref(false)

async function loadStats() {
  statsLoading.value = true
  try {
    stats.value = await getMyUsageStats({ days: 14, top_n: 5 })
  } finally {
    statsLoading.value = false
  }
}

// ---------- 文字历史(chat) ----------
const chatLogs = ref<UsageItem[]>([])
const chatPage = ref({ limit: 20, offset: 0, total: 0 })
const chatLoading = ref(false)

async function loadChatLogs() {
  chatLoading.value = true
  try {
    const data = await listMyUsageLogs({
      type: 'chat',
      limit: chatPage.value.limit,
      offset: chatPage.value.offset,
    })
    chatLogs.value = data.items
    chatPage.value.total = data.total
  } finally {
    chatLoading.value = false
  }
}

function chatPageChange(p: number) {
  chatPage.value.offset = (p - 1) * chatPage.value.limit
  loadChatLogs()
}

// ---------- 图片历史 ----------
const imageTasks = ref<ImageTask[]>([])
const imagePage = ref({ limit: 12, offset: 0 })
const imageLoading = ref(false)
const hasMoreImage = ref(false)
const imageFilters = ref({
  status: '',
  keyword: '',
  range: [] as string[],
})
const previewDialogVisible = ref(false)
const previewImageURL = ref('')
const previewImageTitle = ref('')

async function loadImageTasks(reset = true) {
  imageLoading.value = true
  try {
    if (reset) {
      imagePage.value.offset = 0
      imageTasks.value = []
    }
    const data = await listMyImageTasks({
      limit: imagePage.value.limit,
      offset: imagePage.value.offset,
      status: imageFilters.value.status || undefined,
      keyword: imageFilters.value.keyword.trim() || undefined,
      start_at: imageFilters.value.range[0] || undefined,
      end_at: imageFilters.value.range[1] || undefined,
    })
    if (reset) imageTasks.value = data.items
    else imageTasks.value.push(...data.items)
    hasMoreImage.value = data.items.length >= imagePage.value.limit
  } finally {
    imageLoading.value = false
  }
}

function imageLoadMore() {
  imagePage.value.offset += imagePage.value.limit
  loadImageTasks(false)
}

function resetImageFilters() {
  imageFilters.value.status = ''
  imageFilters.value.keyword = ''
  imageFilters.value.range = []
  loadImageTasks(true)
}

// ---------- SDK 代码示例 ----------
const chatCurl = computed(() => {
  const model = selectedChatModel.value || 'gpt-5'
  return `curl ${origin.value}/v1/chat/completions \\
  -H "Authorization: Bearer \${YOUR_API_KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "stream": true,
    "messages": [
      {"role": "user", "content": "你好,介绍一下你自己"}
    ]
  }'`
})

const chatPython = computed(() => {
  const model = selectedChatModel.value || 'gpt-5'
  return `from openai import OpenAI

client = OpenAI(
    base_url="${origin.value}/v1",
    api_key="\${YOUR_API_KEY}",
)

resp = client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "你好"}],
    stream=True,
)
for chunk in resp:
    print(chunk.choices[0].delta.content or "", end="")`
})

const imageCurl = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `curl ${origin.value}/v1/images/generations \\
  -H "Authorization: Bearer \${YOUR_API_KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "prompt": "A cute orange cat playing with yarn, studio ghibli style",
    "n": 1,
    "size": "1024x1024"
  }'`
})

const imageCurlWithRef = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `curl ${origin.value}/v1/images/generations \\
  -H "Authorization: Bearer \${YOUR_API_KEY}" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${model}",
    "prompt": "根据参考图生成一个类似风格的图片",
    "n": 1,
    "size": "1024x1024",
    "reference_images": [
      "https://example.com/your-image.jpg"
    ]
  }'

# 或使用 base64 编码的图片:
# "reference_images": ["data:image/png;base64,iVBORw0KG..."]
# 或纯 base64: ["iVBORw0KG..."]
# 最多支持 4 张参考图,单张最大 20MB`
})

const imagePython = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `from openai import OpenAI

client = OpenAI(
    base_url="${origin.value}/v1",
    api_key="\${YOUR_API_KEY}",
)

resp = client.images.generate(
    model="${model}",
    prompt="A cute orange cat playing with yarn",
    n=1,
    size="1024x1024",
)
print(resp.data[0].url)`
})

const imagePythonRequests = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `import requests

url = "${origin.value}/v1/images/generations"
headers = {
    "Authorization": "Bearer YOUR_API_KEY",
    "Content-Type": "application/json"
}
data = {
    "model": "${model}",
    "prompt": "A cute orange cat playing with yarn",
    "n": 1,
    "size": "1024x1024"
}

resp = requests.post(url, headers=headers, json=data)
result = resp.json()
print(result["data"][0]["url"])`
})

const imagePythonRequestsWithRef = computed(() => {
  const model = selectedImageModel.value || 'gpt-image-2'
  return `import requests

url = "${origin.value}/v1/images/generations"
headers = {
    "Authorization": "Bearer YOUR_API_KEY",
    "Content-Type": "application/json"
}
data = {
    "model": "${model}",
    "prompt": "根据参考图生成一个类似风格的图片",
    "n": 1,
    "size": "1024x1024",
    "reference_images": [
        "https://example.com/your-image.jpg"
    ]
}

resp = requests.post(url, headers=headers, json=data)
result = resp.json()
print(result["data"][0]["url"])

# 说明:
# - reference_images 支持 HTTPS URL、data URL 或纯 base64
# - 最多支持 4 张参考图,单张最大 20MB`
})

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制到剪贴板')
  } catch {
    ElMessage.error('复制失败,请手动选择文本')
  }
}

// ---------- 状态标签 ----------
function statusTag(s: string): 'success' | 'warning' | 'danger' | 'info' {
  if (s === 'success') return 'success'
  if (s === 'failed') return 'danger'
  if (s === 'running' || s === 'dispatched' || s === 'queued') return 'warning'
  return 'info'
}

function thumbURL(url: string): string {
  if (!url) return url
  return url.includes('?') ? `${url}&thumb_kb=10` : `${url}?thumb_kb=10`
}

function previewImage(url: string, title = '') {
  if (!url) return
  previewImageURL.value = url
  previewImageTitle.value = title
  previewDialogVisible.value = true
}

function downloadImage(url: string, prompt: string) {
  if (!url) return
  const link = document.createElement('a')
  link.href = url
  link.download = `${(prompt || 'image').slice(0, 24).replace(/[\\\\/:*?\"<>|]/g, '_') || 'image'}.jpg`
  link.rel = 'noopener'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

// ---------- 初始化 ----------
onMounted(async () => {
  try {
    const m = await listMyModels()
    models.value = ENABLE_CHAT_MODEL
      ? m.items
      : m.items.filter((x) => x.type !== 'chat')
    const firstChat = m.items.find((x) => x.type === 'chat')
    const firstImage = m.items.find((x) => x.type === 'image')
    if (firstChat) selectedChatModel.value = firstChat.slug
    if (firstImage) selectedImageModel.value = firstImage.slug
  } catch {
    // 忽略
  }
  loadStats()
  if (ENABLE_CHAT_MODEL) loadChatLogs()
  loadImageTasks()
})
</script>

<template>
  <div class="page-container">
    <div class="card-block hero">
      <div>
        <h2 class="page-title">接口文档 & 用量</h2>
        <p class="desc">
          <template v-if="ENABLE_CHAT_MODEL">
            外部调用走 <code>/v1/chat/completions</code> 与 <code>/v1/images/generations</code>,
          </template>
          <template v-else>
            外部调用走 <code>/v1/images/generations</code>,
          </template>
          下面给出 curl / Python SDK 代码片段;个人用量与图片任务汇总在这里。若想在浏览器里直接体验,请打开「在线体验」。
        </p>
      </div>
      <div class="hero-stats" v-loading="statsLoading">
        <div class="stat">
          <div class="lbl">14 天请求</div>
          <div class="val">{{ stats?.overall.requests ?? 0 }}</div>
        </div>
        <div v-if="ENABLE_CHAT_MODEL" class="stat">
          <div class="lbl">文字 Token(in/out)</div>
          <div class="val">{{ stats?.overall.input_tokens ?? 0 }} / {{ stats?.overall.output_tokens ?? 0 }}</div>
        </div>
        <div class="stat">
          <div class="lbl">图片张数</div>
          <div class="val">{{ stats?.overall.image_images ?? 0 }}</div>
        </div>
        <div class="stat">
          <div class="lbl">14 天消耗积分</div>
          <div class="val primary">{{ formatCredit(stats?.overall.credit_cost) }}</div>
        </div>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="pg-tabs">
      <!-- ================== 文字对话 ================== -->
      <el-tab-pane v-if="ENABLE_CHAT_MODEL" label="对话生成(文字模型)" name="chat">
        <div class="card-block">
          <div class="row">
            <div class="label">文字模型</div>
            <el-select v-model="selectedChatModel" placeholder="选择模型" style="width: 320px">
              <el-option
                v-for="m in chatModels"
                :key="m.id"
                :label="`${m.slug}${m.description ? ' · ' + m.description : ''}`"
                :value="m.slug"
              />
            </el-select>
            <router-link to="/personal/keys">
              <el-button text type="primary">没有 Key?去「API Keys」创建</el-button>
            </router-link>
          </div>

          <el-tabs type="border-card" class="code-tabs">
            <el-tab-pane label="curl">
              <pre class="code"><code>{{ chatCurl }}</code></pre>
              <el-button size="small" @click="copy(chatCurl)">复制 curl</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (OpenAI SDK)">
              <pre class="code"><code>{{ chatPython }}</code></pre>
              <el-button size="small" @click="copy(chatPython)">复制 Python</el-button>
            </el-tab-pane>
          </el-tabs>
        </div>

        <div class="card-block">
          <div class="flex-between" style="margin-bottom: 10px">
            <h3 class="section-title">文字调用历史</h3>
            <el-button size="small" @click="loadChatLogs">刷新</el-button>
          </div>
          <el-table v-loading="chatLoading" :data="chatLogs" stripe size="small">
            <el-table-column prop="created_at" label="时间" min-width="160">
              <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column prop="model_slug" label="模型" min-width="140" />
            <el-table-column label="Token (in / out / cache)" min-width="170">
              <template #default="{ row }">
                {{ row.input_tokens }} / {{ row.output_tokens }}
                <span v-if="row.cache_read_tokens" class="mute">/ {{ row.cache_read_tokens }}</span>
              </template>
            </el-table-column>
            <el-table-column label="耗时" width="90">
              <template #default="{ row }">{{ row.duration_ms }} ms</template>
            </el-table-column>
            <el-table-column label="状态" width="90">
              <template #default="{ row }">
                <el-tag :type="statusTag(row.status)" size="small">{{ row.status }}</el-tag>
                <el-tooltip v-if="row.error_code" :content="formatErrorCode(row.error_code) + '(' + row.error_code + ')'">
                  <el-icon style="margin-left:4px"><InfoFilled /></el-icon>
                </el-tooltip>
              </template>
            </el-table-column>
            <el-table-column label="扣费(积分)" width="110">
              <template #default="{ row }">{{ formatCredit(row.credit_cost) }}</template>
            </el-table-column>
          </el-table>
          <div class="pager">
            <el-pagination
              layout="prev, pager, next, total"
              :total="chatPage.total"
              :page-size="chatPage.limit"
              :current-page="Math.floor(chatPage.offset / chatPage.limit) + 1"
              @current-change="chatPageChange"
            />
          </div>
        </div>
      </el-tab-pane>

      <!-- ================== 图片生成 ================== -->
      <el-tab-pane label="图片生成(图片模型)" name="image">
        <div class="card-block">
          <div class="row">
            <div class="label">图片模型</div>
            <el-select v-model="selectedImageModel" placeholder="选择模型" style="width: 320px">
              <el-option
                v-for="m in imageModels"
                :key="m.id"
                :label="`${m.slug}${m.description ? ' · ' + m.description : ''}`"
                :value="m.slug"
              />
            </el-select>
          </div>

          <el-tabs type="border-card" class="code-tabs">
            <el-tab-pane label="curl">
              <pre class="code"><code>{{ imageCurl }}</code></pre>
              <el-button size="small" @click="copy(imageCurl)">复制 curl</el-button>
            </el-tab-pane>
            <el-tab-pane label="curl (带参考图)">
              <pre class="code"><code>{{ imageCurlWithRef }}</code></pre>
              <el-button size="small" @click="copy(imageCurlWithRef)">复制 curl</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (OpenAI SDK)">
              <pre class="code"><code>{{ imagePython }}</code></pre>
              <el-button size="small" @click="copy(imagePython)">复制 Python</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (requests)">
              <pre class="code"><code>{{ imagePythonRequests }}</code></pre>
              <el-button size="small" @click="copy(imagePythonRequests)">复制 Python</el-button>
            </el-tab-pane>
            <el-tab-pane label="Python (requests 带参考图)">
              <pre class="code"><code>{{ imagePythonRequestsWithRef }}</code></pre>
              <el-button size="small" @click="copy(imagePythonRequestsWithRef)">复制 Python</el-button>
            </el-tab-pane>
          </el-tabs>
        </div>

        <div class="card-block">
          <div class="flex-between" style="margin-bottom: 10px">
            <h3 class="section-title">图片任务历史</h3>
            <el-button size="small" @click="loadImageTasks(true)">刷新</el-button>
          </div>
          <div class="img-filters">
            <div class="img-filters__fields">
              <el-input
                v-model="imageFilters.keyword"
                clearable
                placeholder="搜索提示词"
                class="filter-keyword"
                @keyup.enter="loadImageTasks(true)"
              />
              <el-select
                v-model="imageFilters.status"
                clearable
                placeholder="全部状态"
                class="filter-status"
              >
                <el-option label="排队中" value="queued" />
                <el-option label="已分发" value="dispatched" />
                <el-option label="运行中" value="running" />
                <el-option label="成功" value="success" />
                <el-option label="失败" value="failed" />
              </el-select>
              <el-date-picker
                v-model="imageFilters.range"
                type="datetimerange"
                unlink-panels
                range-separator="至"
                start-placeholder="开始时间"
                end-placeholder="结束时间"
                value-format="YYYY-MM-DD HH:mm:ss"
                class="filter-range"
              />
            </div>
            <div class="img-filters__actions">
              <el-button type="primary" @click="loadImageTasks(true)">筛选</el-button>
              <el-button @click="resetImageFilters">重置</el-button>
            </div>
          </div>
          <div v-loading="imageLoading">
            <div v-if="imageTasks.length === 0 && !imageLoading" class="empty">
              暂无图片任务,复制上方代码调用一次即可生成记录。
            </div>
            <div class="grid">
              <el-card
                v-for="t in imageTasks"
                :key="t.id"
                shadow="hover"
                class="img-card"
              >
                <div class="thumb">
                  <img
                    v-if="t.image_urls?.[0]"
                    :src="thumbURL(t.image_urls[0])"
                    :alt="t.prompt"
                    @click="previewImage(t.image_urls[0], t.prompt)"
                  />
                  <div v-else class="thumb-ph">
                    <el-icon :size="32"><PictureRounded /></el-icon>
                    <div class="s">{{ t.status }}</div>
                  </div>
                </div>
                <div class="meta">
                  <div class="title" :title="t.prompt">{{ t.prompt || '(无 prompt)' }}</div>
                  <div class="sub">
                    <el-tag size="small" :type="statusTag(t.status)">{{ t.status }}</el-tag>
                    <span>{{ t.size }}</span>
                    <span class="mute">n={{ t.n }}</span>
                  </div>
                  <div class="foot">
                    <span class="mute">{{ formatDateTime(t.created_at) }}</span>
                    <span class="credit">{{ formatCredit(t.credit_cost) }} 积分</span>
                  </div>
                  <div v-if="t.image_urls?.[0]" class="actions">
                    <el-button size="small" plain @click="previewImage(t.image_urls[0], t.prompt)">放大</el-button>
                    <el-button size="small" type="primary" plain @click="downloadImage(t.image_urls[0], t.prompt)">下载</el-button>
                  </div>
                  <div v-if="t.error" class="err">{{ t.error }}</div>
                </div>
              </el-card>
            </div>
            <div v-if="hasMoreImage" class="pager">
              <el-button @click="imageLoadMore">加载更多</el-button>
            </div>
          </div>
        </div>
      </el-tab-pane>
    </el-tabs>

    <el-dialog
      v-model="previewDialogVisible"
      width="min(92vw, 980px)"
      top="5vh"
      destroy-on-close
      class="img-preview-dialog"
    >
      <template #header>
        <div class="preview-title">{{ previewImageTitle || '图片预览' }}</div>
      </template>
      <div class="preview-wrap">
        <img v-if="previewImageURL" :src="previewImageURL" :alt="previewImageTitle || 'preview'" class="preview-img" />
      </div>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.page-container { padding: 16px; }
.page-title { margin: 0; font-size: 20px; font-weight: 700; }
.section-title { margin: 0; font-size: 16px; font-weight: 600; }
.card-block {
  background: var(--el-bg-color);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 8px;
  padding: 16px;
  margin-bottom: 16px;
}
.flex-between { display: flex; justify-content: space-between; align-items: center; }
.hero {
  display: flex; justify-content: space-between; gap: 24px; flex-wrap: wrap;
  .desc { color: var(--el-text-color-secondary); margin-top: 4px; font-size: 13px; }
  code {
    background: var(--el-fill-color-light); padding: 1px 6px; border-radius: 4px; font-size: 12px;
  }
}
.hero-stats {
  display: flex; gap: 24px; flex-wrap: wrap;
  .stat { min-width: 120px; }
  .lbl { font-size: 12px; color: var(--el-text-color-secondary); }
  .val { font-size: 22px; font-weight: 700; margin-top: 2px; }
  .val.primary { color: #409eff; }
}

.pg-tabs { :deep(.el-tabs__header) { margin-bottom: 12px; } }
.row {
  display: flex; gap: 12px; align-items: center; flex-wrap: wrap; margin-bottom: 12px;
  .label { font-weight: 600; min-width: 68px; }
}
.code-tabs {
  :deep(.el-tabs__content) { padding: 12px; }
}
.code {
  background: #1f2937; color: #e5e7eb; border-radius: 6px;
  padding: 12px 14px; margin: 0 0 10px; font-size: 12px; line-height: 1.6;
  overflow-x: auto; white-space: pre-wrap; word-break: break-word;
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
}
:global(html.dark) .code { background: #0f1115; }

.mute { color: var(--el-text-color-secondary); }
.pager { margin-top: 12px; display: flex; justify-content: flex-end; }
.empty { padding: 24px 0; color: var(--el-text-color-secondary); text-align: center; }
.img-filters {
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
  padding: 12px;
  background: var(--el-fill-color-extra-light);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 10px;
}
.img-filters__fields {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  flex: 0 1 auto;
  min-width: auto;
}
.img-filters__actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex: 0 0 auto;
}
.filter-keyword { width: 220px; }
.filter-status { width: 140px; }
.filter-range { width: 300px; max-width: 300px; }
.filter-range :deep(.el-range-editor.el-input__wrapper) { width: 100%; }
.preview-title {
  max-width: 100%;
  font-size: 15px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.preview-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 320px;
  max-height: 78vh;
  overflow: auto;
  background: var(--el-fill-color-lighter);
  border-radius: 8px;
}
.preview-img {
  max-width: 100%;
  max-height: 76vh;
  object-fit: contain;
}

.grid {
  display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px;
}
.img-card {
  :deep(.el-card__body) { padding: 0; }
  .thumb {
    height: 180px; display: flex; align-items: center; justify-content: center;
    background: var(--el-fill-color-lighter);
    img { max-width: 100%; max-height: 100%; object-fit: contain; cursor: zoom-in; }
  }
  .thumb-ph { text-align: center; color: var(--el-text-color-secondary); .s { font-size: 12px; } }
  .meta { padding: 10px 12px; }
  .title {
    font-size: 13px; font-weight: 600; margin-bottom: 6px;
    overflow: hidden; white-space: nowrap; text-overflow: ellipsis;
  }
  .sub { display: flex; gap: 6px; font-size: 12px; align-items: center; color: var(--el-text-color-regular); }
  .foot {
    display: flex; justify-content: space-between; margin-top: 6px; font-size: 12px;
    .credit { color: #e6a23c; font-weight: 600; }
  }
  .actions { display: flex; gap: 8px; margin-top: 10px; }
  .err {
    color: var(--el-color-danger); font-size: 12px; margin-top: 6px;
    background: var(--el-color-danger-light-9); padding: 4px 6px; border-radius: 4px;
    white-space: pre-wrap; word-break: break-word;
  }
}

@media (max-width: 640px) {
  .hero { flex-direction: column; }
  .hero-stats { gap: 16px; }
  .img-filters { align-items: stretch; }
  .img-filters__fields,
  .img-filters__actions { width: 100%; }
  .filter-keyword,
  .filter-status,
  .filter-range { width: 100%; }
  .img-filters__actions :deep(.el-button) { flex: 1; }
}
</style>

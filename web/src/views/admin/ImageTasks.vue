<script setup lang="ts">
import { computed, ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { http } from '@/api/http'
import { formatDateTime } from '@/utils/format'

interface TaskRow {
  id: number
  task_id: string
  user_id: number
  user_email: string
  prompt: string
  n: number
  size: string
  upscale: string
  status: string
  result_urls_parsed: string[]
  error: string
  credit_cost: number
  estimated_credit: number
  created_at: string
  started_at?: string | null
  finished_at?: string | null
}

const loading = ref(false)
const rows = ref<TaskRow[]>([])
const total = ref(0)
const filter = reactive({
  keyword: '',
  status: '',
  page: 1,
  page_size: 20,
})

async function fetchList() {
  loading.value = true
  try {
    const params: Record<string, any> = {
      page: filter.page,
      page_size: filter.page_size,
    }
    if (filter.keyword) params.keyword = filter.keyword
    if (filter.status) params.status = filter.status
    const d = await http.get<any, any>('/api/admin/image-tasks', { params })
    rows.value = d.list || []
    total.value = d.total || 0
  } finally {
    loading.value = false
  }
}

function onSearch() {
  filter.page = 1
  fetchList()
}
function onReset() {
  filter.keyword = ''
  filter.status = ''
  filter.page = 1
  fetchList()
}

// 弹窗预览图片
const previewDlg = ref(false)
const previewRow = ref<TaskRow | null>(null)
const previewIndex = ref(0)
const activePreviewURL = computed(() => {
  const urls = previewRow.value?.result_urls_parsed || []
  return urls[previewIndex.value] || ''
})
function openPreview(row: TaskRow, index = 0) {
  previewRow.value = row
  previewIndex.value = index
  previewDlg.value = true
}
function selectPreview(index: number) {
  previewIndex.value = index
}

async function downloadURL(url: string, filename: string) {
  const resp = await fetch(url, { credentials: 'include' })
  if (!resp.ok) throw new Error(`download ${resp.status}`)
  const blob = await resp.blob()
  const href = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = href
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(href)
}

function safeName(s: string) {
  return (s || 'image').replace(/[\\\\/:*?"<>|]/g, '_').slice(0, 24) || 'image'
}

async function downloadOne(row: TaskRow, index = 0) {
  const url = row.result_urls_parsed?.[index]
  if (!url) return
  try {
    await downloadURL(url, `${safeName(row.prompt)}-${row.task_id}-${index + 1}.jpg`)
    ElMessage.success('开始下载')
  } catch {
    ElMessage.error('下载失败')
  }
}

async function downloadBatch(row: TaskRow) {
  const urls = row.result_urls_parsed || []
  if (urls.length === 0) return
  let ok = 0
  for (let i = 0; i < urls.length; i += 1) {
    try {
      await downloadURL(urls[i], `${safeName(row.prompt)}-${row.task_id}-${i + 1}.jpg`)
      ok += 1
      await new Promise((resolve) => setTimeout(resolve, 120))
    } catch {
      // 跳过失败项，继续后续下载
    }
  }
  ElMessage[ok > 0 ? 'success' : 'error'](ok > 0 ? `已触发 ${ok} 张下载` : '批量下载失败')
}

const statusColor: Record<string, 'success' | 'danger' | 'warning' | 'info' | 'primary'> = {
  success: 'success',
  failed: 'danger',
  running: 'warning',
  queued: 'info',
  dispatched: 'info',
}

onMounted(fetchList)
</script>

<template>
  <div class="page-container">
    <div class="card-block">
      <h2 class="page-title" style="margin:0">生成记录</h2>
      <div style="color:var(--el-text-color-secondary);font-size:13px;margin:4px 0 14px">
        全站图片生成任务历史,含用户、提示词、生成结果与耗时。
      </div>

      <el-form inline class="flex-wrap-gap" @submit.prevent="onSearch">
        <el-input v-model="filter.keyword" placeholder="提示词 / 邮箱" clearable style="width:260px" />
        <el-select v-model="filter.status" placeholder="状态" clearable style="width:130px">
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
          <el-option label="运行中" value="running" />
          <el-option label="队列中" value="queued" />
        </el-select>
        <el-button type="primary" @click="onSearch"><el-icon><Search /></el-icon> 查询</el-button>
        <el-button @click="onReset">重置</el-button>
      </el-form>

      <el-table v-loading="loading" :data="rows" stripe style="margin-top:12px" size="small">
        <el-table-column prop="id" label="ID" width="72" />
        <el-table-column label="用户" min-width="170">
          <template #default="{ row }">
            <div>{{ row.user_email || '-' }}</div>
            <div style="font-size:11px;color:var(--el-text-color-secondary)">uid {{ row.user_id }}</div>
          </template>
        </el-table-column>
        <el-table-column label="提示词" min-width="240" show-overflow-tooltip>
          <template #default="{ row }">
            <span>{{ row.prompt || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="规格" width="110">
          <template #default="{ row }">
            <div>{{ row.size }}</div>
            <div v-if="row.upscale" style="font-size:11px;color:var(--el-color-success)">{{ row.upscale }}</div>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="statusColor[row.status] || 'info'" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="结果" width="190">
          <template #default="{ row }">
            <div v-if="row.result_urls_parsed?.length" style="display:flex;gap:8px;align-items:center;flex-wrap:wrap">
              <el-button
                type="primary" link size="small"
                @click="openPreview(row)"
              >放大({{ row.result_urls_parsed.length }})</el-button>
              <el-button
                type="success" link size="small"
                @click="downloadOne(row)"
              >下载</el-button>
              <el-button
                v-if="row.result_urls_parsed.length > 1"
                type="warning" link size="small"
                @click="downloadBatch(row)"
              >批量下载</el-button>
            </div>
            <span v-else-if="row.error" style="font-size:11px;color:var(--el-color-danger)" :title="row.error">失败</span>
            <span v-else style="color:var(--el-text-color-secondary)">-</span>
          </template>
        </el-table-column>
        <el-table-column label="积分" width="100">
          <template #default="{ row }">
            <div>{{ row.credit_cost }}</div>
            <div style="font-size:11px;color:var(--el-text-color-secondary)">预估 {{ row.estimated_credit }}</div>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">{{ formatDateTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="完成时间" width="160">
          <template #default="{ row }">{{ row.finished_at ? formatDateTime(row.finished_at) : '-' }}</template>
        </el-table-column>
      </el-table>

      <el-pagination
        style="margin-top:16px;justify-content:flex-end;display:flex"
        :current-page="filter.page"
        @current-change="(p: number) => { filter.page = p; fetchList() }"
        :page-size="filter.page_size"
        @size-change="(s: number) => { filter.page_size = s; filter.page = 1; fetchList() }"
        :total="total"
        :page-sizes="[20, 50, 100]"
        layout="total, sizes, prev, pager, next"
      />
    </div>

    <!-- 图片预览弹窗 -->
    <el-dialog v-model="previewDlg" title="生成结果预览" width="820px">
      <div v-if="previewRow">
        <div style="font-size:13px;color:var(--el-text-color-secondary);margin-bottom:10px;word-break:break-all">
          {{ previewRow.prompt }}
        </div>
        <div style="display:flex;justify-content:flex-end;gap:8px;margin-bottom:12px">
          <el-button size="small" type="success" @click="downloadOne(previewRow, previewIndex)">下载当前</el-button>
          <el-button
            v-if="previewRow.result_urls_parsed.length > 1"
            size="small"
            type="warning"
            @click="downloadBatch(previewRow)"
          >批量下载</el-button>
        </div>
        <div style="display:flex;align-items:center;justify-content:center;min-height:360px;background:var(--el-fill-color-lighter);border-radius:8px;padding:12px">
          <img
            v-if="activePreviewURL"
            :src="activePreviewURL"
            alt="preview"
            style="max-width:100%;max-height:60vh;object-fit:contain;border-radius:6px"
          />
        </div>
        <div style="display:flex;flex-wrap:wrap;gap:8px;margin-top:12px">
          <div
            v-for="(url, idx) in previewRow.result_urls_parsed"
            :key="idx"
            style="width:96px;height:96px;border-radius:6px;overflow:hidden;cursor:pointer;border:2px solid transparent"
            :style="{ borderColor: idx === previewIndex ? 'var(--el-color-primary)' : 'transparent' }"
            @click="selectPreview(idx)"
          >
            <el-image
              :src="url"
              fit="cover"
              style="width:96px;height:96px"
              lazy
            />
          </div>
        </div>
        <div v-if="previewRow.error" style="margin-top:12px;color:var(--el-color-danger);font-size:12px;word-break:break-all">
          错误:{{ previewRow.error }}
        </div>
      </div>
    </el-dialog>
  </div>
</template>

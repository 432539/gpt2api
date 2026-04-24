<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { http } from '@/api/http'
import { formatDateTime } from '@/utils/format'
import ImagePreviewDialog from '@/components/ImagePreviewDialog.vue'

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
  thumb_urls_parsed?: string[]
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
function openPreview(row: TaskRow) {
  previewRow.value = row
  previewIndex.value = 0
  previewDlg.value = true
}

function openPreviewAt(row: TaskRow, idx: number) {
  previewRow.value = row
  previewIndex.value = idx
  previewDlg.value = true
}

function rowThumbs(row: TaskRow) {
  if (row.thumb_urls_parsed?.length) return row.thumb_urls_parsed
  return row.result_urls_parsed || []
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
        <el-table-column label="结果" width="120">
          <template #default="{ row }">
            <div v-if="row.result_urls_parsed?.length" class="result-cell">
              <img
                v-if="rowThumbs(row)[0]"
                :src="rowThumbs(row)[0]"
                :alt="row.task_id"
                class="result-thumb"
                @click="openPreviewAt(row, 0)"
              />
              <el-button type="primary" link size="small" @click="openPreview(row)">
                预览({{ row.result_urls_parsed.length }})
              </el-button>
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

    <ImagePreviewDialog
      v-model="previewDlg"
      :title="previewRow ? `任务 ${previewRow.task_id}` : '生成结果预览'"
      :description="previewRow?.prompt || ''"
      :urls="previewRow?.result_urls_parsed || []"
      :thumb-urls="previewRow ? rowThumbs(previewRow) : []"
      :initial-index="previewIndex"
    />
  </div>
</template>

<style scoped lang="scss">
.result-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.result-thumb {
  width: 42px;
  height: 42px;
  display: block;
  border-radius: 8px;
  object-fit: cover;
  background: var(--el-fill-color-lighter);
  cursor: pointer;
  flex-shrink: 0;
}
</style>

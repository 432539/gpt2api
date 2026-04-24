<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  modelValue: boolean
  urls: string[]
  thumbUrls?: string[]
  title?: string
  description?: string
  initialIndex?: number
}>(), {
  thumbUrls: () => [],
  title: '图片预览',
  description: '',
  initialIndex: 0,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const currentIndex = ref(0)

const visible = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value),
})

const total = computed(() => props.urls.length)
const canSwitch = computed(() => total.value > 1)
const currentURL = computed(() => props.urls[currentIndex.value] || '')

function clampIndex(idx: number) {
  if (total.value <= 0) return 0
  if (idx < 0) return 0
  if (idx >= total.value) return total.value - 1
  return idx
}

function syncIndex() {
  currentIndex.value = clampIndex(props.initialIndex)
}

watch(() => props.modelValue, (open) => {
  if (open) syncIndex()
})

watch(() => props.initialIndex, () => {
  if (props.modelValue) syncIndex()
})

watch(() => props.urls, () => {
  currentIndex.value = clampIndex(currentIndex.value)
}, { deep: false })

function prevImage() {
  if (!canSwitch.value) return
  currentIndex.value = currentIndex.value > 0 ? currentIndex.value - 1 : total.value - 1
}

function nextImage() {
  if (!canSwitch.value) return
  currentIndex.value = currentIndex.value < total.value - 1 ? currentIndex.value + 1 : 0
}

function selectImage(idx: number) {
  currentIndex.value = clampIndex(idx)
}

function downloadCurrent() {
  if (!currentURL.value) return
  const a = document.createElement('a')
  a.href = currentURL.value
  a.target = '_blank'
  a.rel = 'noopener'
  a.download = ''
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}
</script>

<template>
  <el-dialog v-model="visible" :title="title" width="960px" class="image-preview-dialog">
    <div v-if="total > 0" class="preview-shell">
      <div v-if="description" class="preview-desc">{{ description }}</div>

      <div class="preview-toolbar">
        <div class="preview-counter">{{ currentIndex + 1 }} / {{ total }}</div>
        <div class="preview-actions">
          <el-button plain @click="downloadCurrent">
            <el-icon><Download /></el-icon> 下载原图
          </el-button>
          <el-button plain :disabled="!currentURL" tag="a" :href="currentURL" target="_blank" rel="noopener">
            <el-icon><TopRight /></el-icon> 新页打开
          </el-button>
        </div>
      </div>

      <div class="preview-stage">
        <button v-if="canSwitch" class="nav-btn left" type="button" @click="prevImage" aria-label="上一张">
          <el-icon><ArrowLeftBold /></el-icon>
        </button>
        <div class="preview-main">
          <img :src="currentURL" :alt="`${title}-${currentIndex + 1}`" />
        </div>
        <button v-if="canSwitch" class="nav-btn right" type="button" @click="nextImage" aria-label="下一张">
          <el-icon><ArrowRightBold /></el-icon>
        </button>
      </div>

      <div v-if="canSwitch" class="preview-strip">
        <button
          v-for="(url, idx) in urls"
          :key="`${idx}-${url}`"
          type="button"
          class="strip-item"
          :class="{ active: idx === currentIndex }"
          @click="selectImage(idx)"
        >
          <img :src="thumbUrls[idx] || url" :alt="`${title}-thumb-${idx + 1}`" />
        </button>
      </div>
    </div>
    <div v-else class="preview-empty">暂无可预览图片</div>
  </el-dialog>
</template>

<style scoped lang="scss">
.preview-shell {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.preview-desc {
  color: var(--el-text-color-secondary);
  font-size: 13px;
  line-height: 1.6;
  word-break: break-word;
}

.preview-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.preview-counter {
  color: var(--el-text-color-secondary);
  font-size: 13px;
}

.preview-actions {
  display: flex;
  gap: 8px;
}

.preview-stage {
  position: relative;
  border-radius: 18px;
  background: var(--el-fill-color-lighter);
  min-height: 520px;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.preview-main {
  width: 100%;
  height: 100%;
  min-height: 520px;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px 64px;

  img {
    max-width: 100%;
    max-height: 72vh;
    object-fit: contain;
    border-radius: 12px;
    box-shadow: 0 20px 60px rgba(15, 23, 42, 0.12);
    background: #fff;
  }
}

.nav-btn {
  position: absolute;
  top: 50%;
  transform: translateY(-50%);
  z-index: 2;
  width: 42px;
  height: 42px;
  border: none;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.68);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;

  &.left { left: 16px; }
  &.right { right: 16px; }
}

.preview-strip {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 10px;
}

.strip-item {
  width: 100%;
  aspect-ratio: 1;
  border-radius: 12px;
  overflow: hidden;
  border: 2px solid transparent;
  padding: 0;
  background: var(--el-fill-color-lighter);
  cursor: pointer;

  &.active {
    border-color: var(--el-color-primary);
    box-shadow: 0 0 0 3px rgba(64, 158, 255, 0.12);
  }

  img {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: cover;
  }
}

.preview-empty {
  min-height: 260px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--el-text-color-secondary);
}

@media (max-width: 900px) {
  .preview-stage {
    min-height: 360px;
  }

  .preview-main {
    min-height: 360px;
    padding: 20px 48px;
  }

  .preview-toolbar {
    flex-direction: column;
    align-items: flex-start;
  }

  .preview-actions {
    width: 100%;
  }
}
</style>

<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex justify-end">
        <button class="btn btn-secondary inline-flex items-center gap-2" :disabled="historyLoading" @click="() => loadHistory()">
          <Icon name="refresh" size="sm" :class="historyLoading ? 'animate-spin' : ''" />
          刷新
        </button>
      </div>

      <div
        v-if="errorMessage"
        class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-200"
      >
        {{ errorMessage }}
      </div>

      <div class="grid gap-5 xl:grid-cols-[430px_minmax(0,1fr)]">
        <section class="card">
          <div class="border-b border-gray-100 px-5 py-4 dark:border-dark-700">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">生成参数</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  提交后任务会立即进入右侧列表，可继续准备下一张。
                </p>
              </div>
              <div class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition"
                  :class="mode === 'generation' ? activeTabClass : idleTabClass"
                  @click="switchMode('generation')"
                >
                  生图
                </button>
                <button
                  type="button"
                  class="rounded-md px-3 py-1.5 text-sm font-medium transition"
                  :class="mode === 'edit' ? activeTabClass : idleTabClass"
                  @click="switchMode('edit')"
                >
                  编辑
                </button>
              </div>
            </div>
          </div>

          <div class="space-y-5 p-5">
            <div>
              <label class="input-label">提示词</label>
              <textarea
                v-model="prompt"
                rows="7"
                class="input min-h-[168px] resize-y"
                placeholder="描述你想生成或编辑的图片"
              />
            </div>

            <div v-if="mode === 'edit'" class="space-y-3">
              <div class="rounded-lg border border-dashed border-gray-300 p-3 dark:border-dark-600">
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <label class="input-label">输入图片</label>
                    <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                      可上传新图，也可从右侧历史图片点击“编辑”直接引用。
                    </p>
                  </div>
                  <button
                    v-if="sourceResultId || inputImage"
                    type="button"
                    class="btn btn-secondary btn-sm"
                    @click="clearEditSource"
                  >
                    清除
                  </button>
                </div>
                <input class="input mt-3" type="file" accept="image/png,image/jpeg,image/webp" @change="onImageChange" />
                <div
                  v-if="sourceResultId"
                  class="mt-3 flex items-center gap-3 rounded-md bg-primary-50 p-3 text-sm text-primary-800 dark:bg-primary-950/30 dark:text-primary-200"
                >
                  <img
                    v-if="sourcePreviewUrl"
                    :src="sourcePreviewUrl"
                    class="h-12 w-12 rounded-md object-cover"
                    alt=""
                  />
                  <span>正在引用历史图片 #{{ sourceResultId }}，提交时由后端本地读取，不会重新上传原图。</span>
                </div>
              </div>

              <div>
                <label class="input-label">遮罩图片</label>
                <input class="input" type="file" accept="image/png,image/jpeg,image/webp" @change="onMaskChange" />
              </div>
            </div>

            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="input-label">模型</label>
                <input v-model="model" class="input" placeholder="gpt-image-2" />
              </div>
              <div>
                <label class="input-label">数量</label>
                <input v-model.number="n" class="input" type="number" min="1" max="4" />
              </div>
            </div>

            <div class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-700">
              <div class="flex items-center justify-between">
                <label class="input-label mb-0">尺寸</label>
                <span class="text-xs text-gray-500 dark:text-gray-400">{{ resolvedSize }}</span>
              </div>

              <div class="grid grid-cols-3 gap-2">
                <button
                  v-for="tier in sizeTiers"
                  :key="tier"
                  type="button"
                  class="rounded-md border px-3 py-2 text-sm font-medium transition"
                  :class="sizeTier === tier ? selectedControlClass : plainControlClass"
                  @click="sizeTier = tier"
                >
                  {{ tier }}
                </button>
              </div>

              <div class="grid grid-cols-3 gap-2">
                <button
                  v-for="ratio in sizeRatios"
                  :key="ratio.value"
                  type="button"
                  class="rounded-md border px-3 py-2 text-sm font-medium transition"
                  :class="sizeRatio === ratio.value && !customSizeEnabled ? selectedControlClass : plainControlClass"
                  @click="selectRatio(ratio.value)"
                >
                  {{ ratio.label }}
                </button>
              </div>

              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="customSizeEnabled" type="checkbox" class="rounded border-gray-300 text-primary-600" />
                自定义尺寸
              </label>
              <div v-if="customSizeEnabled" class="grid grid-cols-2 gap-2">
                <input v-model.number="customWidth" class="input" type="number" min="1" placeholder="宽度" />
                <input v-model.number="customHeight" class="input" type="number" min="1" placeholder="高度" />
              </div>
            </div>

            <div>
              <label class="input-label">质量</label>
              <select v-model="quality" class="input">
                <option value="auto">auto</option>
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </div>

            <button class="btn btn-primary w-full justify-center gap-2" :disabled="!canSubmit" @click="submit">
              <Icon name="sparkles" size="sm" :class="submitting ? 'animate-spin' : ''" />
              {{ submitting ? '提交中...' : mode === 'edit' ? '提交编辑' : '生成图片' }}
            </button>
          </div>
        </section>

        <section class="space-y-4">
          <div class="card">
            <div class="flex flex-col gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">结果与历史</h2>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  当前共 {{ total }} 条任务，{{ pendingCount }} 条正在生成，保存 {{ bootstrap?.retention_days || 30 }} 天。
                </p>
              </div>
            </div>

            <div v-if="historyLoading && tasks.length === 0" class="p-8 text-center text-sm text-gray-500">
              正在加载图片历史...
            </div>
            <div v-else-if="tasks.length === 0" class="p-8 text-center text-sm text-gray-500">
              还没有图片记录。
            </div>
            <div v-else class="grid gap-4 p-5 sm:grid-cols-2 2xl:grid-cols-3">
              <article
                v-for="task in tasks"
                :key="task.local_id || task.id"
                class="overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
              >
                <button
                  v-if="task.results[0]"
                  type="button"
                  class="block w-full"
                  @click="openPreview(task, task.results[0])"
                >
                  <img
                    :src="imageUrl(task.results[0].id)"
                    class="aspect-square w-full bg-gray-100 object-cover transition hover:opacity-90 dark:bg-dark-700"
                    :alt="task.prompt"
                  />
                </button>
                <div
                  v-else
                  class="flex aspect-square w-full flex-col items-center justify-center gap-3 bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-dark-300"
                >
                  <div
                    class="h-8 w-8 rounded-full border-2 border-primary-500 border-t-transparent"
                    :class="task.status === 'pending' ? 'animate-spin' : ''"
                  />
                  <span class="text-sm">{{ task.status === 'failed' ? '生成失败' : '生成中...' }}</span>
                </div>

                <div class="space-y-3 p-3">
                  <div class="flex items-center justify-between gap-2">
                    <span class="rounded-full px-2 py-0.5 text-xs font-medium" :class="statusClass(task.status)">
                      {{ statusLabel(task.status) }}
                    </span>
                    <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(task.created_at) }}</span>
                  </div>
                  <p class="line-clamp-2 min-h-[2.5rem] text-sm text-gray-800 dark:text-gray-100">
                    {{ task.prompt || '无提示词' }}
                  </p>
                  <p v-if="task.error_message" class="text-xs text-red-600 dark:text-red-300">
                    {{ task.error_message }}
                  </p>
                  <div class="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400">
                    <span>{{ task.model }}</span>
                    <span>{{ task.size || '' }}</span>
                  </div>
                  <div class="flex flex-wrap gap-2">
                    <button
                      v-for="result in task.results"
                      :key="result.id"
                      type="button"
                      class="btn btn-secondary btn-sm inline-flex items-center gap-1"
                      @click="downloadResult(result)"
                    >
                      <Icon name="download" size="xs" />
                      下载 {{ result.index + 1 }}
                    </button>
                    <button
                      v-if="task.results[0]"
                      type="button"
                      class="btn btn-secondary btn-sm inline-flex items-center gap-1"
                      @click="editFromResult(task, task.results[0])"
                    >
                      <Icon name="edit" size="xs" />
                      编辑
                    </button>
                    <button
                      type="button"
                      class="btn btn-secondary btn-sm inline-flex items-center gap-1"
                      @click="reuse(task)"
                    >
                      <Icon name="copy" size="xs" />
                      复用
                    </button>
                    <button
                      v-if="!task.local_id"
                      type="button"
                      class="btn btn-danger btn-sm inline-flex items-center gap-1"
                      @click="remove(task)"
                    >
                      <Icon name="trash" size="xs" />
                      删除
                    </button>
                    <button
                      v-else
                      type="button"
                      class="btn btn-secondary btn-sm inline-flex items-center gap-1"
                      @click="removeLocal(task)"
                    >
                      <Icon name="x" size="xs" />
                      移除
                    </button>
                  </div>
                </div>
              </article>
            </div>
            <div
              v-if="tasks.length > 0"
              class="flex items-center justify-center border-t border-gray-100 px-5 py-4 dark:border-dark-700"
            >
              <button
                v-if="hasMore"
                type="button"
                class="btn btn-secondary inline-flex items-center gap-2"
                :disabled="loadingMore"
                @click="loadMore"
              >
                <Icon name="refresh" size="sm" :class="loadingMore ? 'animate-spin' : ''" />
                {{ loadingMore ? '加载中...' : '加载更多' }}
              </button>
              <span v-else class="text-xs text-gray-500 dark:text-gray-400">
                已显示全部 {{ total }} 条记录
              </span>
            </div>
          </div>
        </section>
      </div>

      <div
        v-if="preview"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
        @click.self="closePreview"
      >
        <div class="max-h-[92vh] w-full max-w-5xl overflow-hidden rounded-lg bg-white shadow-xl dark:bg-dark-800">
          <div class="flex items-center justify-between border-b border-gray-100 px-4 py-3 dark:border-dark-700">
            <div>
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">图片预览</h3>
              <p class="text-xs text-gray-500 dark:text-gray-400">{{ preview.task.model }} · {{ preview.task.size || '' }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" @click="closePreview">
              <Icon name="x" size="xs" />
            </button>
          </div>
          <div class="grid max-h-[calc(92vh-56px)] gap-0 overflow-auto lg:grid-cols-[minmax(0,1fr)_320px]">
            <div class="flex items-center justify-center bg-gray-950 p-4">
              <img :src="imageUrl(preview.result.id)" class="max-h-[75vh] max-w-full object-contain" :alt="preview.task.prompt" />
            </div>
            <div class="space-y-4 p-4">
              <div>
                <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">Prompt</div>
                <p class="mt-2 whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-100">{{ preview.task.prompt || '无提示词' }}</p>
              </div>
              <div class="flex flex-wrap gap-2">
                <button type="button" class="btn btn-primary btn-sm inline-flex items-center gap-1" @click="editFromResult(preview.task, preview.result)">
                  <Icon name="edit" size="xs" />
                  基于此图编辑
                </button>
                <button type="button" class="btn btn-secondary btn-sm inline-flex items-center gap-1" @click="downloadResult(preview.result)">
                  <Icon name="download" size="xs" />
                  下载
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { Icon } from '@/components/icons'
import {
  createImageEdit,
  createImageGeneration,
  deleteImageGenerationTask,
  getImageGenerationFileBlob,
  getImageGenerationBootstrap,
  getImageGenerationTask,
  listImageGenerationTasks,
  type ImageGenerationResult,
  type ImageGenerationSettings,
  type ImageGenerationTask,
} from '@/api/imageGeneration'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

type LocalTask = ImageGenerationTask & {
  local_id?: string
  error_message?: string
  size?: string
}

type SizeTier = '1K' | '2K' | '4K'
type SizeRatio = '1:1' | '2:3' | '3:2' | '9:16' | '16:9'

const appStore = useAppStore()
const activeTabClass = 'bg-white text-primary-700 shadow-sm dark:bg-dark-800 dark:text-primary-300'
const idleTabClass = 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-white'
const selectedControlClass = 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-500 dark:bg-primary-950/30 dark:text-primary-200'
const plainControlClass = 'border-gray-200 bg-white text-gray-700 hover:border-primary-300 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200'

const sizeTiers: SizeTier[] = ['1K', '2K', '4K']
const sizeRatios: Array<{ label: string; value: SizeRatio }> = [
  { label: '1:1', value: '1:1' },
  { label: '2:3', value: '2:3' },
  { label: '3:2', value: '3:2' },
  { label: '9:16', value: '9:16' },
  { label: '16:9', value: '16:9' },
]

const sizeMap: Record<SizeTier, Record<SizeRatio, string>> = {
  '1K': {
    '1:1': '1024x1024',
    '2:3': '1024x1536',
    '3:2': '1536x1024',
    '9:16': '1024x1792',
    '16:9': '1792x1024',
  },
  '2K': {
    '1:1': '2048x2048',
    '2:3': '2048x3072',
    '3:2': '3072x2048',
    '9:16': '2048x3584',
    '16:9': '3584x2048',
  },
  '4K': {
    '1:1': '4096x4096',
    '2:3': '4096x6144',
    '3:2': '6144x4096',
    '9:16': '4096x7168',
    '16:9': '7168x4096',
  },
}

const bootstrap = ref<ImageGenerationSettings | null>(null)
const tasks = ref<LocalTask[]>([])
const total = ref(0)
const mode = ref<'generation' | 'edit'>('generation')
const prompt = ref('')
const model = ref('gpt-image-2')
const quality = ref('auto')
const n = ref(1)
const sizeTier = ref<SizeTier>('1K')
const sizeRatio = ref<SizeRatio>('1:1')
const customSizeEnabled = ref(false)
const customWidth = ref(1024)
const customHeight = ref(1024)
const inputImage = ref<File | null>(null)
const maskImage = ref<File | null>(null)
const sourceResultId = ref<number | null>(null)
const sourcePreviewUrl = ref('')
const historyLoading = ref(false)
const loadingMore = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const imageUrls = ref<Record<number, string>>({})
const preview = ref<{ task: LocalTask; result: ImageGenerationResult } | null>(null)
const currentPage = ref(1)
const pageSize = 24
const totalPages = ref(1)
let pendingRefreshTimer: number | null = null
const pendingRefreshIntervalMs = 30000

const resolvedSize = computed(() => {
  if (customSizeEnabled.value) {
    const width = Math.max(1, Number(customWidth.value) || 1024)
    const height = Math.max(1, Number(customHeight.value) || 1024)
    return `${width}x${height}`
  }
  return sizeMap[sizeTier.value][sizeRatio.value]
})

const pendingCount = computed(() => tasks.value.filter((task) => task.status === 'pending').length)
const hasMore = computed(() => currentPage.value < totalPages.value)

const canSubmit = computed(() => {
  if (submitting.value) return false
  if (!prompt.value.trim()) return false
  if (mode.value === 'edit' && !inputImage.value && !sourceResultId.value) return false
  return !!bootstrap.value?.enabled
})

function switchMode(nextMode: 'generation' | 'edit') {
  mode.value = nextMode
}

function selectRatio(ratio: SizeRatio) {
  sizeRatio.value = ratio
  customSizeEnabled.value = false
}

function onImageChange(event: Event) {
  inputImage.value = (event.target as HTMLInputElement).files?.[0] ?? null
  if (inputImage.value) {
    sourceResultId.value = null
    sourcePreviewUrl.value = ''
  }
}

function onMaskChange(event: Event) {
  maskImage.value = (event.target as HTMLInputElement).files?.[0] ?? null
}

async function loadBootstrap() {
  try {
    bootstrap.value = await getImageGenerationBootstrap()
    model.value = bootstrap.value.default_model || 'gpt-image-2'
    errorMessage.value = ''
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, '图片生成页面未启用或配置不完整')
  }
}

async function loadHistory(options: { reset?: boolean; silent?: boolean; pageSizeOverride?: number } = {}) {
  const reset = options.reset ?? true
  const targetPage = reset ? 1 : currentPage.value + 1
  const targetPageSize = options.pageSizeOverride || pageSize
  if (reset && !options.silent) historyLoading.value = true
  if (!reset) loadingMore.value = true
  if (reset && !options.pageSizeOverride) currentPage.value = 1
  try {
    const data = await listImageGenerationTasks(targetPage, targetPageSize)
    const nextItems = data.items.map(normalizeTask)
    if (reset) {
      revokeAllImageUrls()
      tasks.value = nextItems
      currentPage.value = options.pageSizeOverride ? Math.ceil(targetPageSize / pageSize) : data.page
    } else {
      tasks.value = mergeTaskLists(tasks.value, nextItems)
      currentPage.value = data.page
    }
    total.value = data.total
    totalPages.value = Math.max(1, Math.ceil(data.total / pageSize))
    await attachImageUrls(tasks.value)
    schedulePendingRefresh()
  } catch (error) {
    errorMessage.value = extractApiErrorMessage(error, '加载图片历史失败')
  } finally {
    if (reset && !options.silent) historyLoading.value = false
    if (!reset) loadingMore.value = false
  }
}

async function loadMore() {
  if (!hasMore.value || loadingMore.value) return
  await loadHistory({ reset: false })
}

async function submit() {
  if (!canSubmit.value) return
  submitting.value = true
  const localID = `pending-${Date.now()}-${Math.random().toString(16).slice(2)}`
  const snapshot = buildCommonPayload()
  const pendingTask: LocalTask = {
    id: 0,
    local_id: localID,
    mode: mode.value,
    status: 'pending',
    model: String(snapshot.model),
    prompt: String(snapshot.prompt),
    expires_at: '',
    created_at: new Date().toISOString(),
    results: [],
    size: String(snapshot.size),
  }
  tasks.value = [pendingTask, ...tasks.value]
  errorMessage.value = ''

  try {
    const request = mode.value === 'edit' ? submitEdit(snapshot) : submitGeneration(snapshot)
    prompt.value = ''
    submitting.value = false
    const task = await request
    const normalized = normalizeTask(task)
    const mergeResult = mergeSubmittedTask(localID, normalized)
    if (mergeResult.added) total.value += 1
    await attachImageUrls([normalized])
    schedulePendingRefresh()
    appStore.showSuccess(normalized.status === 'pending' ? '图片生成任务已提交' : '图片生成成功')
  } catch (error) {
    const message = extractApiErrorMessage(error, '图片生成失败')
    markLocalTaskFailed(localID, message)
    errorMessage.value = message
  } finally {
    submitting.value = false
  }
}

function buildCommonPayload() {
  return {
    model: model.value.trim() || bootstrap.value?.default_model || 'gpt-image-2',
    prompt: prompt.value.trim(),
    size: resolvedSize.value,
    quality: quality.value,
    n: Math.min(4, Math.max(1, Number(n.value) || 1)),
  }
}

async function submitGeneration(payload: Record<string, unknown>) {
  return createImageGeneration(payload)
}

async function submitEdit(payload: Record<string, unknown>) {
  const form = new FormData()
  Object.entries(payload).forEach(([key, value]) => form.append(key, String(value)))
  form.append('response_format', 'b64_json')
  if (inputImage.value) {
    form.append('image', inputImage.value)
  } else if (sourceResultId.value) {
    form.append('source_result_id', String(sourceResultId.value))
  }
  if (maskImage.value) form.append('mask', maskImage.value)
  return createImageEdit(form)
}

function normalizeTask(task: ImageGenerationTask): LocalTask {
  return {
    ...task,
    size: extractTaskSize(task),
  }
}

function extractTaskSize(task: ImageGenerationTask) {
  const raw = (task as unknown as { request_json?: Record<string, unknown>; size?: string }).request_json
  if (raw && typeof raw.size === 'string') return raw.size
  return (task as unknown as { size?: string }).size || ''
}

function mergeSubmittedTask(localID: string, task: LocalTask) {
  let replacedLocal = false
  let replacedServer = false
  tasks.value = tasks.value.map((item) => {
    if (item.local_id === localID) {
      replacedLocal = true
      return task
    }
    if (!item.local_id && item.id === task.id) {
      replacedServer = true
      return task
    }
    return item
  })
  if (!replacedLocal && !replacedServer) {
    tasks.value = [task, ...tasks.value]
  }
  return { added: replacedLocal || (!replacedServer && !replacedLocal) }
}

function mergeTaskLists(existing: LocalTask[], incoming: LocalTask[]) {
  const byKey = new Map<string, LocalTask>()
  for (const task of existing) {
    byKey.set(taskKey(task), task)
  }
  for (const task of incoming) {
    byKey.set(taskKey(task), task)
  }
  return Array.from(byKey.values()).sort((a, b) => {
    const timeDiff = new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    if (timeDiff !== 0) return timeDiff
    return Number(b.id || 0) - Number(a.id || 0)
  })
}

function taskKey(task: LocalTask) {
  return task.local_id ? `local:${task.local_id}` : `server:${task.id}`
}

function markLocalTaskFailed(localID: string, message: string) {
  tasks.value = tasks.value.map((item) => (
    item.local_id === localID
      ? { ...item, status: 'failed', error_message: message }
      : item
  ))
}

function removeLocal(task: LocalTask) {
  tasks.value = tasks.value.filter((item) => item.local_id !== task.local_id)
}

function reuse(task: LocalTask) {
  prompt.value = task.prompt
  model.value = task.model
  mode.value = task.mode === 'edit' ? 'edit' : 'generation'
  applySize(task.size || '')
}

function editFromResult(task: LocalTask, result: ImageGenerationResult) {
  prompt.value = task.prompt
  model.value = task.model
  mode.value = 'edit'
  sourceResultId.value = result.id
  sourcePreviewUrl.value = imageUrl(result.id)
  inputImage.value = null
  applySize(task.size || '')
  closePreview()
}

function applySize(value: string) {
  if (!value) return
  for (const tier of sizeTiers) {
    for (const ratio of sizeRatios) {
      if (sizeMap[tier][ratio.value] === value) {
        sizeTier.value = tier
        sizeRatio.value = ratio.value
        customSizeEnabled.value = false
        return
      }
    }
  }
  const match = value.match(/^(\d+)x(\d+)$/)
  if (match) {
    customWidth.value = Number(match[1])
    customHeight.value = Number(match[2])
    customSizeEnabled.value = true
  }
}

function clearEditSource() {
  sourceResultId.value = null
  sourcePreviewUrl.value = ''
  inputImage.value = null
}

async function remove(task: LocalTask) {
  if (!window.confirm('确认删除这条图片记录？')) return
  try {
    await deleteImageGenerationTask(task.id)
    revokeTaskImageUrls(task)
    tasks.value = tasks.value.filter((item) => item.id !== task.id)
    total.value = Math.max(0, total.value - 1)
    appStore.showSuccess('图片记录已删除')
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '删除失败'))
  }
}

function openPreview(task: LocalTask, result: ImageGenerationResult) {
  preview.value = { task, result }
}

function closePreview() {
  preview.value = null
}

function formatDate(value: string) {
  if (!value) return ''
  return new Date(value).toLocaleString()
}

function statusLabel(status: string) {
  if (status === 'pending') return '生成中'
  if (status === 'failed') return '失败'
  return '已完成'
}

function statusClass(status: string) {
  if (status === 'pending') {
    return 'bg-blue-50 text-blue-700 dark:bg-blue-950/30 dark:text-blue-200'
  }
  if (status === 'failed') {
    return 'bg-red-50 text-red-700 dark:bg-red-950/30 dark:text-red-200'
  }
  return 'bg-green-50 text-green-700 dark:bg-green-950/30 dark:text-green-200'
}

function imageUrl(resultId: number) {
  return imageUrls.value[resultId] || ''
}

async function attachImageUrls(items: LocalTask[]) {
  const results = items.flatMap((task) => task.results)
  await Promise.all(results.map(async (result) => {
    if (imageUrls.value[result.id]) return
    try {
      const blob = await getImageGenerationFileBlob(result.id)
      imageUrls.value = {
        ...imageUrls.value,
        [result.id]: URL.createObjectURL(blob),
      }
    } catch (error) {
      console.warn('Failed to load image generation file', error)
    }
  }))
}

function revokeTaskImageUrls(task: LocalTask) {
  const next = { ...imageUrls.value }
  task.results.forEach((result) => {
    const url = next[result.id]
    if (url) URL.revokeObjectURL(url)
    delete next[result.id]
  })
  imageUrls.value = next
}

function revokeAllImageUrls() {
  Object.values(imageUrls.value).forEach((url) => URL.revokeObjectURL(url))
  imageUrls.value = {}
}

function schedulePendingRefresh() {
  if (pendingRefreshTimer) {
    window.clearTimeout(pendingRefreshTimer)
    pendingRefreshTimer = null
  }
  if (pendingCount.value === 0) return
  pendingRefreshTimer = window.setTimeout(async () => {
    pendingRefreshTimer = null
    await refreshPendingTasks()
  }, pendingRefreshIntervalMs)
}

async function refreshPendingTasks() {
  const pendingTasks = tasks.value.filter((task) => task.status === 'pending' && task.id > 0)
  if (pendingTasks.length === 0) {
    schedulePendingRefresh()
    return
  }
  await Promise.all(pendingTasks.map(async (task) => {
    try {
      const latest = normalizeTask(await getImageGenerationTask(task.id))
      replaceTask(latest)
      if (latest.results.length > 0) {
        await attachImageUrls([latest])
      }
    } catch (error) {
      console.warn('Failed to refresh pending image task', error)
    }
  }))
  schedulePendingRefresh()
}

function replaceTask(task: LocalTask) {
  let replaced = false
  tasks.value = tasks.value.map((item) => {
    if (!item.local_id && item.id === task.id) {
      replaced = true
      return task
    }
    return item
  })
  if (!replaced) tasks.value = [task, ...tasks.value]
}

function downloadResult(result: ImageGenerationResult) {
  const url = imageUrls.value[result.id]
  if (!url) {
    appStore.showError('图片文件还在加载，请稍后重试')
    return
  }
  const link = document.createElement('a')
  link.href = url
  link.download = `image-${result.id}${imageExt(result.mime_type)}`
  document.body.appendChild(link)
  link.click()
  link.remove()
}

function imageExt(mimeType: string) {
  if (mimeType === 'image/jpeg') return '.jpg'
  if (mimeType === 'image/webp') return '.webp'
  return '.png'
}

onMounted(async () => {
  await loadBootstrap()
  await loadHistory()
})

onBeforeUnmount(() => {
  if (pendingRefreshTimer) window.clearTimeout(pendingRefreshTimer)
  revokeAllImageUrls()
})
</script>

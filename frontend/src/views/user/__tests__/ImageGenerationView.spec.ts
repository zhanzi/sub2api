import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import ImageGenerationView from '../ImageGenerationView.vue'

const {
  createImageEdit,
  createImageGeneration,
  deleteImageGenerationTask,
  getImageGenerationFileBlob,
  getImageGenerationBootstrap,
  getImageGenerationTask,
  listImageGenerationTasks,
  saveImageGenerationPreference,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  createImageEdit: vi.fn(),
  createImageGeneration: vi.fn(),
  deleteImageGenerationTask: vi.fn(),
  getImageGenerationFileBlob: vi.fn(),
  getImageGenerationBootstrap: vi.fn(),
  getImageGenerationTask: vi.fn(),
  listImageGenerationTasks: vi.fn(),
  saveImageGenerationPreference: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/imageGeneration', () => ({
  createImageEdit,
  createImageGeneration,
  deleteImageGenerationTask,
  getImageGenerationFileBlob,
  getImageGenerationBootstrap,
  getImageGenerationTask,
  listImageGenerationTasks,
  saveImageGenerationPreference,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
  }),
}))

const AppLayoutStub = { template: '<div><slot /></div>' }
const IconStub = { props: ['name'], template: '<i :data-icon="name" />' }

const imageTask = {
  id: 42,
  mode: 'generation',
  status: 'completed',
  model: 'gpt-image-2',
  prompt: '一间明亮的未来工作室',
  size: '2048x2048',
  expires_at: '2026-06-09T08:00:00Z',
  created_at: '2026-06-02T08:00:00Z',
  results: [
    {
      id: 101,
      index: 0,
      mime_type: 'image/png',
      size_bytes: 1536,
      url: '',
    },
    {
      id: 102,
      index: 1,
      mime_type: 'image/png',
      size_bytes: 2_097_152,
      url: '',
    },
  ],
}

function mountView() {
  return mount(ImageGenerationView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Icon: IconStub,
      },
    },
  })
}

describe('ImageGenerationView gallery and preview', () => {
  beforeEach(() => {
    createImageEdit.mockReset()
    createImageGeneration.mockReset()
    deleteImageGenerationTask.mockReset()
    getImageGenerationFileBlob.mockReset()
    getImageGenerationBootstrap.mockReset()
    getImageGenerationTask.mockReset()
    listImageGenerationTasks.mockReset()
    saveImageGenerationPreference.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    getImageGenerationBootstrap.mockResolvedValue({
      enabled: true,
      default_group_id: 1,
      default_model: 'gpt-image-2',
      retention_days: 30,
      key_selection: 'system',
      available_api_keys: [],
    })
    listImageGenerationTasks.mockResolvedValue({
      items: [imageTask],
      total: 1,
      page: 1,
      page_size: 24,
      pages: 1,
    })
    getImageGenerationFileBlob.mockResolvedValue(new Blob(['image'], { type: 'image/png' }))

    let blobId = 0
    vi.stubGlobal('URL', {
      createObjectURL: vi.fn(() => `blob://image-${++blobId}`),
      revokeObjectURL: vi.fn(),
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('uses denser desktop gallery columns and shows richer preview details', async () => {
    const wrapper = mountView()

    await flushPromises()
    await nextTick()

    expect(wrapper.html()).toContain('2xl:grid-cols-4')
    expect(wrapper.html()).toContain('min-[1800px]:grid-cols-5')

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.openPreview(setupState.tasks[0], setupState.tasks[0].results[1])
    await nextTick()

    const text = wrapper.text()
    expect(text).toContain('第 2 / 2 张')
    expect(text).toContain('2 MB')
    expect(text).toContain('复用提示词')
    expect(wrapper.findAll('button[aria-label^="预览第"]')).toHaveLength(2)
  })
})

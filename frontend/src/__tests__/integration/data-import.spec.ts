import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'

const showError = vi.fn()
const showSuccess = vi.fn()
const showWarning = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData: vi.fn()
    }
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

const mountModal = () =>
  mount(ImportDataModal, {
    props: { show: true },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
      }
    }
  })

const makeJsonFile = (name: string, content: string, type = 'application/json') => {
  const file = new File([content], name, { type })
  Object.defineProperty(file, 'text', {
    value: () => Promise.resolve(content)
  })
  return file
}

const setInputFiles = (element: Element, files: File[]) => {
  Object.defineProperty(element, 'files', {
    value: files,
    configurable: true
  })
}

const makeDataPayload = (
  accountName: string,
  options?: {
    proxies?: Array<Record<string, unknown>>
    skippedShadows?: number
  }
) => ({
  exported_at: '2026-07-05T00:00:00Z',
  proxies: options?.proxies || [],
  accounts: [{ name: accountName }],
  skipped_shadows: options?.skippedShadows
})

const selectPasteMode = async (wrapper: ReturnType<typeof mountModal>) => {
  await wrapper.get('[data-testid="import-mode-paste"]').trigger('click')
  return wrapper.get('[data-testid="import-json-textarea"]')
}

describe('ImportDataModal', () => {
  beforeEach(async () => {
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockReset()
  })

  it('未选择文件时提示错误', async () => {
    const wrapper = mountModal()

    await wrapper.find('form').trigger('submit')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')
  })

  it('无效 JSON 时按文件名提示解析失败', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [makeJsonFile('data.json', 'invalid json')])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining('admin.accounts.dataImportParseFailedFile')
    )
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('data.json'))
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('不是导出数据的 JSON 按文件名拒绝', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [makeJsonFile('random.json', JSON.stringify({ name: 'test' }))])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining('admin.accounts.dataImportInvalidFile')
    )
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('random.json'))
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('无有效 JSON 的选择不清空已有选择', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })

    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')

    const valid = makeJsonFile(
      'valid.json',
      JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' }] })
    )
    setInputFiles(input.element, [valid])
    await input.trigger('change')

    setInputFiles(input.element, [new File(['hello'], 'notes.txt', { type: 'text/plain' })])
    await input.trigger('change')
    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectFile')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [{ name: 'a' }]
      }),
      skip_default_group_bind: true
    })
  })

  it('merges multiple selected JSON files before importing', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    })

    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const first = makeJsonFile(
      'first.json',
      JSON.stringify({ exported_at: '2026-07-05T00:00:00Z', proxies: [], accounts: [{ name: 'a' }] })
    )
    const second = makeJsonFile(
      'second.json',
      JSON.stringify({
        exported_at: '2026-07-05T00:00:01Z',
        proxies: [{ proxy_key: 'p' }],
        accounts: [{ name: 'b' }]
      })
    )
    setInputFiles(input.element, [first, second])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        proxies: [{ proxy_key: 'p' }],
        accounts: [{ name: 'a' }, { name: 'b' }]
      }),
      skip_default_group_bind: true
    })
    expect(showSuccess).toHaveBeenCalledWith(
      expect.stringContaining('admin.accounts.dataImportSuccess')
    )
  })

  it('imports one complete export object pasted as JSON', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })

    const wrapper = mountModal()
    const textarea = await selectPasteMode(wrapper)
    await textarea.setValue(JSON.stringify(makeDataPayload('pasted-account')))

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [{ name: 'pasted-account' }],
        proxies: []
      }),
      skip_default_group_bind: true
    })
  })

  it('renders the JSON placeholder without passing it through i18n', async () => {
    const wrapper = mountModal()
    const textarea = await selectPasteMode(wrapper)

    expect(textarea.attributes('placeholder')).toContain('"exported_at": "..."')
    expect(textarea.attributes('placeholder')).toContain('"accounts": []')
  })

  it('merges an array of complete export objects pasted as JSON', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 1,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 2,
      account_failed: 0
    })

    const wrapper = mountModal()
    const textarea = await selectPasteMode(wrapper)
    await textarea.setValue(
      JSON.stringify([
        makeDataPayload('first', { skippedShadows: 1 }),
        makeDataPayload('second', {
          proxies: [{ proxy_key: 'proxy-1' }],
          skippedShadows: 2
        })
      ])
    )

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [{ name: 'first' }, { name: 'second' }],
        proxies: [{ proxy_key: 'proxy-1' }],
        skipped_shadows: 3
      }),
      skip_default_group_bind: true
    })
  })

  it('rejects empty pasted content before sending a request', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()
    await selectPasteMode(wrapper)

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportPasteRequired')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('rejects invalid pasted JSON before sending a request', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()
    const textarea = await selectPasteMode(wrapper)
    await textarea.setValue('{ invalid json')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportPasteParseFailed')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('rejects an empty pasted JSON array', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()
    const textarea = await selectPasteMode(wrapper)
    await textarea.setValue('[]')

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportPasteEmptyArray')
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('rejects a pasted array of account objects and reports the item index', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()
    const textarea = await selectPasteMode(wrapper)
    await textarea.setValue(
      JSON.stringify([
        {
          name: 'account-only',
          platform: 'openai',
          type: 'oauth',
          credentials: {}
        }
      ])
    )

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining('admin.accounts.dataImportPasteInvalidItem')
    )
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"index":1'))
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('reports the invalid item index in a pasted export array', async () => {
    const { adminAPI } = await import('@/api/admin')
    const wrapper = mountModal()
    const textarea = await selectPasteMode(wrapper)
    await textarea.setValue(JSON.stringify([makeDataPayload('valid'), { accounts: [] }]))

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(expect.stringContaining('"index":2'))
    expect(adminAPI.accounts.importData).not.toHaveBeenCalled()
  })

  it('submits only the active input mode', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 0
    })

    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [
      makeJsonFile('file.json', JSON.stringify(makeDataPayload('file-account')))
    ])
    await input.trigger('change')

    const textarea = await selectPasteMode(wrapper)
    await textarea.setValue(JSON.stringify(makeDataPayload('paste-account')))
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(adminAPI.accounts.importData).toHaveBeenCalledWith({
      data: expect.objectContaining({
        accounts: [{ name: 'paste-account' }]
      }),
      skip_default_group_bind: true
    })
  })

  it('部分成功时关闭弹窗仍通知父组件刷新', async () => {
    const { adminAPI } = await import('@/api/admin')
    vi.mocked(adminAPI.accounts.importData).mockResolvedValue({
      proxy_created: 0,
      proxy_reused: 0,
      proxy_failed: 0,
      account_created: 1,
      account_failed: 1
    })

    const wrapper = mountModal()
    const input = wrapper.find('input[type="file"]')
    setInputFiles(input.element, [
      makeJsonFile(
        'mixed.json',
        JSON.stringify({
          exported_at: '2026-07-05T00:00:00Z',
          proxies: [],
          accounts: [{ name: 'a' }, { name: 'b' }]
        })
      )
    ])

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith(
      expect.stringContaining('admin.accounts.dataImportCompletedWithErrors')
    )
    expect(wrapper.emitted('imported')).toBeUndefined()

    // 第二个 btn-secondary 是 footer 的取消按钮(第一个是选择文件)
    await wrapper.findAll('button.btn-secondary')[1]!.trigger('click')

    expect(wrapper.emitted('imported')).toHaveLength(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})

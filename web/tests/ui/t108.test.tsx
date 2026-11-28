import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router-dom'
import Logs from '../../src/pages/Logs'
import RetryTimeline, { lastAttempt } from '../../src/components/RetryTimeline'
import { fmtTime } from '../../src/lib/format'
import type { RequestLog, RetryAttempt } from '../../src/api/types'

const state = vi.hoisted(() => ({
  logs: [] as unknown[],
  lastFilters: null as Record<string, unknown> | null,
  lastInterval: undefined as unknown,
  refetch: vi.fn(async () => {}),
}))

vi.mock('../../src/api/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../src/api/hooks')>()
  return {
    ...actual,
    useChannels: () => ({ data: { items: [] }, isPending: false }),
    useTokens: () => ({ data: { items: [] }, isPending: false }),
    useLogs: (filters: Record<string, unknown>, interval: number | false) => {
      state.lastFilters = filters
      state.lastInterval = interval
      return { data: { items: state.logs, total: state.logs.length }, isPending: false, isFetching: false, refetch: state.refetch }
    },
    useLogModels: () => ({ data: { items: [{ model: 'gpt-4o', requests: 2 }, { model: 'custom-model', requests: 1 }] }, isPending: false }),
  }
})

const successLog: RequestLog = {
  id: 1, created_at: '2026-11-27T02:00:00Z', token_id: 5, token_name: 'demo-token', source: 'api',
  channel_id: 2, channel_name: 'OpenAI 主渠道', key_id: 12, model: 'gpt-4o', upstream_model: 'gpt-4o-2024-08-06',
  stream: true, status_code: 200, error_type: '', error_message: '', retry_trace: null,
  first_token_ms: 320, total_ms: 1200, prompt_tokens: 100, completion_tokens: 50, total_tokens: 150,
  cost_micros: 2100, cost_status: 'priced', cost: 0.0021, price_source: 'catalog', ip: '127.0.0.1', user_agent: 'curl/8.0',
}

// 全部尝试失败：没有命中渠道，只有轨迹能说明"最后死在哪个渠道"
const failedTrace: RetryAttempt[] = [
  { channel_id: 2, channel_name: 'OpenAI 主渠道', key_id: 12, attempt: 1, status_code: 429, error_type: 'rate_limited', error_message: '{"error":{"code":"1305","message":"Rate limit reached"}}', elapsed_ms: 320 },
  { channel_id: 3, channel_name: 'DeepSeek 备用', key_id: 7, attempt: 2, status_code: 503, error_type: 'transient', error_message: 'upstream connect error', elapsed_ms: 900 },
  { channel_id: 0, channel_name: '模型降级', key_id: 0, attempt: 3, error_type: 'budget_threshold', error_message: '令牌剩余额度低于阈值，按降级链切换模型', elapsed_ms: 0, reason: 'budget_threshold', from_model: 'gpt-4o', to_model: 'gpt-4o-mini' },
]
const failedLog: RequestLog = {
  ...successLog, id: 2, channel_id: 0, channel_name: '', key_id: 0, upstream_model: 'gpt-4o', status_code: 503,
  error_type: 'channel_exhausted', error_message: 'all upstream channels exhausted: {"error":{"code":"1305","message":"Rate limit reached"}}',
  retry_trace: failedTrace, first_token_ms: 0, total_ms: 1300, prompt_tokens: 0, completion_tokens: 0, total_tokens: 0,
  cost_micros: 0, cost_status: 'no_usage', cost: 0, price_source: '',
}
const unpricedLog: RequestLog = { ...successLog, id: 3, model: 'custom-model', upstream_model: 'custom-model', cost_status: 'unpriced', cost_micros: 0, cost: 0, price_source: '' }

function renderLogs() {
  return render(<MemoryRouter><Logs /></MemoryRouter>)
}

const dataRows = () => screen.getAllByRole('row').slice(1)

describe('Logs list and detail (T108)', () => {
  beforeEach(() => {
    state.logs = [successLog, failedLog, unpricedLog]
    state.lastFilters = null
    state.lastInterval = undefined
    state.refetch.mockClear()
  })
  afterEach(() => vi.unstubAllGlobals())

  it('shows the last attempted channel in the list for a failed request', () => {
    renderLogs()
    const [, failed] = dataRows()
    expect(within(failed).getByText('DeepSeek 备用')).toBeTruthy()
    expect(within(failed).getByText('503')).toBeTruthy()
    // 成功请求仍显示命中渠道
    expect(within(dataRows()[0]).getByText('OpenAI 主渠道')).toBeTruthy()
  })

  it('groups the detail without repeating summary facts and highlights the fallback boundary', async () => {
    renderLogs()
    fireEvent.click(screen.getByText('DeepSeek 备用'))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('503 失败')).toBeTruthy()
    for (const section of ['用量与成本', '错误', '重试轨迹', '请求上下文']) {
      expect(within(dialog).getByText(section)).toBeTruthy()
    }
    // 摘要带已经回答的事实不再在下方分组里重复：时间 / 模型 / 渠道 / 耗时只出现一次
    expect(within(dialog).queryByText('请求')).toBeNull()
    expect(within(dialog).queryByText('路由与性能')).toBeNull()
    expect(within(dialog).getAllByText(fmtTime(failedLog.created_at)).length).toBe(1)
    expect(within(dialog).getAllByText('1.30s').length).toBe(1)
    // 对外模型只在摘要带出现一次（降级边界里的 from / to 是轨迹事实，不算重复）
    expect(within(dialog).getByText('gpt-4o', { selector: '[title="模型"]' })).toBeTruthy()
    // 请求上下文默认折叠
    expect(within(dialog).getByText('请求上下文').closest('[data-slot="collapsible"]')?.getAttribute('data-state')).toBe('closed')
    // 摘要带给出最后尝试的渠道并标注
    expect(within(dialog).getByText(/DeepSeek 备用（最后尝试）/)).toBeTruthy()
    // 降级边界：标题 + 本地化原因，不回显后端写入的中文展示文案
    expect(within(dialog).getByText('模型降级')).toBeTruthy()
    expect(within(dialog).getByText('预算降级')).toBeTruthy()
    expect(within(dialog).queryByText(/令牌剩余额度低于阈值/)).toBeNull()
    // 未计价 / 无 usage 给出原因
    expect(within(dialog).getByText(/上游没有返回 usage/)).toBeTruthy()
  })

  it('filters by channel from the detail and closes the drawer', async () => {
    renderLogs()
    fireEvent.click(dataRows()[0])
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getByRole('button', { name: '按此渠道筛选' }))
    expect(state.lastFilters).toMatchObject({ channel_id: '2', page: 1 })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('opens a row with the keyboard', async () => {
    renderLogs()
    const row = dataRows()[0]
    row.focus()
    fireEvent.keyDown(row, { key: 'Enter' })
    expect(await screen.findByRole('dialog')).toBeTruthy()
  })

  it('polls only after auto refresh is switched on and keeps manual refresh explicit', async () => {
    renderLogs()
    expect(state.lastInterval).toBe(false)
    fireEvent.click(screen.getByRole('switch', { name: '自动刷新' }))
    await waitFor(() => expect(state.lastInterval).toBe(5000))
    fireEvent.click(screen.getByRole('button', { name: '刷新' }))
    await waitFor(() => expect(state.refetch).toHaveBeenCalledTimes(1))
  })

  it('explains an unpriced request and links to the price settings', async () => {
    renderLogs()
    fireEvent.click(screen.getByText('custom-model'))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText(/没有生效价格/)).toBeTruthy()
    expect(within(dialog).getByRole('link', { name: '去设置价格' }).getAttribute('href')).toBe('/usage')
  })

  it('offers logged models as options instead of a free-text input', () => {
    renderLogs()
    const trigger = screen.getByRole('combobox', { name: '模型' })
    expect(screen.queryByPlaceholderText('精确匹配，如 gpt-4o')).toBeNull()
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    expect(screen.getByRole('option', { name: '全部模型' })).toBeTruthy()
    expect(screen.getByRole('option', { name: 'gpt-4o（2 次）' })).toBeTruthy()
  })

  it('filters by status group and quick time range, and resets everything', async () => {
    renderLogs()
    const status = screen.getByRole('combobox', { name: '状态码' })
    fireEvent.keyDown(status, { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('option', { name: '失败（4xx / 5xx）' }), { key: 'Enter' })
    expect(state.lastFilters).toMatchObject({ status: 'error' })

    const range = screen.getByRole('combobox', { name: '时间范围' })
    expect(screen.queryByLabelText('日期范围')).toBeNull()
    fireEvent.keyDown(range, { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('option', { name: '今天' }), { key: 'Enter' })
    expect(state.lastFilters).toMatchObject({ status: 'error', end: undefined })
    const localMidnight = new Date()
    localMidnight.setHours(0, 0, 0, 0)
    expect(state.lastFilters?.start).toBe(localMidnight.toISOString())

    // 自定义才显示日期范围选择器
    fireEvent.keyDown(range, { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('option', { name: '自定义…' }), { key: 'Enter' })
    expect(screen.getByLabelText('日期范围')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: '重置' }))
    expect(state.lastFilters).toMatchObject({ status: undefined, start: undefined, end: undefined, page: 1 })
    expect(screen.getByRole('button', { name: '重置' }).hasAttribute('disabled')).toBe(true)
  })

  it('copies key fields through the shared CopyButton', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    renderLogs()
    fireEvent.click(dataRows()[0])
    const dialog = await screen.findByRole('dialog')
    fireEvent.click(within(dialog).getAllByRole('button', { name: '复制' })[0])
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('gpt-4o-2024-08-06'))
    // 图标形态把结果放在可访问名称上，读屏同样能听到
    expect(await within(dialog).findByRole('button', { name: '已复制' })).toBeTruthy()
  })
})

describe('RetryTimeline', () => {
  it('resolves the last upstream attempt and skips fallback boundaries', () => {
    expect(lastAttempt(failedTrace)?.channel_name).toBe('DeepSeek 备用')
    expect(lastAttempt([failedTrace[2]])).toBeNull()
    expect(lastAttempt([])).toBeNull()
    expect(lastAttempt(null)).toBeNull()
  })

  it('renders attempts with channel, key, HTTP code, elapsed and readable reason', () => {
    render(<RetryTimeline attempts={failedTrace} />)
    expect(screen.getByText('OpenAI 主渠道')).toBeTruthy()
    expect(screen.getByText('key #12')).toBeTruthy()
    expect(screen.getByText('HTTP 429')).toBeTruthy()
    expect(screen.getByText('320ms')).toBeTruthy()
    expect(screen.getByText('限流/配额')).toBeTruthy()
    expect(screen.getByText('上游 1305')).toBeTruthy()
    expect(screen.getByText('gpt-4o-mini')).toBeTruthy()
    expect(screen.getByText('预算降级')).toBeTruthy()
  })
})

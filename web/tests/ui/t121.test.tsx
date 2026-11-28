import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import AlertCard from '../../src/components/AlertCard'
import Channels from '../../src/pages/Channels'
import type { AlertSetting, Channel } from '../../src/api/types'
import { t, tf } from '../../src/lib/i18n'

const state = vi.hoisted(() => ({
  data: null as unknown,
  test: vi.fn(),
  save: vi.fn(),
  channels: null as unknown,
}))

vi.mock('../../src/api/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../src/api/hooks')>()
  return {
    ...actual,
    useAlerts: () => ({ data: state.data, isPending: false, isError: false, error: null }),
    useSaveAlerts: () => ({ mutate: state.save, isPending: false, isError: false, error: null }),
    useTestAlert: () => ({ mutate: state.test, isPending: false, isError: false, error: null }),
    useChannels: () => ({ data: state.channels, isPending: false, isError: false, error: null }),
    useUsagePrices: () => ({ data: undefined, isPending: false, isError: false, error: null }),
  }
})

const setting: AlertSetting = { id: 1, enabled: false, preset: 'generic', url: '', events: [], updated_at: '2026-11-01T00:00:00Z' }
const EVENTS = ['channel_down', 'key_disabled']
/** 服务端载荷：setting.events 是已订阅事件，顶层 events 是可订阅事件全集 */
const payload = (overrides: Partial<AlertSetting> = {}) => ({
  setting: { ...setting, events: EVENTS, ...overrides },
  events: EVENTS,
  records: [],
})

describe('AlertCard（T121 ②）', () => {
  it('测试发送按当前草稿发，记录刷新后不清掉未保存的编辑', () => {
    state.data = payload()
    state.test.mockClear()
    const { rerender } = render(<AlertCard />)
    fireEvent.change(screen.getByLabelText(t('alerts.url')), { target: { value: 'https://draft.example/hook' } })
    fireEvent.click(screen.getByLabelText(t('alerts.event.channel_down')))
    fireEvent.click(screen.getByRole('button', { name: t('alerts.test') }))
    expect(state.test).toHaveBeenCalledTimes(1)
    expect(state.test.mock.calls[0][0]).toEqual({ enabled: false, preset: 'generic', url: 'https://draft.example/hook', events: ['key_disabled'] })

    // 测试成功后 invalidateQueries 触发重取：服务端配置仍是旧的，草稿必须保留
    state.data = payload({ updated_at: '2026-11-02T00:00:00Z' })
    rerender(<AlertCard />)
    expect((screen.getByLabelText(t('alerts.url')) as HTMLInputElement).value).toBe('https://draft.example/hook')
    expect(screen.getByLabelText(t('alerts.event.channel_down')).getAttribute('aria-checked')).toBe('false')
  })

  it('保存成功后用服务端返回的配置回填草稿', () => {
    state.data = payload()
    state.save.mockReset()
    render(<AlertCard />)
    fireEvent.change(screen.getByLabelText(t('alerts.url')), { target: { value: 'https://saved.example/hook' } })
    fireEvent.click(screen.getByRole('button', { name: t('common.save') }))
    act(() => state.save.mock.calls[0][1].onSuccess({ setting: { ...setting, enabled: true, url: 'https://saved.example/hook' } }))
    expect((screen.getByLabelText(t('alerts.url')) as HTMLInputElement).value).toBe('https://saved.example/hook')
    expect(screen.getByLabelText(t('alerts.enable')).getAttribute('aria-checked')).toBe('true')
  })

  it('草稿测试的记录带「草稿配置」徽标', () => {
    state.data = {
      ...payload(),
      records: [
        { id: 2, created_at: '2026-11-28T00:00:00Z', event: 'test', preset: 'wecom', result: 'test', status_code: 200, error: '', message: 'test alert', draft: true },
        { id: 1, created_at: '2026-11-27T00:00:00Z', event: 'test', preset: 'generic', result: 'test', status_code: 200, error: '', message: 'test alert', draft: false },
      ],
    }
    render(<AlertCard />)
    const rows = screen.getAllByRole('row')
    expect(within(rows[1]).getByText(t('alerts.record.draft'))).toBeTruthy()
    expect(within(rows[2]).queryByText(t('alerts.record.draft'))).toBeNull()
  })
})

describe('渠道目录差异徽标（T121 ①）', () => {
  it('悬停徽标出现差异明细（新增模型名与同步时间）', async () => {
    const channel = {
      id: 3, name: 'fresh', type: 'openai-compatible', preset_id: '', base_url: 'http://127.0.0.1:9/v1',
      priority: 0, weight: 1, models: ['mock-alpha'], model_mapping: null, enabled: true, status: 'active',
      available_key_count: 1, last_probe_at: null, last_probe_result: '', last_probe_status_code: 0,
      last_probe_latency_ms: 0, last_probe_error_type: '', last_probe_error: '', next_probe_at: null,
      supports_responses: false, supports_anthropic: false, supports_gemini: false, timeout_config: null,
      catalog_diff: { checked_at: '2026-11-28T07:20:00Z', added: ['mock-beta'], removed: ['old-model'], upstream_count: 2 },
      key_count: 1, created_at: '2026-11-01T00:00:00Z', updated_at: '2026-11-01T00:00:00Z',
    } as Channel
    state.channels = { items: [channel] }
    render(<QueryClientProvider client={new QueryClient()}><Channels /></QueryClientProvider>)
    const badge = screen.getByText('+1 / −1')
    expect(badge.getAttribute('data-catalog-drift')).toBe('true')
    fireEvent.pointerMove(badge)
    expect(await screen.findByText(t('channels.catalog.driftTitle'))).toBeTruthy()
    expect(screen.getByText(tf('channels.catalog.added', { n: 1 }))).toBeTruthy()
    expect(screen.getByText('mock-beta')).toBeTruthy()
    expect(screen.getByText(tf('channels.catalog.removed', { n: 1 }))).toBeTruthy()
    expect(screen.getByText('old-model')).toBeTruthy()
    await waitFor(() => expect(screen.getByText(/上游共 2 个模型/)).toBeTruthy())
  })
})

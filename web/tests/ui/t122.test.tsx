import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import Channels from '../../src/pages/Channels'
import Logs from '../../src/pages/Logs'
import { t } from '../../src/lib/i18n'

const state = vi.hoisted(() => ({ logsFilters: null as Record<string, unknown> | null }))

vi.mock('../../src/api/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../src/api/hooks')>()
  return {
    ...actual,
    useChannels: () => ({ data: { items: [] }, isPending: false }),
    useTokens: () => ({ data: { items: [] }, isPending: false }),
    useUsagePrices: () => ({ data: undefined, isPending: false }),
    useChannel: () => ({ data: undefined, isPending: false }),
    useLogs: (filters: Record<string, unknown>) => {
      state.logsFilters = filters
      return { data: { items: [], total: 0 }, isPending: false, isFetching: false, refetch: async () => {} }
    },
    useLogModels: () => ({ data: { items: [] }, isPending: false }),
  }
})

function renderPage(page: React.ReactElement) {
  return render(<QueryClientProvider client={new QueryClient()}><MemoryRouter>{page}</MemoryRouter></QueryClientProvider>)
}

describe('T122 接口行为收口', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('渠道名称冲突：错误落在名称字段并聚焦，不再在弹窗顶部重复提示', async () => {
    const posts: string[] = []
    vi.stubGlobal('fetch', vi.fn(async (url: string) => {
      posts.push(url)
      return {
        ok: false, status: 409,
        json: async () => ({ ok: false, message: '渠道名称已被占用', code: 'channel_name_taken' }),
      } as Response
    }))
    renderPage(<Channels />)
    fireEvent.click(screen.getAllByRole('button', { name: t('page.channels.create') })[0])
    const dialog = await screen.findByRole('dialog')
    // 切到自定义接入才会出现名称字段
    fireEvent.keyDown(within(dialog).getByRole('combobox', { name: t('channels.form.preset') }), { key: 'ArrowDown' })
    fireEvent.keyDown(await screen.findByRole('option', { name: t('preset.custom.label') }), { key: 'Enter' })
    const name = within(dialog).getByLabelText(t('channels.form.name')) as HTMLInputElement
    fireEvent.change(name, { target: { value: 'dup' } })
    fireEvent.change(within(dialog).getByLabelText('Base URL'), { target: { value: 'https://example.com' } })
    fireEvent.click(within(dialog).getByRole('button', { name: t('common.save') }))

    expect(await within(dialog).findByText(t('error.channel_name_taken'))).toBeTruthy()
    expect(name.getAttribute('aria-invalid')).toBe('true')
    expect(document.activeElement).toBe(name)
    // 名称冲突已定位到字段：弹窗顶部不再叠一条同样的错误
    expect(within(dialog).queryByRole('alert')).toBeNull()
    await waitFor(() => expect(posts.some((url) => url.endsWith('/admin/api/channels'))).toBe(true))
  })

  it('其他保存失败（非名称冲突）仍在弹窗顶部提示', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false, status: 500,
      json: async () => ({ ok: false, message: '创建失败', code: 'channel_create_failed' }),
    } as Response)))
    renderPage(<Channels />)
    fireEvent.click(screen.getAllByRole('button', { name: t('page.channels.create') })[0])
    const dialog = await screen.findByRole('dialog')
    fireEvent.change(within(dialog).getByLabelText('Base URL'), { target: { value: 'https://example.com' } })
    fireEvent.click(within(dialog).getByRole('button', { name: t('common.save') }))
    expect(await within(dialog).findByRole('alert')).toBeTruthy()
    expect(within(dialog).getByRole('alert').textContent).toContain(t('error.channel_create_failed'))
    expect(within(dialog).getByRole('alert').textContent).not.toContain('名称重复')
  })

  it('日志自定义区间：两端互相 min/max 约束，越界时另一端自动拉齐', () => {
    renderPage(<Logs />)
    const range = screen.getByRole('combobox', { name: t('logs.filter.range') })
    fireEvent.keyDown(range, { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('option', { name: t('logs.filter.rangeCustom') }), { key: 'Enter' })
    const start = screen.getByLabelText(t('logs.filter.start')) as HTMLInputElement
    const end = screen.getByLabelText(t('logs.filter.end')) as HTMLInputElement
    expect(start.type).toBe('date')
    expect(start.max).toBe('')
    expect(end.min).toBe('')

    fireEvent.change(end, { target: { value: '2026-11-10' } })
    expect(start.max).toBe('2026-11-10')
    // 开始晚于结束：结束日期被拉到同一天，而不是发出一个倒置区间
    fireEvent.change(start, { target: { value: '2026-11-12' } })
    expect(end.value).toBe('2026-11-12')
    expect(end.min).toBe('2026-11-12')
    expect(state.logsFilters?.start).toBe(new Date('2026-11-12T00:00:00').toISOString())
    // 反向越界同理
    fireEvent.change(end, { target: { value: '2026-11-08' } })
    expect(start.value).toBe('2026-11-08')
    expect(start.max).toBe('2026-11-08')
  })
})

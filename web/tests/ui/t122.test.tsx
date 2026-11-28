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

  it('日志自定义区间：范围日历一次选定起止，先点晚的日子再点早的日子也只会得到正序区间', () => {
    renderPage(<Logs />)
    const range = screen.getByRole('combobox', { name: t('logs.filter.range') })
    fireEvent.keyDown(range, { key: 'ArrowDown' })
    fireEvent.keyDown(screen.getByRole('option', { name: t('logs.filter.rangeCustom') }), { key: 'Enter' })
    const trigger = screen.getByLabelText(t('logs.filter.dateRange'))
    expect(state.logsFilters).toMatchObject({ start: undefined, end: undefined })
    const localISO = (day: number, endOfDay = false) => new Date(new Date().getFullYear(), new Date().getMonth(), day, endOfDay ? 23 : 0, endOfDay ? 59 : 0, endOfDay ? 59 : 0, endOfDay ? 999 : 0).toISOString()

    fireEvent.click(trigger)
    const popover = screen.getByRole('dialog')
    // 本月 10 日 / 20 日：日历用 data-day（zh-CN 本地日期）标记每一天
    const day = (n: number) => popover.querySelector(`[data-day="${new Date(new Date().getFullYear(), new Date().getMonth(), n).toLocaleDateString('zh-CN')}"]`) as HTMLButtonElement
    // 首次点选就是单日区间，立刻回写；弹层保持展开，方便再点一天扩展
    fireEvent.click(day(20))
    expect(state.logsFilters?.start).toBe(localISO(20))
    expect(state.logsFilters?.end).toBe(localISO(20, true))
    // 点在起点之前：得到 10–20 的正序区间而不是倒置区间，弹层随即收起
    fireEvent.click(day(10))
    expect(state.logsFilters?.start).toBe(localISO(10))
    expect(state.logsFilters?.end).toBe(localISO(20, true))
    expect(screen.queryByRole('dialog')).toBeNull()

    // 清除回到无区间
    fireEvent.click(trigger)
    fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: /清除/ }))
    expect(state.logsFilters).toMatchObject({ start: undefined, end: undefined })
  })
})

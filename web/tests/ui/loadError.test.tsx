import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { LoadError } from '../../src/components/ui'
import { ApiError } from '../../src/api/client'
import Channels from '../../src/pages/Channels'
import Dashboard from '../../src/pages/Dashboard'
import Logs from '../../src/pages/Logs'
import { t } from '../../src/lib/i18n'

const state = vi.hoisted(() => ({
  channels: {} as Record<string, unknown>,
  tokens: {} as Record<string, unknown>,
  stats: {} as Record<string, unknown>,
  logs: {} as Record<string, unknown>,
}))

vi.mock('../../src/api/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../src/api/hooks')>()
  return {
    ...actual,
    useChannels: () => state.channels,
    useTokens: () => state.tokens,
    useLogsStats: () => state.stats,
    useUpdateStatus: () => ({ isPending: false, data: undefined }),
    useLogs: () => state.logs,
    useLogModels: () => ({ data: { items: [] }, isPending: false }),
  }
})

const refetch = vi.fn(async () => {})
const failed = { isError: true, error: new ApiError('数据库不可用', 500), isPending: false, refetch }

function renderPage(page: ReactElement) {
  return render(<QueryClientProvider client={new QueryClient()}><MemoryRouter>{page}</MemoryRouter></QueryClientProvider>)
}

describe('加载失败、空数据、加载中三态可区分', () => {
  beforeEach(() => {
    state.channels = {}
    state.tokens = {}
    state.stats = {}
    state.logs = {}
    refetch.mockClear()
  })

  it('LoadError 用 alert 播报原因，重试按钮回调', () => {
    render(<LoadError error={new ApiError('数据库不可用', 500)} onRetry={refetch} />)
    const alert = screen.getByRole('alert')
    expect(alert.textContent).toContain(t('loadFailed.list'))
    expect(alert.textContent).toContain('数据库不可用')
    fireEvent.click(screen.getByRole('button', { name: t('common.retry') }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })

  it('渠道接口失败：显示错误态，不渲染空表也不渲染「还没有渠道」', () => {
    state.channels = failed
    renderPage(<Channels />)
    expect(screen.getByRole('alert').textContent).toContain('数据库不可用')
    expect(screen.queryByRole('table')).toBeNull()
    expect(screen.queryByText(t('empty.channels.title'))).toBeNull()
  })

  it('网络层失败显示人话，而不是浏览器原生的 Failed to fetch', () => {
    render(<LoadError error={new TypeError('Failed to fetch')} />)
    expect(screen.getByRole('alert').textContent).toContain(t('session.loadFailed'))
    expect(screen.getByRole('alert').textContent).not.toContain('Failed to fetch')
  })

  it('渠道接口成功但为空：显示空状态，没有错误态', () => {
    state.channels = { data: { items: [] }, isPending: false }
    renderPage(<Channels />)
    expect(screen.getByText(t('empty.channels.title'))).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByRole('table')).toBeNull()
  })

  it('渠道接口加载中：显示骨架，不提前下「空」的结论', () => {
    state.channels = { isPending: true }
    renderPage(<Channels />)
    expect(screen.getByRole('status')).toBeTruthy()
    expect(screen.queryByText(t('empty.channels.title'))).toBeNull()
  })

  it('概览：渠道接口失败时不进入「欢迎使用」引导态', () => {
    state.channels = failed
    state.tokens = { data: { items: [] }, isPending: false }
    state.stats = { data: undefined, isPending: false }
    renderPage(<Dashboard />)
    expect(screen.getByRole('alert').textContent).toContain('数据库不可用')
    expect(screen.queryByText(t('dashboard.welcome.title'))).toBeNull()
  })

  it('概览：令牌仍在加载时不闪现「零渠道」引导态', () => {
    state.channels = { data: { items: [] }, isPending: false }
    state.tokens = { isPending: true }
    state.stats = { data: undefined, isPending: false }
    renderPage(<Dashboard />)
    expect(screen.queryByText(t('dashboard.welcome.title'))).toBeNull()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('日志首次加载失败显示错误态；带旧数据的刷新失败保留表格', () => {
    state.channels = { data: { items: [] }, isPending: false }
    state.tokens = { data: { items: [] }, isPending: false }
    state.logs = failed
    const first = renderPage(<Logs />)
    expect(screen.getByRole('alert').textContent).toContain('数据库不可用')
    expect(screen.queryByRole('table')).toBeNull()
    first.unmount()

    state.logs = { ...failed, data: { items: [{ id: 1, created_at: '2026-11-27T00:00:00Z', status_code: 200, model: 'mock-alpha', retry_trace: null }], total: 1 } }
    renderPage(<Logs />)
    expect(screen.getByRole('table')).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
  })
})

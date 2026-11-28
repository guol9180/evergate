import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import AuditLogCard from '../../src/components/AuditLogCard'
import Settings from '../../src/pages/Settings'
import { t } from '../../src/lib/i18n'
import type { AuditLog, SystemSettings } from '../../src/api/types'

const state = vi.hoisted(() => ({
  auditCalls: [] as Array<{ page: number; pageSize: number; action?: string }>,
  auditItems: [] as Array<Record<string, unknown>>,
  saved: null as Record<string, unknown> | null,
  settings: {} as Record<string, unknown>,
}))

vi.mock('../../src/api/hooks', () => ({
  useAuditLogs: (page: number, pageSize: number, action?: string) => {
    state.auditCalls.push({ page, pageSize, action })
    return { data: { items: state.auditItems, total: state.auditItems.length }, isPending: false, isError: false, refetch: async () => {} }
  },
  useSystemSettings: () => ({
    data: {
      settings: state.settings,
      paths: { config_path: '', data_dir: '', config_source: 'default' },
      scope: { runtime_source: '', hot_update_enabled: true },
    },
    isPending: false,
    isError: false,
  }),
  useUpdateSystemSettings: () => ({
    mutate: (values: Record<string, unknown>, options?: { onSuccess?: () => void }) => {
      state.saved = values
      options?.onSuccess?.()
    },
    isPending: false,
  }),
}))

// 设置页下方的卡片各自有数据依赖，这里只验证系统设置表单里的开关
vi.mock('../../src/components/ModelFallbackCard', () => ({ default: () => null }))
vi.mock('../../src/components/ConfigTransferCard', () => ({ default: () => null }))
vi.mock('../../src/components/BackupCard', () => ({ default: () => null }))
vi.mock('../../src/components/AlertCard', () => ({ default: () => null }))

const baseSettings: SystemSettings = {
  connect_timeout_ms: 5000,
  first_token_timeout_ms: 30000,
  overall_timeout_ms: 300000,
  retry_channels: 3,
  retry_keys_per_channel: 2,
  retry_total_ms: 60000,
  cooldown_fail_threshold: 3,
  cooldown_base_ms: 1000,
  cooldown_max_ms: 300000,
  log_retention_days: 14,
  health_probe_interval_ms: 300000,
  health_probe_concurrency: 2,
  health_probe_timeout_ms: 10000,
  log_unknown_tokens: true,
  source: 'database',
  initialized_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
}

const auditRows: AuditLog[] = [
  { id: 1, created_at: '2026-11-28T00:00:00Z', username: 'admin', action: 'backup.create', target: '', summary: '', ip: '127.0.0.1' },
  { id: 2, created_at: '2026-11-28T00:00:00Z', username: 'admin', action: 'price.apply_preset', target: '#1', summary: '', ip: '127.0.0.1' },
  { id: 3, created_at: '2026-11-28T00:00:00Z', username: 'admin', action: 'alerts.update', target: '', summary: '', ip: '127.0.0.1' },
]

function renderPage(page: React.ReactElement) {
  return render(<QueryClientProvider client={new QueryClient()}><MemoryRouter>{page}</MemoryRouter></QueryClientProvider>)
}

describe('T124 会话与审计加固', () => {
  it('审计卡片：新动作有文案，动作筛选透传到接口并重置页码', async () => {
    state.auditItems = auditRows
    state.auditCalls = []
    renderPage(<AuditLogCard />)
    expect(screen.getByText(t('audit.action.backup.create'))).toBeTruthy()
    expect(screen.getByText(t('audit.action.price.apply_preset'))).toBeTruthy()
    expect(screen.getByText(t('audit.action.alerts.update'))).toBeTruthy()
    expect(state.auditCalls.at(-1)).toMatchObject({ page: 1, action: undefined })

    const trigger = screen.getByRole('combobox', { name: t('audit.filter.action') })
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    fireEvent.keyDown(await screen.findByRole('option', { name: t('audit.action.backup.create') }), { key: 'Enter' })
    await waitFor(() => expect(state.auditCalls.at(-1)).toMatchObject({ page: 1, action: 'backup.create' }))
  })

  it('系统设置：未知令牌日志开关随表单提交', async () => {
    state.settings = baseSettings
    state.saved = null
    renderPage(<Settings />)
    fireEvent.click(screen.getByText(t('settings.maintenance')))
    const toggle = await screen.findByRole('switch', { name: t('settings.logUnknownTokens') })
    expect(toggle.getAttribute('aria-checked')).toBe('true')
    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-checked')).toBe('false')
    fireEvent.click(screen.getByRole('button', { name: t('settings.save') }))
    await waitFor(() => expect(state.saved).not.toBeNull())
    expect(state.saved?.log_unknown_tokens).toBe(false)
    expect(state.saved?.log_retention_days).toBe(14)
  })
})

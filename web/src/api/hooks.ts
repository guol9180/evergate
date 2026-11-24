import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, jsonBody } from './client'
import { withBasePath } from '../lib/runtime'
import type { AdminInfo, AuditLog, Channel, ChannelDetail, ConfigImportResult, LogStats, LogsPage, ModelCatalogItem, ModelFallback, ModelPreview, SystemSettings, SystemSettingsResponse, SystemSettingsValues, TestAllItem, TestResult, Token, UpdateJob, UpdateProgress, UpdateStatus, UsagePrices, UsageReport } from './types'

// ---------- 会话 ----------
export function useMe() {
  return useQuery({
    queryKey: ['me'],
    queryFn: () => api<AdminInfo>('/admin/api/auth/me'),
    retry: false,
    staleTime: 60_000,
  })
}

export function useLogin() {
  return useMutation({
    mutationFn: (data: { username: string; password: string }) =>
      api<AdminInfo>('/admin/api/auth/login', jsonBody(data)),
  })
}

export function useLogout() {
  return useMutation({ mutationFn: () => api<{ logout: boolean }>('/admin/api/auth/logout', { method: 'POST' }) })
}

export function useUpdateStatus(enabled = true) {
  return useQuery({
    queryKey: ['system-update'],
    queryFn: () => api<UpdateStatus>('/admin/api/system/update'),
    enabled,
    retry: false,
    staleTime: 5 * 60_000,
    refetchInterval: 30 * 60_000,
  })
}

export function useUpdateProgress(enabled: boolean) {
  return useQuery({
    queryKey: ['system-update-progress'],
    queryFn: () => api<UpdateProgress>('/admin/api/system/update/progress'),
    enabled,
    retry: false,
    staleTime: 0,
    refetchInterval: enabled ? 900 : false,
  })
}

export function useApplyUpdate() {
  return useMutation({
    mutationFn: (data: { version?: string; source?: string }) =>
      api<UpdateJob>('/admin/api/system/update/apply', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
  })
}

export function useCancelUpdate() {
  return useMutation({
    mutationFn: () => api<{ cancelled: boolean }>('/admin/api/system/update/cancel', { method: 'POST' }),
  })
}

// ---------- 系统设置 ----------
export function useSystemSettings() {
  return useQuery({
    queryKey: ['system-settings'],
    queryFn: () => api<SystemSettingsResponse>('/admin/api/system/settings'),
  })
}

export function useUpdateSystemSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: SystemSettingsValues) =>
      api<{ settings: SystemSettings }>('/admin/api/system/settings', { method: 'PUT', body: JSON.stringify(data) }),
    onSuccess: (data) => qc.setQueryData<SystemSettingsResponse>(['system-settings'], (current) =>
      current ? { ...current, settings: data.settings } : current,
    ),
  })
}

export function useModelFallbacks() {
  return useQuery({
    queryKey: ['model-fallbacks'],
    queryFn: () => api<{ items: ModelFallback[] }>('/admin/api/model-fallbacks'),
  })
}

export function useSaveModelFallbacks() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (items: ModelFallback[]) =>
      api<{ items: ModelFallback[] }>('/admin/api/model-fallbacks', {
        method: 'PUT',
        body: JSON.stringify({ items }),
      }),
    onSuccess: (data) => qc.setQueryData<{ items: ModelFallback[] }>(['model-fallbacks'], data),
  })
}

export function useChangePassword() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { old_password: string; new_password: string }) =>
      api<{ changed: boolean }>('/admin/api/auth/password', { method: 'PUT', body: JSON.stringify(data) }),
    onSuccess: () => {
      // 路由守卫读取 ['me'] 缓存；必须同步清除强制改密标记，否则成功后会被旧缓存弹回改密页。
      qc.setQueryData<AdminInfo>(['me'], (current) =>
        current ? { ...current, must_change_password: false } : current,
      )
    },
  })
}

// ---------- 渠道 ----------
export function useChannels() {
  return useQuery({
    queryKey: ['channels'],
    queryFn: () => api<{ items: Channel[] }>('/admin/api/channels'),
  })
}

export function useChannel(id: number | null) {
  return useQuery({
    queryKey: ['channel', id],
    queryFn: () => api<ChannelDetail>(`/admin/api/channels/${id}`),
    enabled: id != null,
  })
}

export function useSaveChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { id?: number } & Record<string, unknown>) => {
      const { id, ...body } = data
      return id
        ? api<Channel>(`/admin/api/channels/${id}`, { method: 'PUT', body: JSON.stringify(body) })
        : api<Channel>('/admin/api/channels', jsonBody(body))
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['channels'] }),
  })
}

export function useDeleteChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api<{ deleted: boolean }>(`/admin/api/channels/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['channels'] }),
  })
}

export function useToggleChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { id: number; enabled: boolean }) =>
      api<{ enabled: boolean }>(`/admin/api/channels/${data.id}/toggle`, jsonBody({ enabled: data.enabled })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['channels'] }),
  })
}

export function useTestChannel() {
  return useMutation({
    mutationFn: (id: number) => api<TestResult>(`/admin/api/channels/${id}/test`, { method: 'POST' }),
  })
}

export function usePreviewChannelModels() {
  return useMutation({
    mutationFn: (id: number) => api<ModelPreview>(`/admin/api/channels/${id}/models/preview`, { method: 'POST' }),
  })
}

export function useModelCatalog() {
  return useQuery({
    queryKey: ['model-catalog'],
    queryFn: () => api<{ items: ModelCatalogItem[] }>('/admin/api/model-catalog'),
    staleTime: 30_000,
  })
}

// ---------- 凭据 ----------
export function useBatchAddKeys() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { channelId: number; keys: string }) =>
      api<{ added: number }>(`/admin/api/channels/${data.channelId}/keys`, jsonBody({ keys: data.keys })),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ['channels'] })
      qc.invalidateQueries({ queryKey: ['channel', vars.channelId] })
    },
  })
}

export function useUpdateKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { id: number; enabled?: boolean; weight?: number }) =>
      api<{ updated: boolean }>(`/admin/api/keys/${data.id}`, {
        method: 'PUT',
        body: JSON.stringify({ enabled: data.enabled, weight: data.weight }),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['channel'] }),
  })
}

export function useDeleteKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { id: number; channelId: number }) =>
      api<{ deleted: boolean }>(`/admin/api/keys/${data.id}`, { method: 'DELETE' }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ['channel', vars.channelId] })
      qc.invalidateQueries({ queryKey: ['channels'] })
    },
  })
}

// ---------- 令牌 ----------
export function useTokens() {
  return useQuery({
    queryKey: ['tokens'],
    queryFn: () => api<{ items: Token[] }>('/admin/api/tokens'),
  })
}

export function useCreateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: TokenGovernanceInput) => api<{ token: Token; key: string }>('/admin/api/tokens', jsonBody(data)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tokens'] }),
  })
}

export interface TokenGovernanceInput {
  name: string
  quota?: number
  expired_at?: string | null
  allowed_models?: string[]
  allowed_ips?: string[]
}

export function useUpdateToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { id: number } & Record<string, unknown>) => {
      const { id, ...body } = data
      return api<{ updated: boolean }>(`/admin/api/tokens/${id}`, { method: 'PUT', body: JSON.stringify(body) })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tokens'] }),
  })
}

export function useDeleteToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api<{ deleted: boolean }>(`/admin/api/tokens/${id}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tokens'] }),
  })
}

// ---------- 日志 ----------
export interface LogFilters {
  model?: string
  status?: string
  channel_id?: string
  token_id?: string
  start?: string
  end?: string
  page?: number
  page_size?: number
}

/** 把筛选条件编成 query string；导出与列表共用，保证"导出即所见"。 */
export function buildLogQuery(filters: LogFilters): string {
  const params = new URLSearchParams()
  Object.entries(filters).forEach(([k, v]) => {
    if (v !== undefined && v !== '' && v !== null) params.set(k, String(v))
  })
  return params.toString()
}

export function useLogs(filters: LogFilters) {
  return useQuery({
    queryKey: ['logs', filters],
    queryFn: () => api<LogsPage>(`/admin/api/logs?${buildLogQuery(filters)}`),
    placeholderData: (prev) => prev,
  })
}

/** 全量匹配行聚合（不受分页限制），修复 Dashboard 只看最新 100 条的口径偏差。 */
export function useLogsStats(filters: Omit<LogFilters, 'page' | 'page_size'>) {
  return useQuery({
    queryKey: ['logs-stats', filters],
    queryFn: () => api<{ stats: LogStats }>(`/admin/api/logs/stats?${buildLogQuery(filters)}`),
    placeholderData: (prev) => prev,
  })
}

// ---------- 渠道批量探测 ----------
export function useChannelsTestAll() {
  return useMutation({
    mutationFn: () => api<{ items: TestAllItem[]; timeout_sec: number }>('/admin/api/channels/test-all', { method: 'POST' }),
  })
}

// ---------- 审计 ----------
export function useAuditLogs(page: number, pageSize: number, action?: string) {
  const params = new URLSearchParams({ page: String(page), page_size: String(pageSize) })
  if (action) params.set('action', action)
  return useQuery({
    queryKey: ['audit-logs', page, pageSize, action],
    queryFn: () => api<{ items: AuditLog[]; total: number }>(`/admin/api/audit-logs?${params.toString()}`),
    placeholderData: (prev) => prev,
  })
}

// ---------- 配置导出 / 导入 ----------
export function useImportConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: unknown) => api<{ result: ConfigImportResult }>('/admin/api/config/import', jsonBody(payload)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['channels'] })
      qc.invalidateQueries({ queryKey: ['tokens'] })
      qc.invalidateQueries({ queryKey: ['model-fallbacks'] })
      qc.invalidateQueries({ queryKey: ['system-settings'] })
      qc.invalidateQueries({ queryKey: ['usage-prices'] })
    },
  })
}

/** 带 Cookie 下载管理台生成的文件（CSV / JSON），失败时抛 ApiError 而不是下载一个错误页。 */
export async function downloadAdminFile(path: string, fallbackName: string) {
  const resp = await fetch(withBasePath(path), { credentials: 'same-origin' })
  if (!resp.ok) {
    let message = `HTTP ${resp.status}`
    try {
      const json = await resp.json()
      if (json?.message) message = json.message
    } catch {
      // 非 JSON 错误体，保留 HTTP 状态文案
    }
    throw new ApiError(message, resp.status)
  }
  const blob = await resp.blob()
  const disposition = resp.headers.get('Content-Disposition') || ''
  const match = disposition.match(/filename="?([^";]+)"?/i)
  const name = match?.[1] ?? fallbackName
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = name
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

// ---------- 用量与价格 ----------
export interface UsageFilters {
  start: string
  end: string
  channel_id?: string
  model?: string
  token_id?: string
}

export function useUsageReport(filters: UsageFilters) {
  const params = new URLSearchParams()
  Object.entries(filters).forEach(([key, value]) => {
    if (value !== undefined && value !== '') params.set(key, String(value))
  })
  return useQuery({
    queryKey: ['usage-report', filters],
    queryFn: () => api<UsageReport>(`/admin/api/usage/report?${params.toString()}`),
  })
}

export function useUsagePrices() {
  return useQuery({
    queryKey: ['usage-prices'],
    queryFn: () => api<UsagePrices>('/admin/api/usage/prices'),
  })
}

export function useSetDefaultPrice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { input_micros_per_million: number; output_micros_per_million: number }) =>
      api<UsagePrices>('/admin/api/usage/prices/default', { method: 'PUT', body: JSON.stringify(data) }),
    onSuccess: (data) => qc.setQueryData(['usage-prices'], data),
  })
}

export function useUpsertPriceOverride() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { channel_id: number; model: string; input_micros_per_million: number; output_micros_per_million: number }) =>
      api<UsagePrices>('/admin/api/usage/prices/overrides', { method: 'POST', body: JSON.stringify(data) }),
    onSuccess: (data) => qc.setQueryData(['usage-prices'], data),
  })
}

export function useDeletePriceOverride() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      api<UsagePrices>(`/admin/api/usage/prices/overrides/${id}`, { method: 'DELETE' }),
    onSuccess: (data) => qc.setQueryData(['usage-prices'], data),
  })
}

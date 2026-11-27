import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError, jsonBody } from './client'
import { withBasePath } from '../lib/runtime'
import type { AdminInfo, AlertRecord, AlertsPayload, AlertSetting, AuditLog, BackupsPayload, Channel, ChannelDetail, EffectivePrice, ConfigImportResult, LogModelFacet, LogStats, LogsPage, ModelCatalogItem, ModelFallback, ModelPreview, SetupStatus, SystemSettings, SystemSettingsResponse, SystemSettingsValues, TestAllItem, TestResult, Token, UpdateJob, UpdateProgress, UpdateStatus, UsagePrices, UsageReport } from './types'

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
    // 登录页在表单顶部内联展示错误，不走全局 Toast
    meta: { silentError: true },
  })
}

/** 首启设置状态：登录页与 /setup 用它决定跳转方向。 */
export function useSetupStatus(enabled = true) {
  return useQuery({
    queryKey: ['setup-status'],
    queryFn: () => api<SetupStatus>('/admin/api/auth/setup-status'),
    retry: false,
    staleTime: 0,
    enabled,
  })
}

export function useSetupAdmin() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { username: string; password: string }) =>
      api<{ username: string; setup: boolean }>('/admin/api/auth/setup', jsonBody(data)),
    // 设置成功后立刻更新状态缓存，否则登录页会读到旧的 setup_required=true 再跳回 /setup
    onSuccess: () => qc.setQueryData<SetupStatus>(['setup-status'], { setup_required: false }),
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
export function useAlerts() {
  return useQuery({
    queryKey: ['alerts'],
    queryFn: () => api<AlertsPayload>('/admin/api/alerts'),
  })
}

// ---------- 整库备份 ----------
export function useBackups() {
  return useQuery({
    queryKey: ['backups'],
    queryFn: () => api<BackupsPayload>('/admin/api/backups'),
  })
}

export function useCreateBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api<{ name: string }>('/admin/api/backups', { method: 'POST' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backups'] }),
  })
}

export function useDeleteBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => api<{ deleted: boolean }>(`/admin/api/backups/${encodeURIComponent(name)}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backups'] }),
  })
}

export function useRestoreBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData()
      form.append('file', file)
      return api<{ restart_required: boolean; safety_backup: string }>('/admin/api/backups/restore', { method: 'POST', body: form })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['backups'] }),
  })
}

export function useSaveAlerts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { enabled: boolean; preset: string; url: string; events: string[] }) =>
      api<{ setting: AlertSetting }>('/admin/api/alerts', { method: 'PUT', body: JSON.stringify(data) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts'] }),
  })
}

export function useTestAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => api<{ record: AlertRecord }>('/admin/api/alerts/test', { method: 'POST' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alerts'] }),
  })
}

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
    onSuccess: (data) => {
      qc.setQueryData<SystemSettingsResponse>(['system-settings'], (current) =>
        current ? { ...current, settings: data.settings } : current,
      )
      // 设置页底部的审计日志与保存结果同页，必须一起刷新，否则要 reload 才能看到"修改系统设置"
      void qc.invalidateQueries({ queryKey: ['audit-logs'] })
    },
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
    meta: { silentError: true },
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
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api<TestResult>(`/admin/api/channels/${id}/test`, { method: 'POST' }),
    // 手动探测会写入 last_probe_*，渠道状态由它派生；不刷新列表就会出现"状态正常但测试 ✗"的矛盾
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['channels'] }),
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
      api<{ added: number; duplicates?: number; invalid?: number }>(`/admin/api/channels/${data.channelId}/keys`, jsonBody({ keys: data.keys })),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: ['channels'] })
      qc.invalidateQueries({ queryKey: ['channel', vars.channelId] })
    },
  })
}

export function useUpdateKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: { id: number; enabled?: boolean; weight?: number; rpm_limit?: number }) =>
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
  rpm_limit?: number
  concurrency_limit?: number
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

/** refetchInterval 只影响轮询，不进 queryKey：开关自动刷新不会让缓存失效。 */
export function useLogs(filters: LogFilters, refetchInterval: number | false = false) {
  return useQuery({
    queryKey: ['logs', filters],
    queryFn: () => api<LogsPage>(`/admin/api/logs?${buildLogQuery(filters)}`),
    placeholderData: (prev) => prev,
    refetchInterval,
  })
}

/** 模型候选跟随其他筛选（调用方不传 model），让下拉只列当前范围内真实出现过的模型。 */
export function useLogModels(filters: Omit<LogFilters, 'model' | 'page' | 'page_size'>) {
  return useQuery({
    queryKey: ['log-models', filters],
    queryFn: () => api<{ items: LogModelFacet[] }>(`/admin/api/logs/models?${buildLogQuery(filters)}`),
    staleTime: 60_000,
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
    // ConfigTransferCard 把导入结果（含错误）内联渲染在卡片里
    meta: { silentError: true },
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
  // 「今天」按用户本地日：服务端用 tz_offset（分钟，东八区 480）换算日期区间
  params.set('tz_offset', String(-new Date().getTimezoneOffset()))
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

/** 价格写入载荷：缓存价为 0 表示未设置，计费时按输入价。 */
export interface PriceFields {
  input_micros_per_million: number
  output_micros_per_million: number
  cache_read_micros_per_million: number
  cache_write_micros_per_million: number
}

export function useSetDefaultPrice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: PriceFields) =>
      api<UsagePrices>('/admin/api/usage/prices/default', { method: 'PUT', body: JSON.stringify(data) }),
    onSuccess: (data) => qc.setQueryData(['usage-prices'], data),
  })
}

/** 把预设目录价格一键写成该渠道的模型覆盖（只补缺不覆盖）。 */
export function useApplyPresetPrice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (channelId: number) =>
      api<{ created: number; prices: EffectivePrice[] }>(`/admin/api/channels/${channelId}/prices/apply-preset`, { method: 'POST' }),
    onSuccess: (data, channelId) => {
      qc.setQueryData<ChannelDetail>(['channel', channelId], (current) =>
        current ? { ...current, prices: data.prices } : current)
      void qc.invalidateQueries({ queryKey: ['usage-prices'] })
    },
  })
}

export function useUpsertPriceOverride() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: PriceFields & { channel_id: number; model: string }) =>
      api<UsagePrices>('/admin/api/usage/prices/overrides', { method: 'POST', body: JSON.stringify(data) }),
    onSuccess: (data, variables) => {
      qc.setQueryData(['usage-prices'], data)
      // 渠道详情里带着生效价格，覆盖变了要重取，否则编辑弹窗的价格表还是旧值
      void qc.invalidateQueries({ queryKey: ['channel', variables.channel_id] })
    },
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

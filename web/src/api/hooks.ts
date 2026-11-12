import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, jsonBody } from './client'
import type { AdminInfo, Channel, ChannelDetail, LogsPage, SystemSettings, SystemSettingsResponse, SystemSettingsValues, TestResult, Token, UpdateStatus } from './types'

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
  page?: number
  page_size?: number
}

export function useLogs(filters: LogFilters) {
  const params = new URLSearchParams()
  Object.entries(filters).forEach(([k, v]) => {
    if (v !== undefined && v !== '' && v !== null) params.set(k, String(v))
  })
  return useQuery({
    queryKey: ['logs', filters],
    queryFn: () => api<LogsPage>(`/admin/api/logs?${params.toString()}`),
    placeholderData: (prev) => prev,
  })
}

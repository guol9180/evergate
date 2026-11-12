// 与后端 T5 API 对应的数据结构

export interface AdminInfo {
  username: string
  must_change_password: boolean
}

export interface UpdateAsset {
  name: string
  browser_download_url: string
  size: number
}

export interface UpdateStatus {
  current_version: string
  latest_version?: string
  release_url?: string
  published_at?: string
  update_available: boolean
  development?: boolean
  checked_at: string
  assets?: UpdateAsset[]
}

export interface SystemSettingsValues {
  connect_timeout_ms: number
  first_token_timeout_ms: number
  overall_timeout_ms: number
  retry_channels: number
  retry_keys_per_channel: number
  retry_total_ms: number
  cooldown_fail_threshold: number
  cooldown_base_ms: number
  cooldown_max_ms: number
  log_retention_days: number
  health_probe_interval_ms: number
  health_probe_concurrency: number
  health_probe_timeout_ms: number
}

export interface SystemSettings extends SystemSettingsValues {
  source: string
  initialized_at: string
  updated_at: string
}

export interface SystemSettingsResponse {
  settings: SystemSettings
  scope: {
    runtime_source: string
    hot_update_enabled: boolean
  }
}

export interface ChannelTimeouts {
  connect_sec?: number | null
  first_token_sec?: number | null
  overall_sec?: number | null
}

export interface Channel {
  id: number
  name: string
  type: string
  base_url: string
  priority: number
  weight: number
  models: string[] | null
  model_mapping: Record<string, string> | null
  enabled: boolean
  status: string
  supports_responses: boolean
  timeout_config: ChannelTimeouts | null
  key_count: number
  created_at: string
  updated_at: string
}

export interface ChannelKey {
  id: number
  enabled: boolean
  weight: number
  state: string
  consecutive_fails: number
  level: number
  success_count: number
  fail_count: number
  last_error: string
  cooldown_until: string | null
  rpm_limit: number
  last_used_at: string | null
}

export interface ChannelDetail {
  channel: Channel
  keys: ChannelKey[]
}

export interface TestResult {
  ok: boolean
  latency_ms: number
  status_code?: number
  error?: string
}

export interface Token {
  id: number
  name: string
  enabled: boolean
  quota: number
  used_quota: number
  expired_at: string | null
  allowed_models: string[] | null
  group: string
  created_at: string
}

export interface RetryAttempt {
  channel_id: number
  channel_name: string
  key_id: number
  attempt: number
  status_code?: number
  error_type: string
  error_message: string
  elapsed_ms: number
}

export interface RequestLog {
  id: number
  created_at: string
  token_id: number
  token_name: string
  channel_id: number
  channel_name: string
  key_id: number
  model: string
  upstream_model: string
  stream: boolean
  status_code: number
  error_type: string
  error_message: string
  retry_trace: RetryAttempt[] | null
  first_token_ms: number
  total_ms: number
  prompt_tokens: number
  completion_tokens: number
  total_tokens?: number
  cached_tokens?: number
  reasoning_tokens?: number
  cost: number
  ip: string
  user_agent: string
}

export interface LogsPage {
  items: RequestLog[]
  total: number
  page: number
  page_size: number
}

// 与后端 T5 API 对应的数据结构

export interface AdminInfo {
  username: string
  must_change_password: boolean
}

export interface SetupStatus {
  setup_required: boolean
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
  source?: string
  assets?: UpdateAsset[]
}

export interface SelfUpdateCapability {
  available: boolean
  reason?: string
  executable?: string
  platform: string
  dev: boolean
}

export interface UpdateJob {
  id: string
  stage: 'downloading' | 'verifying' | 'installing' | 'restarting' | 'failed'
  progress: number
  source?: 'npm' | 'github' | string
  target_version: string
  message?: string
  error?: string
  failed_stage?: string
  backup_path?: string
  cancelable: boolean
  started_at: string
  updated_at: string
}

export interface UpdateProgress {
  capability: SelfUpdateCapability
  job: UpdateJob | null
  current_version: string
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
  paths?: {
    config_path: string
    data_dir: string
    config_source: string
  }
  scope: {
    runtime_source: string
    hot_update_enabled: boolean
  }
}

export interface ModelFallback {
  id?: number
  model: string
  budget_threshold: number
  fallbacks: string[]
  created_at?: string
  updated_at?: string
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
  preset_id: string
  base_url: string
  priority: number
  weight: number
  models: string[] | null
  model_mapping: Record<string, string> | null
  enabled: boolean
  status: string
  available_key_count: number
  last_probe_at: string | null
  last_probe_result: '' | 'success' | 'failure' | 'inconclusive'
  last_probe_status_code: number
  last_probe_latency_ms: number
  last_probe_error_type: string
  last_probe_error: string
  next_probe_at: string | null
  supports_responses: boolean
  supports_anthropic: boolean
  supports_gemini: boolean
  timeout_config: ChannelTimeouts | null
  /** 上游目录同步差异（只提示，不改写 models）；checked_at 为空表示尚未同步 */
  catalog_diff?: ChannelCatalogDiff | null
  key_count: number
  created_at: string
  updated_at: string
}

export interface ChannelCatalogDiff {
  added?: string[] | null
  removed?: string[] | null
  upstream_count?: number
  checked_at?: string
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
  cooldown_remaining_sec: number
  rpm_limit: number
  last_used_at: string | null
}

export interface ChannelDetail {
  channel: Channel
  keys: ChannelKey[]
  prices?: EffectivePrice[]
}

/** 某渠道某模型的生效价格及来源（override / default / catalog / 空表示未计价）。 */
export interface EffectivePrice {
  model: string
  input_micros_per_million: number
  output_micros_per_million: number
  /** 缓存读写价（微美元 / 百万 token）；0 表示未设置，计费时按输入价 */
  cache_read_micros_per_million: number
  cache_write_micros_per_million: number
  source: 'override' | 'default' | 'catalog' | ''
  source_url?: string
}

export interface TestResult {
  ok: boolean
  channel_id?: number
  key_id?: number
  result?: 'success' | 'failure' | 'inconclusive'
  status?: string
  latency_ms: number
  status_code?: number
  error_type?: string
  error?: string
  probed_at?: string
  next_probe_at?: string
}

export interface ModelPreviewItem {
  public_model: string
  upstream_model: string
  /** 上游目录带 OpenRouter 风格 pricing 时的微美元 / 百万 token */
  price_micros?: {
    input_micros_per_million: number
    output_micros_per_million: number
    cache_read_micros_per_million?: number
    cache_write_micros_per_million?: number
  }
}

export interface ModelPreview {
  items: ModelPreviewItem[]
  protocol: 'anthropic' | 'gemini' | 'openai-compatible'
  pages: number
  truncated: boolean
}

export interface ModelCatalogChannel {
  channel_id: number
  channel_name: string
  upstream_model: string
}

export interface ModelCatalogItem {
  model: string
  channels: ModelCatalogChannel[]
}

export interface Token {
  id: number
  name: string
  enabled: boolean
  quota: number
  used_quota: number
  remaining_quota: number
  status: 'active' | 'disabled' | 'expired' | 'exhausted'
  expired_at: string | null
  allowed_models: string[] | null
  allowed_ips: string[] | null
  disable_fallback: boolean
  /** 0 = 不限；单进程内存计数 */
  rpm_limit: number
  concurrency_limit: number
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
  reason?: string
  from_model?: string
  to_model?: string
}

export interface RequestLog {
  id: number
  created_at: string
  token_id: number
  token_name: string
  source?: string
  channel_id: number
  channel_name: string
  key_id: number
  original_model?: string
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
  cache_write_tokens?: number
  reasoning_tokens?: number
  cost_micros: number
  cost_status: string
  cost: number
  /** 计价来源：override / default / catalog；空表示历史日志或未计价 */
  price_source?: '' | 'override' | 'default' | 'catalog'
  ip: string
  user_agent: string
  /** 请求级覆盖的脱敏摘要；为空表示没有覆盖 */
  config_override?: string
}

export interface LogsPage {
  items: RequestLog[]
  total: number
  page: number
  page_size: number
}

/** 日志筛选栏的模型候选：当前筛选下真实出现过的对外模型及请求数 */
export interface LogModelFacet {
  model: string
  requests: number
}

export interface UsageTotals {
  requests: number
  success: number
  tokens_in: number
  tokens_out: number
  cache_read_tokens?: number
  cost_micros: number
  cost: number
  priced_requests: number
  unpriced_requests: number
  no_usage_requests: number
  unknown_cost_requests: number
}

/** 聚合表按 UTC 小时落库：hour 为 UTC 整点起点（RFC3339） */
export interface UsageHourlyPoint {
  hour: string
  requests: number
  success: number
  tokens_in: number
  tokens_out: number
  cache_read_tokens?: number
  cost_micros: number
  cost: number
  priced_requests: number
  unpriced_requests: number
  no_usage_requests: number
  unknown_cost_requests: number
  first_token_avg_ms?: number
  first_token_samples?: number
}

/** 展示层按用户本地日分桶后的一天 */
export interface UsageDailyPoint {
  date: string
  requests: number
  success: number
  tokens_in: number
  tokens_out: number
  cache_read_tokens?: number
  cost_micros: number
  cost: number
  priced_requests: number
  unpriced_requests: number
  no_usage_requests: number
  unknown_cost_requests: number
  /** 该小时成功请求的首 token 平均延迟（ms）；日志已清理时缺省 */
  first_token_avg_ms?: number
  /** 参与首 token 均值的样本数，供按本地日加权平均 */
  first_token_samples?: number
}

export interface UsageDimensionRow {
  id: number
  name: string
  name_kind?: 'unattributed' | 'playground' | 'deleted'
  requests: number
  success: number
  tokens_in: number
  tokens_out: number
  cache_read_tokens?: number
  cost_micros: number
  cost: number
  priced_requests: number
  unpriced_requests: number
  no_usage_requests: number
  unknown_cost_requests: number
  /** 首 token 延迟统计（ms）来自仍保留的请求日志；无样本时缺省 */
  first_token_avg_ms?: number
  first_token_p50_ms?: number
  first_token_samples?: number
}

export interface UsageReport {
  start: string
  end: string
  totals: UsageTotals
  hourly: UsageHourlyPoint[]
  top_channels: UsageDimensionRow[]
  top_models: UsageDimensionRow[]
  top_tokens: UsageDimensionRow[]
}

export interface ModelPrice {
  id: number
  channel_id: number
  model: string
  input_micros_per_million: number
  output_micros_per_million: number
  cache_read_micros_per_million: number
  cache_write_micros_per_million: number
  created_at: string
  updated_at: string
}

export interface CatalogEntry {
  preset_id: string
  model: string
  input_micros_per_million: number
  output_micros_per_million: number
  cache_read_micros_per_million?: number
  cache_write_micros_per_million?: number
  source_url: string
}

export interface UsagePrices {
  default: ModelPrice | null
  overrides: ModelPrice[]
  catalog?: { as_of: string; note: string; entries: CatalogEntry[] }
}

/** /admin/api/logs/stats：基于全部匹配行的聚合（T75）。 */
export interface LogStats {
  total: number
  success: number
  success_rate: number | null
  p50_ms: number | null
  p95_ms: number | null
  avg_total_ms: number | null
}

/** /admin/api/channels/test-all 单渠道结果。 */
export interface TestAllItem {
  channel_id: number
  name: string
  ok: boolean
  result: string
  latency_ms: number
  error?: string
}

/** 管理台写操作审计记录。 */
export interface AuditLog {
  id: number
  created_at: string
  username: string
  action: string
  target: string
  summary: string
  ip: string
}

/** /admin/api/config/import 结果。 */
export interface ConfigImportResult {
  created: number
  updated: number
  skipped: number
  errors?: string[]
}

/** 告警通道配置（单例）。 */
export interface AlertSetting {
  id: number
  enabled: boolean
  preset: 'generic' | 'wecom' | 'feishu' | 'telegram' | 'bark'
  url: string
  events: string[]
  updated_at: string
}

export interface AlertRecord {
  id: number
  created_at: string
  event: string
  preset: string
  result: 'sent' | 'failed' | 'test'
  status_code: number
  error: string
  message: string
}

export interface AlertsPayload {
  setting: AlertSetting
  records: AlertRecord[]
  events: string[]
}

/** 整库备份条目（管理台列表 / 下载 / 删除）。 */
export interface BackupInfo {
  name: string
  size: number
  time: string
  version: string
}

export interface BackupsPayload {
  items: BackupInfo[]
  dir: string
  restart_pending: boolean
}

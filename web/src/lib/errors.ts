import { ApiError } from '../api/client'
import { colon, dict, t, type MessageKey } from './i18n'

/** 服务端稳定错误码 → 字典键；新增 code 必须同时补这里与 error.* 文案（tests/errorCodes.test.ts 守卫）。 */
const codeKeys: Record<string, MessageKey> = {
  admin_local_only: 'error.admin_local_only',
  already_initialized: 'error.already_initialized',
  catalog_query_failed: 'error.catalog_query_failed',
  catalog_unavailable: 'error.catalog_unavailable',
  channel_create_failed: 'error.channel_create_failed',
  channel_disabled: 'error.channel_disabled',
  channel_no_enabled_key: 'error.channel_no_enabled_key',
  channel_no_key: 'error.channel_no_key',
  channel_not_found: 'error.channel_not_found',
  channel_not_preset: 'error.channel_not_preset',
  channels_query_failed: 'error.channels_query_failed',
  config_channels_invalid: 'error.config_channels_invalid',
  config_fallbacks_invalid: 'error.config_fallbacks_invalid',
  config_price_default_invalid: 'error.config_price_default_invalid',
  config_price_overrides_invalid: 'error.config_price_overrides_invalid',
  config_settings_invalid: 'error.config_settings_invalid',
  config_tokens_invalid: 'error.config_tokens_invalid',
  conflict: 'error.conflict',
  create_failed: 'error.create_failed',
  credentials_required: 'error.credentials_required',
  delete_failed: 'error.delete_failed',
  end_before_start: 'error.end_before_start',
  end_in_future: 'error.end_in_future',
  fallbacks_query_failed: 'error.fallbacks_query_failed',
  fallbacks_save_failed: 'error.fallbacks_save_failed',
  hash_failed: 'error.hash_failed',
  import_rolled_back: 'error.import_rolled_back',
  invalid_credentials: 'error.invalid_credentials',
  invalid_end_date: 'error.invalid_end_date',
  invalid_id: 'error.invalid_id',
  invalid_param: 'error.invalid_param',
  invalid_start_date: 'error.invalid_start_date',
  invalid_username: 'error.invalid_username',
  key_batch_too_large: 'error.key_batch_too_large',
  key_decrypt_failed: 'error.key_decrypt_failed',
  key_encrypt_failed: 'error.key_encrypt_failed',
  key_import_rolled_back: 'error.key_import_rolled_back',
  key_not_found: 'error.key_not_found',
  key_query_failed: 'error.key_query_failed',
  log_not_found: 'error.log_not_found',
  login_rate_limited: 'error.login_rate_limited',
  models_fetch_cancelled: 'error.models_fetch_cancelled',
  models_fetch_failed: 'error.models_fetch_failed',
  models_fetch_timeout: 'error.models_fetch_timeout',
  must_change_password: 'error.must_change_password',
  negative_quota: 'error.negative_quota',
  not_found: 'error.not_found',
  password_too_short: 'error.password_too_short',
  playground_disabled: 'error.playground_disabled',
  preset_price_write_failed: 'error.preset_price_write_failed',
  prices_query_failed: 'error.prices_query_failed',
  pricing_not_ready: 'error.pricing_not_ready',
  prober_not_ready: 'error.prober_not_ready',
  query_failed: 'error.query_failed',
  range_too_long: 'error.range_too_long',
  session_expired: 'error.session_expired',
  session_invalid: 'error.session_invalid',
  settings_not_ready: 'error.settings_not_ready',
  setup_failed: 'error.setup_failed',
  setup_local_only: 'error.setup_local_only',
  setup_status_failed: 'error.setup_status_failed',
  stats_query_failed: 'error.stats_query_failed',
  test_cancelled: 'error.test_cancelled',
  test_failed: 'error.test_failed',
  token_create_failed: 'error.token_create_failed',
  token_not_found: 'error.token_not_found',
  tokens_query_failed: 'error.tokens_query_failed',
  unauthorized: 'error.unauthorized',
  unsupported_config_version: 'error.unsupported_config_version',
  update_check_failed: 'error.update_check_failed',
  update_failed: 'error.update_failed',
  update_not_configured: 'error.update_not_configured',
  usage_not_ready: 'error.usage_not_ready',
  wrong_old_password: 'error.wrong_old_password',
}

/** 原样透传细节的码：服务端 message 是 "<中文前缀>：<细节>" 或整段细节，前端按字典本地化前缀、保留细节。 */
const dynamicCodes = new Set(['invalid_param', 'not_found', 'conflict', 'update_check_failed', 'models_fetch_failed', 'fallbacks_save_failed',
  'config_channels_invalid', 'config_fallbacks_invalid', 'config_price_default_invalid', 'config_price_overrides_invalid', 'config_settings_invalid', 'config_tokens_invalid'])

export function errorCode(error: unknown): string | null {
  return error instanceof ApiError && error.code ? error.code : null
}

function detailOf(message: string, code: string): string {
  const key = codeKeys[code]
  const serverPrefix: Record<string, RegExp> = {
    config_channels_invalid: /^channels\[\d+\]:\s*/,
    config_fallbacks_invalid: /^model_fallbacks:\s*/,
    config_price_default_invalid: /^price_default:\s*/,
    config_price_overrides_invalid: /^price_overrides:\s*/,
    config_settings_invalid: /^settings:\s*/,
    config_tokens_invalid: /^tokens\[[^\]]*\]:\s*/,
  }
  const raw = message.replace(serverPrefix[code] ?? /^$/, '')
  // 服务端前缀是中文字典文案；按两种语言的标签各试一次，对不上说明整段就是细节
  for (const label of [dict['zh-CN'][key], dict.en[key]]) {
    const stripped = raw.replace(new RegExp('^' + label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&') + '\\s*[：:]\\s*'), '')
    if (stripped !== raw) return stripped.trim()
    if (raw.trim() === label) return ''
  }
  return raw.trim()
}

/** 统一错误文案：有稳定 code 走字典（细节原样附带），否则回退原始 message；非 Error 值给通用提示。 */
export function errorMessage(error: unknown): string {
  const message = error instanceof Error ? error.message : typeof error === 'string' ? error : ''
  const code = errorCode(error)
  const key = code ? codeKeys[code] : undefined
  if (!key) return message || t('toast.failed')
  if (!dynamicCodes.has(code!) || !message) return t(key)
  const detail = detailOf(message, code!)
  return detail ? t(key) + colon() + detail : t(key)
}

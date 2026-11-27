// 展示层格式化（存储 UTC，展示转本地时区——AGENTS 红线）
// 错误类别人话映射参照 gpt-load credential-failure-presenter 模式
import type { Tone } from './tone'
// 带 .ts 后缀：node --experimental-strip-types 跑单测时不做扩展名解析
import { colon, getLocale, t, tf, type MessageKey } from './i18n.ts'

export function fmtTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  // hourCycle h23 而不是 hour12:false：后者在部分引擎的 en-US 下会把午夜显示成 24:00
  return d.toLocaleString(getLocale(), { hourCycle: 'h23' })
}

export function fmtMs(ms: number | null | undefined): string {
  if (ms == null || ms === 0) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

export function fmtInt(n: number | null | undefined): string {
  if (n == null) return '—'
  return n.toLocaleString(getLocale())
}

/** 微美元 → 美元展示；去掉末尾多余的 0。 */
export function fmtMoney(micros: number | null | undefined): string {
  return `$${((micros ?? 0) / 1_000_000).toFixed(6).replace(/\.?0+$/, '')}`
}

export function fmtRequestCost(micros: number | null | undefined, status: string | null | undefined): string {
  switch (status) {
    case 'priced':
      return fmtMoney(micros)
    case 'unpriced':
      return t('cost.unpriced')
    case 'no_usage':
      return t('cost.noUsage')
    default:
      return t('cost.unknown')
  }
}

/**
 * key 导入结果的统一文案（渠道 key 面板与新建渠道内联导入共用）：
 * 有新增走成功提示并附跳过数，全被跳过时明确告知没有新增，避免误以为导入成功。
 */
export function keyImportMessage(result: { added: number; duplicates?: number; invalid?: number }): { tone: 'success' | 'info'; text: string } {
  const duplicates = result.duplicates ?? 0
  const invalid = result.invalid ?? 0
  if (result.added === 0) {
    return { tone: 'info', text: tf('channels.import.nothingNew', { duplicates, invalid }) }
  }
  const skipped: string[] = []
  if (duplicates > 0) skipped.push(tf('channels.import.duplicates', { n: duplicates }))
  if (invalid > 0) skipped.push(tf('channels.import.invalid', { n: invalid }))
  return { tone: 'success', text: tf('channels.import.done', { n: result.added }) + (skipped.length ? ` · ${skipped.join(' · ')}` : '') }
}

export function cooldownRemaining(until: string | null | undefined): string | null {
  if (!until) return null
  const ms = new Date(until).getTime() - Date.now()
  if (ms <= 0) return null
  return formatRemainingSeconds(ms / 1000)
}

/** 把剩余秒数格式化为 12s / 3m05s / 1h20m；≤0 返回 null（表示不在冷却）。 */
export function formatRemainingSeconds(totalSec: number): string | null {
  if (!totalSec || totalSec <= 0) return null
  const sec = Math.ceil(totalSec)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m${sec % 60}s`
  return `${Math.floor(min / 60)}h${min % 60}m`
}

/**
 * 解析上游 / 网关错误文本：识别 `all upstream channels exhausted: {"error":{"code":"1305","message":"…"}}`
 * 与 OpenAI 风格 `{"error":{"message":"…"}}`，提取上游错误码与可读信息；无法解析时原样返回。
 */
export function describeUpstreamError(message: string | null | undefined): { code: string | null; text: string } {
  const raw = (message ?? '').trim()
  const start = raw.indexOf('{')
  if (start < 0) return { code: null, text: raw }
  const candidate = raw.slice(start)
  const lastBrace = candidate.lastIndexOf('}')
  const jsonText = lastBrace > 0 ? candidate.slice(0, lastBrace + 1) : candidate
  try {
    const parsed = JSON.parse(jsonText) as { error?: { code?: string | number; message?: string }; message?: string }
    const text = parsed.error?.message ?? parsed.message
    if (!text) return { code: null, text: raw }
    const prefix = raw.slice(0, start).replace(/:\s*$/, '').trim()
    return { code: parsed.error?.code != null ? String(parsed.error.code) : null, text: prefix ? `${prefix}${colon()}${text}` : text }
  } catch {
    return { code: null, text: raw }
  }
}

// 状态 → 语义色调（Tone 枚举唯一来源在 components/ui.tsx；此处只做业务状态到色调的映射）
export function channelStateTone(status: string): Tone {
  const tones: Record<string, Tone> = {
    active: 'success',
    half_open: 'info',
    cooldown: 'warning',
    disabled: 'danger',
    degraded: 'warning',
    down: 'danger',
    inconclusive: 'neutral',
  }
  return tones[status] ?? 'neutral'
}

export function tokenStatusTone(status: string): Tone {
  const tones: Record<string, Tone> = {
    active: 'success',
    disabled: 'neutral',
    expired: 'warning',
    exhausted: 'danger',
  }
  return tones[status] ?? 'neutral'
}

export function httpStatusTone(code: number): Tone {
  if (code >= 200 && code < 300) return 'success'
  if (code >= 400 && code < 500) return 'warning'
  return 'danger'
}

const tokenStatusKeys: Record<string, MessageKey> = {
  active: 'tokenStatus.active',
  disabled: 'tokenStatus.disabled',
  expired: 'tokenStatus.expired',
  exhausted: 'tokenStatus.exhausted',
}

export function tokenStatusLabel(status: string): string {
  const key = tokenStatusKeys[status]
  return key ? t(key) : status
}

const stateKeys: Record<string, MessageKey> = {
  active: 'keyState.active',
  half_open: 'keyState.half_open',
  cooldown: 'keyState.cooldown',
  disabled: 'keyState.disabled',
  degraded: 'keyState.degraded',
  down: 'keyState.down',
  inconclusive: 'keyState.inconclusive',
}

export function stateLabel(status: string): string {
  const key = stateKeys[status]
  return key ? t(key) : status
}

export function statusRank(status: string): number {
  const ranks: Record<string, number> = { down: 0, degraded: 1, inconclusive: 2, active: 3 }
  return ranks[status] ?? 1
}

// retry.ErrorKind → 人话（与后端 T4a 分类学对齐）
const kindKeys: Record<string, MessageKey> = {
  transient: 'kind.transient',
  rate_limited: 'kind.rate_limited',
  auth: 'kind.auth',
  model_unavailable: 'kind.model_unavailable',
  non_retryable: 'kind.non_retryable',
  content_policy: 'kind.content_policy',
  budget_threshold: 'kind.budget_threshold',
  channel_exhausted: 'kind.channel_exhausted',
}

export function kindText(kind: string): string {
  const key = kindKeys[kind]
  return key ? t(key) : kind || t('kind.success')
}

export function statusTone(code: number): string {
  if (code >= 200 && code < 300) return 'text-success-text'
  if (code >= 400 && code < 500) return 'text-warning-text'
  return 'text-danger-text'
}

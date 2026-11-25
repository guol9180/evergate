// 展示层格式化（存储 UTC，展示转本地时区——AGENTS 红线）
// 错误类别人话映射参照 gpt-load credential-failure-presenter 模式
import type { Tone } from './tone'

export function fmtTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleString('zh-CN', { hour12: false })
}

export function fmtMs(ms: number | null | undefined): string {
  if (ms == null || ms === 0) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

export function fmtInt(n: number | null | undefined): string {
  if (n == null) return '—'
  return n.toLocaleString('zh-CN')
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
      return '未计价'
    case 'no_usage':
      return '无 usage'
    default:
      return '未记录'
  }
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
    return { code: parsed.error?.code != null ? String(parsed.error.code) : null, text: prefix ? `${prefix}：${text}` : text }
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

export const tokenStatusLabel: Record<string, string> = {
  active: '可用',
  disabled: '停用',
  expired: '过期',
  exhausted: '额度用尽',
}

export const stateLabel: Record<string, string> = {
  active: '正常',
  half_open: '半开探测',
  cooldown: '冷却中',
  disabled: '已禁用',
  degraded: '降级',
  down: '不可用',
  inconclusive: '无法判定',
}

export function statusRank(status: string): number {
  const ranks: Record<string, number> = { down: 0, degraded: 1, inconclusive: 2, active: 3 }
  return ranks[status] ?? 1
}

// retry.ErrorKind → 人话（与后端 T4a 分类学对齐）
export const kindLabel: Record<string, string> = {
  transient: '服务器故障',
  rate_limited: '限流/配额',
  auth: '凭据无效',
  model_unavailable: '模型不可用',
  non_retryable: '请求错误',
  content_policy: '内容策略',
  budget_threshold: '预算降级',
  channel_exhausted: '渠道耗尽降级',
}

export function kindText(kind: string): string {
  return kindLabel[kind] || kind || '成功'
}

export function statusTone(code: number): string {
  if (code >= 200 && code < 300) return 'text-success-text'
  if (code >= 400 && code < 500) return 'text-warning-text'
  return 'text-danger-text'
}

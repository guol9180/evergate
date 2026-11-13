// 展示层格式化（存储 UTC，展示转本地时区——AGENTS 红线）
// 错误类别人话映射参照 gpt-load credential-failure-presenter 模式
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

export function cooldownRemaining(until: string | null | undefined): string | null {
  if (!until) return null
  const ms = new Date(until).getTime() - Date.now()
  if (ms <= 0) return null
  const sec = Math.ceil(ms / 1000)
  if (sec < 60) return `${sec}s`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min}m${sec % 60}s`
  return `${Math.floor(min / 60)}h${min % 60}m`
}

// 状态 → 徽标色调
export const stateTone: Record<string, string> = {
  active: 'bg-emerald-100 text-emerald-700',
  half_open: 'bg-blue-100 text-blue-700',
  cooldown: 'bg-amber-100 text-amber-700',
  disabled: 'bg-red-100 text-red-700',
  degraded: 'bg-amber-100 text-amber-700',
  down: 'bg-red-100 text-red-700',
  inconclusive: 'bg-slate-200 text-slate-700',
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
}

export function kindText(kind: string): string {
  return kindLabel[kind] || kind || '成功'
}

export function statusTone(code: number): string {
  if (code >= 200 && code < 300) return 'text-emerald-600'
  if (code >= 400 && code < 500) return 'text-amber-600'
  return 'text-red-600'
}

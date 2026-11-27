import type { UsageDailyPoint, UsageHourlyPoint } from '../api/types'

const FIELDS = ['requests', 'success', 'tokens_in', 'tokens_out', 'cache_read_tokens', 'cost_micros', 'priced_requests', 'unpriced_requests', 'no_usage_requests', 'unknown_cost_requests'] as const

/** 本地日键（YYYY-MM-DD）：用本地时区拆年月日，不依赖 toLocaleDateString 的实现差异。 */
export function localDayKey(iso: string): string {
  const d = new Date(iso)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/**
 * 把 UTC 小时点按用户本地日分桶求和（展示层口径）：聚合表只知道 UTC 小时，
 * 「今天」在哪一天由浏览器时区决定，这样概览页与用量页在同一时刻包含同一批请求。
 */
export function bucketByLocalDay(hours: UsageHourlyPoint[]): UsageDailyPoint[] {
  const byDay = new Map<string, UsageDailyPoint>()
  // 首 token 延迟是均值而非可加量：按样本数加权，避免把各小时的均值直接求和
  const latencyWeight = new Map<string, { weighted: number; samples: number }>()
  for (const point of hours) {
    const key = localDayKey(point.hour)
    const bucket = byDay.get(key) ?? { date: key, requests: 0, success: 0, tokens_in: 0, tokens_out: 0, cache_read_tokens: 0, cost_micros: 0, cost: 0, priced_requests: 0, unpriced_requests: 0, no_usage_requests: 0, unknown_cost_requests: 0 }
    for (const field of FIELDS) bucket[field] = (bucket[field] ?? 0) + (point[field] ?? 0)
    bucket.cost += point.cost
    if (point.first_token_avg_ms != null && point.first_token_samples) {
      const current = latencyWeight.get(key) ?? { weighted: 0, samples: 0 }
      current.weighted += point.first_token_avg_ms * point.first_token_samples
      current.samples += point.first_token_samples
      latencyWeight.set(key, current)
    }
    byDay.set(key, bucket)
  }
  for (const [key, latency] of latencyWeight) {
    const bucket = byDay.get(key)
    if (!bucket || latency.samples === 0) continue
    bucket.first_token_avg_ms = latency.weighted / latency.samples
    bucket.first_token_samples = latency.samples
  }
  return [...byDay.values()].sort((a, b) => a.date.localeCompare(b.date))
}

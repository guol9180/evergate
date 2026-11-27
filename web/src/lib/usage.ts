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
  for (const point of hours) {
    const key = localDayKey(point.hour)
    const bucket = byDay.get(key) ?? { date: key, requests: 0, success: 0, tokens_in: 0, tokens_out: 0, cache_read_tokens: 0, cost_micros: 0, cost: 0, priced_requests: 0, unpriced_requests: 0, no_usage_requests: 0, unknown_cost_requests: 0 }
    for (const field of FIELDS) bucket[field] = (bucket[field] ?? 0) + (point[field] ?? 0)
    bucket.cost += point.cost
    byDay.set(key, bucket)
  }
  return [...byDay.values()].sort((a, b) => a.date.localeCompare(b.date))
}

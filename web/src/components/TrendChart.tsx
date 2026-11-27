import { useMemo } from 'react'
import {
  Bar,
  CartesianGrid,
  ComposedChart,
  Line,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  type TooltipContentProps,
} from 'recharts'
import type { UsageDailyPoint } from '../api/types'
import { fmtInt, fmtMoney, fmtMs } from '../lib/format'
import { t, tf } from '../lib/i18n'

/** 趋势图可切换的柱状指标；成功率折线始终保留。 */
export type TrendMetric = 'requests' | 'tokens' | 'cost' | 'firstToken'

export const TREND_METRICS: readonly { value: TrendMetric; label: Parameters<typeof t>[0] }[] = [
  { value: 'requests', label: 'chart.metric.requests' },
  { value: 'tokens', label: 'chart.metric.tokens' },
  { value: 'cost', label: 'chart.metric.cost' },
  { value: 'firstToken', label: 'chart.metric.firstToken' },
]

type TrendRow = UsageDailyPoint & {
  label: string
  successRate: number | null
  totalTokens: number | null
}

/** 轴刻度用紧凑数字：1.2k / 3.4M，避免大数撑宽 Y 轴。 */
function compactNumber(value: number): string {
  const abs = Math.abs(value)
  if (abs >= 1_000_000) return `${Number((value / 1_000_000).toFixed(1))}M`
  if (abs >= 1_000) return `${Number((value / 1_000).toFixed(1))}k`
  return String(Math.round(value))
}

function metricDataKey(metric: TrendMetric): keyof TrendRow {
  switch (metric) {
    case 'tokens': return 'totalTokens'
    case 'cost': return 'cost_micros'
    case 'firstToken': return 'first_token_avg_ms'
    default: return 'requests'
  }
}

function formatMetricValue(metric: TrendMetric, row: TrendRow): string {
  switch (metric) {
    case 'tokens': return row.totalTokens == null ? '—' : fmtInt(row.totalTokens)
    case 'cost': return fmtMoney(row.cost_micros)
    case 'firstToken': return row.first_token_avg_ms == null ? '—' : fmtMs(row.first_token_avg_ms)
    default: return fmtInt(row.requests)
  }
}

/**
 * 用量趋势：柱 = 所选指标（请求 / Token / 成本 / 首 token 延迟），线 = 成功率。
 * 使用 Recharts ComposedChart，数据量固定为按天聚合，SVG 渲染足够且交互/主题一致。
 */
export function TrendChart({ data, metric = 'requests', className = 'h-56' }: { data: UsageDailyPoint[]; metric?: TrendMetric; className?: string }) {
  const rows = useMemo<TrendRow[]>(
    () =>
      data.map((point) => ({
        ...point,
        label: point.date.slice(5),
        successRate: point.requests > 0 ? Math.round((point.success / point.requests) * 1000) / 10 : null,
        totalTokens: point.requests > 0 ? point.tokens_in + point.tokens_out : null,
      })),
    [data],
  )
  const dataKey = metricDataKey(metric)
  const axisFormatter = (value: number) => (metric === 'cost' ? `$${compactNumber(value / 1_000_000)}` : compactNumber(value))

  return (
    <div className={`w-full ${className}`}>
      <ResponsiveContainer width="100%" height="100%">
        <ComposedChart data={rows} margin={{ top: 10, right: 0, bottom: 0, left: 0 }} accessibilityLayer>
          <CartesianGrid vertical={false} stroke="var(--color-line)" strokeDasharray="3 3" />
          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={20}
            tick={{ fontSize: 11, fill: 'var(--color-subtle)' }}
          />
          <YAxis
            yAxisId="metric"
            width={48}
            allowDecimals={false}
            tickFormatter={axisFormatter}
            tickLine={false}
            axisLine={false}
            tick={{ fontSize: 11, fill: 'var(--color-subtle)' }}
          />
          <YAxis
            yAxisId="rate"
            orientation="right"
            width={38}
            domain={[0, 100]}
            ticks={[0, 50, 100]}
            tickFormatter={(value: number) => `${value}%`}
            tickLine={false}
            axisLine={false}
            tick={{ fontSize: 11, fill: 'var(--color-subtle)' }}
          />
          <Tooltip
            content={(props) => <ChartTooltip {...props} metric={metric} />}
            cursor={{ fill: 'rgba(99, 102, 241, 0.06)' }}
          />
          <Bar
            yAxisId="metric"
            dataKey={dataKey}
            name={t(`chart.metric.${metric}`)}
            fill="var(--color-brand-text)"
            radius={[4, 4, 0, 0]}
            maxBarSize={28}
            activeBar={{ fill: 'var(--color-brand-text)' }}
          />
          <Line
            yAxisId="rate"
            dataKey="successRate"
            name={t('chart.successRate')}
            stroke="var(--color-success-solid)"
            strokeWidth={2}
            connectNulls={false}
            dot={rows.length <= 14 ? { r: 2.5, fill: 'var(--color-success-solid)', strokeWidth: 0 } : false}
            activeDot={{ r: 4, strokeWidth: 2, className: 'eg-chart-dot' }}
          />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  )
}

function ChartTooltip({ active, payload, metric }: TooltipContentProps & { metric?: TrendMetric }) {
  if (!active || !payload?.length) return null
  const row = payload[0]?.payload as TrendRow | undefined
  if (!row) return null
  const current = metric ?? 'requests'
  const costIssues = row.unpriced_requests + row.no_usage_requests + row.unknown_cost_requests
  const costKnown = row.requests === 0 || row.priced_requests > 0 || costIssues === 0
  return (
    <div className="min-w-36 rounded-lg border border-line bg-surface/95 px-3 py-2 text-xs shadow-lg backdrop-blur">
      <div className="font-medium text-foreground-soft">{row.date}</div>
      <dl className="mt-1.5 space-y-1">
        <div className="flex items-center justify-between gap-4">
          <dt className="text-subtle">{t(`chart.metric.${current}`)}</dt>
          <dd className="tabular-nums text-foreground-soft">{formatMetricValue(current, row)}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="text-subtle">{t('chart.success')}</dt>
          <dd className="tabular-nums text-foreground-soft">{fmtInt(row.success)}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="text-subtle">{t('chart.successRate')}</dt>
          <dd className="tabular-nums text-success-text">{row.successRate == null ? '—' : `${row.successRate.toFixed(1)}%`}</dd>
        </div>
        {current !== 'firstToken' && row.first_token_avg_ms != null && (
          <div className="flex items-center justify-between gap-4">
            <dt className="text-subtle">{t('chart.metric.firstToken')}</dt>
            <dd className="tabular-nums text-foreground-soft">{fmtMs(row.first_token_avg_ms)}</dd>
          </div>
        )}
        {!!row.cache_read_tokens && (
          <div className="flex items-center justify-between gap-4">
            <dt className="text-subtle">{t('chart.cacheRead')}</dt>
            <dd className="tabular-nums text-foreground-soft">{fmtInt(row.cache_read_tokens)}</dd>
          </div>
        )}
        <div className="flex items-center justify-between gap-4">
          <dt className="text-subtle">{t('chart.cost')}</dt>
          <dd className="tabular-nums text-foreground-soft">{costKnown ? fmtMoney(row.cost_micros) : '—'}</dd>
        </div>
        {costIssues > 0 && (
          <div className="flex items-center justify-between gap-4 text-warning-text">
            <dt>{t('chart.costStatus')}</dt>
            <dd>{tf('chart.costIssues', { count: fmtInt(costIssues) })}</dd>
          </div>
        )}
      </dl>
    </div>
  )
}

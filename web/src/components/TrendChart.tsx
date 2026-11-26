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
import { fmtInt, fmtMoney } from '../lib/format'
import { t, tf } from '../lib/i18n'

type TrendRow = UsageDailyPoint & {
  label: string
  successRate: number | null
}

/**
 * 用量日趋势：柱 = 请求数，线 = 成功率。
 * 使用 Recharts ComposedChart，数据量固定为按天聚合，SVG 渲染足够且交互/主题一致。
 */
export function TrendChart({ data, className = 'h-56' }: { data: UsageDailyPoint[]; className?: string }) {
  const rows = useMemo<TrendRow[]>(
    () =>
      data.map((point) => ({
        ...point,
        label: point.date.slice(5),
        successRate: point.requests > 0 ? Math.round((point.success / point.requests) * 1000) / 10 : null,
      })),
    [data],
  )

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
            yAxisId="requests"
            width={44}
            allowDecimals={false}
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
            content={ChartTooltip}
            cursor={{ fill: 'rgba(99, 102, 241, 0.06)' }}
          />
          <Bar
            yAxisId="requests"
            dataKey="requests"
            name={t('chart.requests')}
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
            activeDot={{ r: 4, strokeWidth: 2, stroke: '#fff' }}
          />
        </ComposedChart>
      </ResponsiveContainer>
    </div>
  )
}

function ChartTooltip({ active, payload }: TooltipContentProps) {
  if (!active || !payload?.length) return null
  const row = payload[0]?.payload as TrendRow | undefined
  if (!row) return null
  const costIssues = row.unpriced_requests + row.no_usage_requests + row.unknown_cost_requests
  const costKnown = row.requests === 0 || row.priced_requests > 0 || costIssues === 0
  return (
    <div className="min-w-36 rounded-lg border border-line bg-surface/95 px-3 py-2 text-xs shadow-lg backdrop-blur">
      <div className="font-medium text-foreground-soft">{row.date}</div>
      <dl className="mt-1.5 space-y-1">
        <div className="flex items-center justify-between gap-4">
          <dt className="text-subtle">{t('chart.request')}</dt>
          <dd className="tabular-nums text-foreground-soft">{fmtInt(row.requests)}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="text-subtle">{t('chart.success')}</dt>
          <dd className="tabular-nums text-foreground-soft">{fmtInt(row.success)}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="text-subtle">{t('chart.successRate')}</dt>
          <dd className="tabular-nums text-success-text">{row.successRate == null ? '—' : `${row.successRate.toFixed(1)}%`}</dd>
        </div>
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

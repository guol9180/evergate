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
        <ComposedChart data={rows} margin={{ top: 10, right: 0, bottom: 0, left: -20 }} accessibilityLayer>
          <CartesianGrid vertical={false} stroke="#eef2f7" strokeDasharray="3 3" />
          <XAxis
            dataKey="label"
            tickLine={false}
            axisLine={false}
            tickMargin={8}
            minTickGap={20}
            tick={{ fontSize: 11, fill: '#94a3b8' }}
          />
          <YAxis
            yAxisId="requests"
            width={40}
            allowDecimals={false}
            tickLine={false}
            axisLine={false}
            tick={{ fontSize: 11, fill: '#94a3b8' }}
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
            tick={{ fontSize: 11, fill: '#94a3b8' }}
          />
          <Tooltip
            content={ChartTooltip}
            cursor={{ fill: 'rgba(99, 102, 241, 0.06)' }}
          />
          <Bar
            yAxisId="requests"
            dataKey="requests"
            name="请求数"
            fill="#6366f1"
            radius={[4, 4, 0, 0]}
            maxBarSize={28}
            activeBar={{ fill: '#4f46e5' }}
          />
          <Line
            yAxisId="rate"
            dataKey="successRate"
            name="成功率"
            stroke="#10b981"
            strokeWidth={2}
            connectNulls={false}
            dot={rows.length <= 14 ? { r: 2.5, fill: '#10b981', strokeWidth: 0 } : false}
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
  return (
    <div className="min-w-36 rounded-lg border border-slate-200 bg-white/95 px-3 py-2 text-xs shadow-lg backdrop-blur">
      <div className="font-medium text-slate-700">{row.date}</div>
      <dl className="mt-1.5 space-y-1">
        <div className="flex items-center justify-between gap-4">
          <dt className="text-slate-400">请求</dt>
          <dd className="tabular-nums text-slate-700">{fmtInt(row.requests)}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="text-slate-400">成功</dt>
          <dd className="tabular-nums text-slate-700">{fmtInt(row.success)}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="text-slate-400">成功率</dt>
          <dd className="tabular-nums text-emerald-600">{row.successRate == null ? '—' : `${row.successRate.toFixed(1)}%`}</dd>
        </div>
        <div className="flex items-center justify-between gap-4">
          <dt className="text-slate-400">成本</dt>
          <dd className="tabular-nums text-slate-700">{fmtMoney(row.cost_micros)}</dd>
        </div>
      </dl>
    </div>
  )
}

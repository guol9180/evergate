import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { AlertTriangle, BarChart3, CircleDollarSign, Plus, RefreshCw, Trash2 } from 'lucide-react'
import {
  useChannels,
  useDeletePriceOverride,
  useSetDefaultPrice,
  useTokens,
  useUpsertPriceOverride,
  useUsagePrices,
  useUsageReport,
} from '../api/hooks'
import type { UsageDailyPoint, UsageDimensionRow } from '../api/types'
import { Badge, Button, Card, Empty, ErrorText, Field, Input, PageHeader, Select, Spinner, Td, Th } from '../components/ui'
import { TrendChart } from '../components/TrendChart'
import { fmtInt, fmtMoney } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'

const today = () => new Date().toISOString().slice(0, 10)
const daysAgo = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString().slice(0, 10)

export default function Usage() {
  usePageTitle(t('page.usage.title'))
  const [start, setStart] = useState(daysAgo(6))
  const [end, setEnd] = useState(today())
  const [channelID, setChannelID] = useState('')
  const [model, setModel] = useState('')
  const [tokenID, setTokenID] = useState('')
  const channels = useChannels()
  const tokens = useTokens()
  const report = useUsageReport({ start, end, channel_id: channelID, model, token_id: tokenID })
  const prices = useUsagePrices()

  const chs = channels.data?.items ?? []
  const tks = tokens.data?.items ?? []
  const data = report.data
  const daily = useMemo(() => fillDailyGaps(data?.daily ?? [], data?.start, data?.end), [data])
  const successRate = data?.totals.requests ? (data.totals.success / data.totals.requests) * 100 : null
  const costIssues = data ? [
    data.totals.unpriced_requests > 0 ? `${fmtInt(data.totals.unpriced_requests)} 条未计价` : '',
    data.totals.no_usage_requests > 0 ? `${fmtInt(data.totals.no_usage_requests)} 条无 usage` : '',
    data.totals.unknown_cost_requests > 0 ? `${fmtInt(data.totals.unknown_cost_requests)} 条历史未记录` : '',
  ].filter(Boolean) : []
  const costKnown = data != null && data.totals.requests > 0 &&
    (data.totals.priced_requests > 0 || data.totals.unpriced_requests + data.totals.no_usage_requests + data.totals.unknown_cost_requests === 0)

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.usage.title')}
        description={t('page.usage.desc')}
        actions={
          <Button variant="outline" onClick={() => report.refetch()} disabled={report.isFetching}>
            <RefreshCw size={14} className={report.isFetching ? 'animate-spin' : ''} /> {t('common.refresh')}
          </Button>
        }
      />

      <Card>
        <div className="eg-grid eg-grid-auto">
          <Field label="开始日期"><Input type="date" value={start} max={end} onChange={(e) => setStart(e.target.value)} /></Field>
          <Field label="结束日期"><Input type="date" value={end} min={start} max={today()} onChange={(e) => setEnd(e.target.value)} /></Field>
          <Field label="渠道">
            <Select value={channelID} onChange={(e) => setChannelID(e.target.value)}>
              <option value="">全部渠道</option>
              {chs.map((ch) => <option key={ch.id} value={ch.id}>{ch.name}</option>)}
            </Select>
          </Field>
          <Field label="模型"><Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="全部模型" /></Field>
          <Field label="令牌">
            <Select value={tokenID} onChange={(e) => setTokenID(e.target.value)}>
              <option value="">全部令牌</option>
              {tks.map((token) => <option key={token.id} value={token.id}>{token.name}</option>)}
            </Select>
          </Field>
        </div>
        <ErrorText>{report.isError ? (report.error as Error).message : null}</ErrorText>
      </Card>

      {prices.data && !prices.data.default && prices.data.overrides.length === 0 && (
        <Card className="border-warning-border bg-warning-subtle">
          <div className="flex items-start gap-3 text-sm text-warning-text">
            <AlertTriangle size={18} className="mt-0.5 shrink-0 text-warning-text" />
            <div>
              <div className="font-semibold">尚未配置任何模型价格</div>
              <p className="mt-1 text-xs leading-5 text-warning-text">
                usage 与 token 已正常记录，但成本会标记为“未计价”，不会用 <code>$0</code> 冒充已知成本。
                在下方“模型价格”保存全局默认价或渠道+模型覆盖价后，新请求会按请求时价格快照计价；历史未计价请求不会追溯重算。
              </p>
            </div>
          </div>
        </Card>
      )}

      {report.isPending ? (
        <div className="flex justify-center py-16"><Spinner className="h-6 w-6" /></div>
      ) : !data ? null : (
        <>
          <div className="eg-grid eg-grid-kpi">
            <Metric label="请求数" value={fmtInt(data.totals.requests)} hint={`成功 ${fmtInt(data.totals.success)}`} />
            <Metric label="成功率" value={successRate == null ? '—' : `${successRate.toFixed(1)}%`} hint="2xx / 3xx" />
            <Metric label="Token" value={fmtInt(data.totals.tokens_in + data.totals.tokens_out)} hint={`输入 ${fmtInt(data.totals.tokens_in)} · 输出 ${fmtInt(data.totals.tokens_out)}`} />
            <Metric
              label="成本"
              value={costKnown ? fmtMoney(data.totals.cost_micros) : '—'}
              hint={costIssues.length > 0 ? costIssues.join(' · ') : '全部已计价'}
              warning={costIssues.length > 0}
            />
          </div>

          <div className="eg-grid [grid-template-columns:repeat(auto-fit,minmax(min(100%,24rem),1fr))]">
            <TrendCard daily={daily} />
            <PriceCard />
          </div>

          <div className="eg-grid eg-grid-triple">
            <TopCard title="Top 渠道" rows={data.top_channels} />
            <TopCard title="Top 模型" rows={data.top_models} />
            <TopCard title="Top 令牌" rows={data.top_tokens} />
          </div>
        </>
      )}
    </div>
  )
}

function Metric({ label, value, hint, warning }: { label: string; value: string; hint: string; warning?: boolean }) {
  return (
    <Card>
      <div className="text-xs text-muted">{label}</div>
      <div className={`mt-1 text-2xl font-bold ${warning ? 'text-warning-text' : 'text-foreground'}`}>{value}</div>
      <div className="mt-1 text-xs text-subtle">{hint}</div>
    </Card>
  )
}

function TrendCard({ daily }: { daily: UsageDailyPoint[] }) {
  return (
    <Card className="flex h-full flex-col">
      <div className="flex items-center justify-between">
        <h3 className="flex items-center gap-2 text-sm font-semibold"><BarChart3 size={15} className="text-brand-text" /> 日趋势</h3>
        <div className="flex items-center gap-3 text-xs text-subtle">
          <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-sm bg-brand" />请求数</span>
          <span className="flex items-center gap-1.5"><span className="h-0.5 w-3.5 rounded bg-success-solid" />成功率</span>
        </div>
      </div>
      {daily.length === 0 ? <Empty title={t('empty.usage.title')} /> : (
        <div className="mt-4 h-[clamp(224px,32vw,360px)] xl:h-auto xl:min-h-72 xl:flex-1">
          <TrendChart data={daily} className="h-full" />
        </div>
      )}
    </Card>
  )
}

/**
 * 补齐所选日期区间内没有聚合记录的天，避免缺失日期在分类轴上被压缩（趋势失真）。
 * 超过 400 天不补齐，避免长时间范围生成过多 SVG 节点。
 */
function fillDailyGaps(points: UsageDailyPoint[], start?: string, end?: string): UsageDailyPoint[] {
  if (points.length === 0 || !start || !end) return points
  const startMs = Date.parse(`${start}T00:00:00Z`)
  const endMs = Date.parse(`${end}T00:00:00Z`)
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs < startMs) return points
  const days = Math.round((endMs - startMs) / 86_400_000) + 1
  if (days <= 0 || days > 400) return points
  const byDate = new Map(points.map((point) => [point.date, point]))
  return Array.from({ length: days }, (_, i) => {
    const date = new Date(startMs + i * 86_400_000).toISOString().slice(0, 10)
    return byDate.get(date) ?? {
      date, requests: 0, success: 0, tokens_in: 0, tokens_out: 0, cost_micros: 0, cost: 0,
      priced_requests: 0, unpriced_requests: 0, no_usage_requests: 0, unknown_cost_requests: 0,
    }
  })
}

function TopCard({ title, rows }: { title: string; rows: UsageDimensionRow[] }) {
  return (
    <Card flush>
      <h3 className="border-b border-line-soft text-sm font-semibold" style={{ padding: 'var(--eg-cell-y) var(--eg-card-pad)' }}>{title}</h3>
      {rows.length === 0 ? <Empty title={t('empty.usage.title')} /> : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[300px]">
            <thead className="bg-surface-soft"><tr><Th>名称</Th><Th>请求</Th><Th>成本</Th></tr></thead>
            <tbody className="divide-y divide-line-soft">
              {rows.map((row) => (
                <tr key={`${row.id}-${row.name}`}>
                  <Td className="max-w-40 truncate font-medium" title={row.name}>{row.name}</Td>
                  <Td>{fmtInt(row.requests)}</Td>
                  <Td className="text-xs">{fmtMoney(row.cost_micros)}</Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  )
}

function PriceCard() {
  const prices = useUsagePrices()
  const channels = useChannels()
  const setDefault = useSetDefaultPrice()
  const upsert = useUpsertPriceOverride()
  const remove = useDeletePriceOverride()
  const [defaultIn, setDefaultIn] = useState('0')
  const [defaultOut, setDefaultOut] = useState('0')
  const [channelID, setChannelID] = useState('')
  const [model, setModel] = useState('')
  const [overrideIn, setOverrideIn] = useState('0')
  const [overrideOut, setOverrideOut] = useState('0')

  useEffect(() => {
    if (prices.data) {
      setDefaultIn(toPriceInput(prices.data.default?.input_micros_per_million))
      setDefaultOut(toPriceInput(prices.data.default?.output_micros_per_million))
    }
  }, [prices.data])
  useEffect(() => {
    if (!channelID && channels.data?.items[0]) setChannelID(String(channels.data.items[0].id))
  }, [channelID, channels.data])

  const channelName = useMemo(() => new Map((channels.data?.items ?? []).map((ch) => [ch.id, ch.name])), [channels.data])

  const submitDefault = (e: FormEvent) => {
    e.preventDefault()
    setDefault.mutate({ input_micros_per_million: toMicros(defaultIn), output_micros_per_million: toMicros(defaultOut) })
  }
  const submitOverride = (e: FormEvent) => {
    e.preventDefault()
    if (!channelID || !model.trim()) return
    upsert.mutate({
      channel_id: Number(channelID), model: model.trim(),
      input_micros_per_million: toMicros(overrideIn), output_micros_per_million: toMicros(overrideOut),
    }, { onSuccess: () => setModel('') })
  }

  if (prices.isPending) return <Card className="flex justify-center p-8"><Spinner /></Card>
  return (
    <Card>
      <h3 className="flex items-center gap-2 text-sm font-semibold"><CircleDollarSign size={15} className="text-success-text" /> 模型价格</h3>
      <p className="mt-1 text-xs text-subtle">单位：USD / 百万 token；价格只影响之后的新请求。</p>
      <form onSubmit={submitDefault} className="mt-4 rounded-lg border border-line bg-surface-soft p-3">
        <div className="mb-2 text-xs font-medium text-muted">全局默认</div>
        <div className="grid grid-cols-2 gap-2">
          <Field label="输入"><Input type="number" min={0} step="0.000001" value={defaultIn} onChange={(e) => setDefaultIn(e.target.value)} /></Field>
          <Field label="输出"><Input type="number" min={0} step="0.000001" value={defaultOut} onChange={(e) => setDefaultOut(e.target.value)} /></Field>
        </div>
        <Button type="submit" size="sm" className="mt-2" disabled={setDefault.isPending}>保存默认价格</Button>
      </form>

      <form onSubmit={submitOverride} className="mt-4 space-y-2 rounded-lg border border-dashed border-line-strong p-3">
        <div className="text-xs font-medium text-muted">渠道 + 模型覆盖</div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Select value={channelID} onChange={(e) => setChannelID(e.target.value)}>
            {(channels.data?.items ?? []).map((ch) => <option key={ch.id} value={ch.id}>{ch.name}</option>)}
          </Select>
          <Input value={model} onChange={(e) => setModel(e.target.value)} placeholder="上游模型名" />
          <Input type="number" min={0} step="0.000001" value={overrideIn} onChange={(e) => setOverrideIn(e.target.value)} placeholder="输入价格" />
          <Input type="number" min={0} step="0.000001" value={overrideOut} onChange={(e) => setOverrideOut(e.target.value)} placeholder="输出价格" />
        </div>
        <Button type="submit" size="sm" variant="outline" disabled={upsert.isPending || !model.trim()}><Plus size={13} /> 保存覆盖</Button>
      </form>
      <div className="mt-3 max-h-44 space-y-1 overflow-auto">
        {(prices.data?.overrides ?? []).map((price) => (
          <div key={price.id} className="flex items-center gap-2 rounded-md bg-surface-soft px-2 py-1.5 text-xs">
            <Badge tone="brand">{channelName.get(price.channel_id) || `#${price.channel_id}`}</Badge>
            <span className="min-w-0 flex-1 truncate font-medium text-foreground-soft">{price.model}</span>
            <span className="text-muted">{toPriceInput(price.input_micros_per_million)} / {toPriceInput(price.output_micros_per_million)}</span>
            <button onClick={() => remove.mutate(price.id)} className="text-subtle hover:text-danger-text" title="删除覆盖"><Trash2 size={13} /></button>
          </div>
        ))}
      </div>
      <ErrorText>{prices.isError ? (prices.error as Error).message : setDefault.isError ? (setDefault.error as Error).message : upsert.isError ? (upsert.error as Error).message : null}</ErrorText>
    </Card>
  )
}

function toMicros(value: string): number {
  return Math.max(0, Math.round((Number(value) || 0) * 1_000_000))
}

function toPriceInput(micros: number | undefined): string {
  return ((micros || 0) / 1_000_000).toFixed(6).replace(/\.?0+$/, '')
}


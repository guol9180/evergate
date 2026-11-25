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
import { t, tf } from '../lib/i18n'

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
    data.totals.unpriced_requests > 0 ? tf('usage.unpricedCount', { n: fmtInt(data.totals.unpriced_requests) }) : '',
    data.totals.no_usage_requests > 0 ? tf('usage.noUsageCount', { n: fmtInt(data.totals.no_usage_requests) }) : '',
    data.totals.unknown_cost_requests > 0 ? tf('usage.unknownCount', { n: fmtInt(data.totals.unknown_cost_requests) }) : '',
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
          <Field label={t('logs.filter.start')}><Input type="date" value={start} max={end} onChange={(e) => setStart(e.target.value)} /></Field>
          <Field label={t('logs.filter.end')}><Input type="date" value={end} min={start} max={today()} onChange={(e) => setEnd(e.target.value)} /></Field>
          <Field label={t('logs.filter.channel')}>
            <Select value={channelID} onChange={(e) => setChannelID(e.target.value)}>
              <option value="">{t('logs.filter.allChannels')}</option>
              {chs.map((ch) => <option key={ch.id} value={ch.id}>{ch.name}</option>)}
            </Select>
          </Field>
          <Field label={t('logs.filter.model')}><Input value={model} onChange={(e) => setModel(e.target.value)} placeholder={t('tokens.allModels')} /></Field>
          <Field label={t('logs.filter.token')}>
            <Select value={tokenID} onChange={(e) => setTokenID(e.target.value)}>
              <option value="">{t('logs.filter.allTokens')}</option>
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
              <div className="font-semibold">{t('usage.noPrice.title')}</div>
              <p className="mt-1 text-xs leading-5 text-warning-text">
                {t('usage.noPrice.body1')}
                {t('usage.noPrice.body2')}
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
            <Metric label={t('usage.requests')} value={fmtInt(data.totals.requests)} hint={tf('usage.successCount', { n: fmtInt(data.totals.success) })} />
            <Metric label={t('dashboard.kpi.successRate')} value={successRate == null ? '—' : `${successRate.toFixed(1)}%`} hint="2xx / 3xx" />
            <Metric label={t('usage.tokens')} value={fmtInt(data.totals.tokens_in + data.totals.tokens_out)} hint={tf('usage.tokensHint', { input: fmtInt(data.totals.tokens_in), output: fmtInt(data.totals.tokens_out) })} />
            <Metric
              label={t('table.cost')}
              value={costKnown ? fmtMoney(data.totals.cost_micros) : '—'}
              hint={costIssues.length > 0 ? costIssues.join(' · ') : t('usage.allPriced')}
              warning={costIssues.length > 0}
            />
          </div>

          <div className="eg-grid [grid-template-columns:repeat(auto-fit,minmax(min(100%,24rem),1fr))]">
            <TrendCard daily={daily} />
            <PriceCard />
          </div>

          <div className="eg-grid eg-grid-triple">
            <TopCard title={t('usage.topChannels')} rows={data.top_channels} />
            <TopCard title={t('usage.topModels')} rows={data.top_models} />
            <TopCard title={t('usage.topTokens')} rows={data.top_tokens} />
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
        <h3 className="flex items-center gap-2 text-sm font-semibold"><BarChart3 size={15} className="text-brand-text" /> {t('usage.trend')}</h3>
        <div className="flex items-center gap-3 text-xs text-subtle">
          <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-sm bg-brand" />{t('chart.requests')}</span>
          <span className="flex items-center gap-1.5"><span className="h-0.5 w-3.5 rounded bg-success-solid" />{t('chart.successRate')}</span>
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
            <thead className="bg-surface-soft"><tr><Th>{t('table.name')}</Th><Th>{t('table.requests')}</Th><Th>{t('table.cost')}</Th></tr></thead>
            <tbody className="divide-y divide-line-soft">
              {rows.map((row) => (
                <tr key={`${row.id}-${row.name}`}>
                  <Td className="max-w-40 truncate font-medium" title={row.name}>{dimensionName(row)}</Td>
                  <Td>{fmtInt(row.requests)}</Td>
                  <Td className="text-xs">
                    {row.priced_requests === 0
                      ? <span className="text-subtle" title={t('usage.topCostHint')}>—</span>
                      : fmtMoney(row.cost_micros)}
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  )
}

// 服务端对合成行（未命中渠道 / 调试台无令牌 / 已删除）只打标不翻译，由前端按当前语言显示。
function dimensionName(row: UsageDimensionRow): string {
  if (row.name_kind === 'unattributed') return t('usage.row.unattributed')
  if (row.name_kind === 'playground') return t('usage.row.playground')
  if (row.name_kind === 'deleted') return tf('usage.row.deleted', { name: row.name })
  return row.name
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
      <h3 className="flex items-center gap-2 text-sm font-semibold"><CircleDollarSign size={15} className="text-success-text" /> {t('usage.price.title')}</h3>
      <p className="mt-1 text-xs text-subtle">{t('usage.price.unit')}</p>
      <form onSubmit={submitDefault} className="mt-4 rounded-lg border border-line bg-surface-soft p-3">
        <div className="mb-2 text-xs font-medium text-muted">{t('usage.price.globalDefault')}</div>
        <div className="grid grid-cols-2 gap-2">
          <Field label={t('usage.input')}><Input type="number" min={0} step="0.000001" value={defaultIn} onChange={(e) => setDefaultIn(e.target.value)} /></Field>
          <Field label={t('usage.output')}><Input type="number" min={0} step="0.000001" value={defaultOut} onChange={(e) => setDefaultOut(e.target.value)} /></Field>
        </div>
        <Button type="submit" size="sm" className="mt-2" disabled={setDefault.isPending}>{t('usage.price.saveDefault')}</Button>
      </form>

      <form onSubmit={submitOverride} className="mt-4 space-y-2 rounded-lg border border-dashed border-line-strong p-3">
        <div className="text-xs font-medium text-muted">{t('usage.price.override')}</div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Field label={t('table.channel')}>
            <Select value={channelID} onChange={(e) => setChannelID(e.target.value)}>
              {(channels.data?.items ?? []).map((ch) => <option key={ch.id} value={ch.id}>{ch.name}</option>)}
            </Select>
          </Field>
          <Field label={t('usage.price.upstreamModel')}><Input value={model} onChange={(e) => setModel(e.target.value)} placeholder={t('usage.price.upstreamPlaceholder')} /></Field>
          <Field label={t('usage.price.inputPrice')}><Input type="number" min={0} step="0.000001" value={overrideIn} onChange={(e) => setOverrideIn(e.target.value)} placeholder={t('usage.price.useDefault')} /></Field>
          <Field label={t('usage.price.outputPrice')}><Input type="number" min={0} step="0.000001" value={overrideOut} onChange={(e) => setOverrideOut(e.target.value)} placeholder={t('usage.price.useDefault')} /></Field>
        </div>
        <Button type="submit" size="sm" variant="outline" disabled={upsert.isPending || !model.trim()}><Plus size={13} /> {t('usage.price.saveOverride')}</Button>
      </form>
      <div className="mt-3 max-h-44 space-y-1 overflow-auto">
        {(prices.data?.overrides ?? []).map((price) => (
          <div key={price.id} className="flex items-center gap-2 rounded-md bg-surface-soft px-2 py-1.5 text-xs">
            <Badge tone="brand">{channelName.get(price.channel_id) || `#${price.channel_id}`}</Badge>
            <span className="min-w-0 flex-1 truncate font-medium text-foreground-soft">{price.model}</span>
            <span className="text-muted">{toPriceInput(price.input_micros_per_million)} / {toPriceInput(price.output_micros_per_million)}</span>
            <button onClick={() => remove.mutate(price.id)} className="text-subtle hover:text-danger-text" title={t('usage.price.deleteOverride')}><Trash2 size={13} /></button>
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


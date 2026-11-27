import { useEffect, useMemo, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { BarChart3, CircleDollarSign, Plus, RefreshCw, Trash2 } from 'lucide-react'
import {
  useApplyPresetPrice,
  useChannel,
  useChannels,
  useDeletePriceOverride,
  useSetDefaultPrice,
  useTokens,
  useUpsertPriceOverride,
  useUsagePrices,
  useUsageReport,
} from '../api/hooks'
import type { ModelPrice, UsageDailyPoint, UsageDimensionRow } from '../api/types'
import { bucketByLocalDay, localDateKey } from '../lib/usage'
import { Alert, Badge, Button, Card, Empty, Field, IconButton, Input, PageHeader, SegmentedControl, Select, Spinner, Td, Th } from '../components/ui'
import { notify } from '../components/Toaster'
import { TREND_METRICS, TrendChart, type TrendMetric } from '../components/TrendChart'
import PriceTable, { fmtPriceMicros } from '../components/PriceTable'
import { fmtInt, fmtMoney, fmtMs } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'
import { errorMessage } from '../lib/errors'

// 日期默认值与上限一律按本地日：toISOString 取的是 UTC 日期，东八区 0–8 点会落到昨天
const today = () => localDateKey(new Date())
const daysAgo = (days: number) => {
  const d = new Date()
  d.setDate(d.getDate() - days)
  return localDateKey(d)
}

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
  // 小时点按本地日分桶后再补齐空天：与概览页「今天」同一口径
  const daily = useMemo(() => fillDailyGaps(bucketByLocalDay(data?.hourly ?? []), data?.start, data?.end), [data])
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
          <Button variant="outline" onClick={() => report.refetch()} loading={report.isFetching}>
            <RefreshCw size={14} /> {t('common.refresh')}
          </Button>
        }
      />

      <Card>
        <div className="eg-grid eg-grid-auto">
          <Field label={t('logs.filter.start')}><Input type="date" value={start} max={end} onChange={(e) => setStart(e.target.value)} /></Field>
          <Field label={t('logs.filter.end')}><Input type="date" value={end} min={start} max={today()} onChange={(e) => setEnd(e.target.value)} /></Field>
          <Field label={t('logs.filter.channel')}>
            <Select
              value={channelID}
              onValueChange={setChannelID}
              options={[{ value: '', label: t('logs.filter.allChannels') }, ...chs.map((ch) => ({ value: String(ch.id), label: ch.name }))]}
            />
          </Field>
          <Field label={t('logs.filter.model')}><Input value={model} onChange={(e) => setModel(e.target.value)} placeholder={t('tokens.allModels')} /></Field>
          <Field label={t('logs.filter.token')}>
            <Select
              value={tokenID}
              onValueChange={setTokenID}
              options={[{ value: '', label: t('logs.filter.allTokens') }, ...tks.map((token) => ({ value: String(token.id), label: token.name }))]}
            />
          </Field>
        </div>
      </Card>

      {report.isError && <Alert tone="danger">{errorMessage(report.error)}</Alert>}

      {prices.data && !prices.data.default && prices.data.overrides.length === 0 && (
        <Alert tone="warning" title={t('usage.noPrice.title')}>
          <p>{t('usage.noPrice.body1')}{t('usage.noPrice.body2')}</p>
        </Alert>
      )}

      {report.isPending ? (
        <div className="flex justify-center py-16"><Spinner className="h-6 w-6" /></div>
      ) : !data ? null : (
        <>
          <div className="eg-grid eg-grid-kpi">
            <Metric label={t('usage.requests')} value={fmtInt(data.totals.requests)} hint={tf('usage.successCount', { n: fmtInt(data.totals.success) })} />
            <Metric label={t('dashboard.kpi.successRate')} value={successRate == null ? '—' : `${successRate.toFixed(1)}%`} hint="2xx / 3xx" />
            <Metric
              label={t('usage.tokens')}
              value={fmtInt(data.totals.tokens_in + data.totals.tokens_out)}
              hint={tf('usage.tokensHint', { input: fmtInt(data.totals.tokens_in), output: fmtInt(data.totals.tokens_out) })
                + (data.totals.cache_read_tokens ? tf('usage.tokensHintCached', { cached: fmtInt(data.totals.cache_read_tokens) }) : '')}
            />
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
            <TopCard title={t('usage.topChannels')} rows={data.top_channels} latency />
            <TopCard title={t('usage.topModels')} rows={data.top_models} latency />
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
  const [metric, setMetric] = useState<TrendMetric>('requests')
  return (
    <Card className="flex flex-col self-start">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold"><BarChart3 size={15} className="text-brand-text" /> {t('usage.trend')}</h3>
        <SegmentedControl
          label={t('chart.metric')}
          value={metric}
          onChange={setMetric}
          options={TREND_METRICS.map((item) => ({ value: item.value, label: t(item.label) }))}
          size="sm"
          className="grid-cols-2 sm:grid-cols-4"
        />
      </div>
      <div className="mt-2 flex items-center gap-3 text-xs text-subtle">
        <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-sm bg-brand" />{t(`chart.metric.${metric}`)}</span>
          <span className="flex items-center gap-1.5"><span className="h-0.5 w-3.5 rounded bg-success-solid" />{t('chart.successRate')}</span>
      </div>
      {daily.length === 0 ? <Empty title={t('empty.usage.title')} /> : (
        <div className="mt-4 h-[clamp(224px,32vw,360px)]">
          <TrendChart data={daily} metric={metric} className="h-full" />
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

function TopCard({ title, rows, latency }: { title: string; rows: UsageDimensionRow[]; latency?: boolean }) {
  return (
    <Card flush>
      <h3 className="border-b border-line-soft text-sm font-semibold" style={{ padding: 'var(--eg-cell-y) var(--eg-card-pad)' }}>{title}</h3>
      {rows.length === 0 ? <Empty title={t('empty.usage.title')} /> : (
        <div className="overflow-x-auto">
          <table className={`w-full ${latency ? 'min-w-[340px]' : 'min-w-[300px]'}`}>
            <thead className="bg-surface-soft">
              <tr>
                <Th>{t('table.name')}</Th><Th>{t('table.requests')}</Th>
                {latency && <Th title={t('usage.firstTokenHint')}>{t('table.firstToken')}</Th>}
                <Th>{t('table.cost')}</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {rows.map((row) => (
                <tr key={`${row.id}-${row.name}`}>
                  <Td className="max-w-40 truncate font-medium" title={row.name}>{dimensionName(row)}</Td>
                  <Td>{fmtInt(row.requests)}</Td>
                  {latency && (
                    <Td className="text-xs" title={t('usage.firstTokenHint')}>
                      {row.first_token_avg_ms == null
                        ? <span className="text-subtle">—</span>
                        : `${fmtMs(row.first_token_avg_ms)} / ${fmtMs(row.first_token_p50_ms ?? undefined)}`}
                    </Td>
                  )}
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
  const applyPreset = useApplyPresetPrice()
  // 价格按美元 / 百万 token 的字符串编辑，提交时转微美元；非负校验文案走 i18n
  const price = (optional: boolean) => z.string().trim().refine((value) => (optional && value === '') || (value !== '' && Number.isFinite(Number(value)) && Number(value) >= 0), t('usage.price.invalid'))
  const defaultSchema = z.object({
    defaultIn: price(false), defaultOut: price(false), defaultCacheRead: price(true), defaultCacheWrite: price(true),
  })
  const overrideSchema = z.object({
    channelID: z.string().min(1, t('usage.price.channelRequired')),
    model: z.string().trim().min(1, t('usage.price.modelRequired')),
    overrideIn: price(false), overrideOut: price(false), overrideCacheRead: price(true), overrideCacheWrite: price(true),
  })
  const defaultForm = useForm<z.infer<typeof defaultSchema>>({
    resolver: zodResolver(defaultSchema),
    values: prices.data ? {
      defaultIn: toPriceInput(prices.data.default?.input_micros_per_million),
      defaultOut: toPriceInput(prices.data.default?.output_micros_per_million),
      defaultCacheRead: toOptionalPriceInput(prices.data.default?.cache_read_micros_per_million),
      defaultCacheWrite: toOptionalPriceInput(prices.data.default?.cache_write_micros_per_million),
    } : undefined,
  })
  const overrideForm = useForm<z.infer<typeof overrideSchema>>({
    resolver: zodResolver(overrideSchema),
    defaultValues: { channelID: '', model: '', overrideIn: '0', overrideOut: '0', overrideCacheRead: '', overrideCacheWrite: '' },
  })
  const channelID = overrideForm.watch('channelID')
  const model = overrideForm.watch('model')
  const [presetChannelID, setPresetChannelID] = useState('')
  const presetChannels = useMemo(
    () => (channels.data?.items ?? []).filter((ch) => ch.preset_id),
    [channels.data],
  )
  // 默认挑一个已经填了模型的预设渠道，避免一进来就是空表
  const presetDefault = presetChannels.find((ch) => (ch.models ?? []).length > 0) ?? presetChannels[0]
  const presetDetail = useChannel(presetChannelID ? Number(presetChannelID) : null)

  // 覆盖表单默认落到第一个渠道（有预设渠道时优先在下方 preset 浏览里处理）
  useEffect(() => {
    if (!channelID && channels.data?.items[0]) overrideForm.setValue('channelID', String(channels.data.items[0].id))
  }, [channelID, channels.data, overrideForm])
  useEffect(() => {
    if (!presetChannelID && presetDefault) setPresetChannelID(String(presetDefault.id))
  }, [presetChannelID, presetDefault])

  const channelName = useMemo(() => new Map((channels.data?.items ?? []).map((ch) => [ch.id, ch.name])), [channels.data])

  const submitDefault = defaultForm.handleSubmit(
    (values) => setDefault.mutate(
      {
        input_micros_per_million: toMicros(values.defaultIn), output_micros_per_million: toMicros(values.defaultOut),
        cache_read_micros_per_million: toMicros(values.defaultCacheRead), cache_write_micros_per_million: toMicros(values.defaultCacheWrite),
      },
      { onSuccess: () => notify.success(t('toast.price.saved')) },
    ),
    (errors) => defaultForm.setFocus(Object.keys(errors)[0] as 'defaultIn'),
  )
  const submitOverride = overrideForm.handleSubmit(
    (values) => upsert.mutate({
      channel_id: Number(values.channelID), model: values.model.trim(),
      input_micros_per_million: toMicros(values.overrideIn), output_micros_per_million: toMicros(values.overrideOut),
      cache_read_micros_per_million: toMicros(values.overrideCacheRead), cache_write_micros_per_million: toMicros(values.overrideCacheWrite),
    }, {
      onSuccess: () => {
        notify.success(t('toast.price.saved'))
        overrideForm.resetField('model')
      }
    }),
    (errors) => overrideForm.setFocus(Object.keys(errors)[0] as 'model'),
  )

  if (prices.isPending) return <Card className="flex justify-center p-8"><Spinner /></Card>
  return (
    <Card>
      <h3 className="flex items-center gap-2 text-sm font-semibold"><CircleDollarSign size={15} className="text-success-text" /> {t('usage.price.title')}</h3>
      <p className="mt-1 text-xs text-subtle">{t('usage.price.unit')}</p>
      {prices.isError && <Alert tone="danger" className="mt-3">{errorMessage(prices.error)}</Alert>}
      <form onSubmit={submitDefault} className="mt-4 rounded-lg border border-line bg-surface-soft p-3">
        <div className="mb-2 text-xs font-medium text-muted">{t('usage.price.globalDefault')}</div>
        <div className="grid grid-cols-2 gap-2">
          <Field label={t('usage.input')} error={defaultForm.formState.errors.defaultIn?.message}><Input type="number" min={0} step="0.000001" {...defaultForm.register('defaultIn')} /></Field>
          <Field label={t('usage.output')} error={defaultForm.formState.errors.defaultOut?.message}><Input type="number" min={0} step="0.000001" {...defaultForm.register('defaultOut')} /></Field>
          <Field label={t('usage.price.cacheRead')} error={defaultForm.formState.errors.defaultCacheRead?.message}><Input type="number" min={0} step="0.000001" {...defaultForm.register('defaultCacheRead')} placeholder={t('usage.price.cacheFollowInput')} /></Field>
          <Field label={t('usage.price.cacheWrite')} error={defaultForm.formState.errors.defaultCacheWrite?.message}><Input type="number" min={0} step="0.000001" {...defaultForm.register('defaultCacheWrite')} placeholder={t('usage.price.cacheFollowInput')} /></Field>
        </div>
        <p className="mb-2 text-[11px] leading-4 text-subtle">{t('usage.price.cacheHint')}</p>
        <Button type="submit" size="sm" className="mt-2" loading={setDefault.isPending}>{t('usage.price.saveDefault')}</Button>
      </form>

      <form onSubmit={submitOverride} className="mt-4 space-y-2 rounded-lg border border-dashed border-line-strong p-3">
        <div className="text-xs font-medium text-muted">{t('usage.price.override')}</div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <Field label={t('table.channel')} error={overrideForm.formState.errors.channelID?.message}>
            <Controller
              control={overrideForm.control}
              name="channelID"
              render={({ field }) => (
                <Select
                  value={field.value}
                  onValueChange={field.onChange}
                  options={(channels.data?.items ?? []).map((ch) => ({ value: String(ch.id), label: ch.name }))}
                />
              )}
            />
          </Field>
          <Field label={t('usage.price.upstreamModel')} error={overrideForm.formState.errors.model?.message}><Input {...overrideForm.register('model')} placeholder={t('usage.price.upstreamPlaceholder')} /></Field>
          <Field label={t('usage.price.inputPrice')} error={overrideForm.formState.errors.overrideIn?.message}><Input type="number" min={0} step="0.000001" {...overrideForm.register('overrideIn')} placeholder={t('usage.price.useDefault')} /></Field>
          <Field label={t('usage.price.outputPrice')} error={overrideForm.formState.errors.overrideOut?.message}><Input type="number" min={0} step="0.000001" {...overrideForm.register('overrideOut')} placeholder={t('usage.price.useDefault')} /></Field>
          <Field label={t('usage.price.cacheRead')} error={overrideForm.formState.errors.overrideCacheRead?.message}><Input type="number" min={0} step="0.000001" {...overrideForm.register('overrideCacheRead')} placeholder={t('usage.price.cacheFollowInput')} /></Field>
          <Field label={t('usage.price.cacheWrite')} error={overrideForm.formState.errors.overrideCacheWrite?.message}><Input type="number" min={0} step="0.000001" {...overrideForm.register('overrideCacheWrite')} placeholder={t('usage.price.cacheFollowInput')} /></Field>
        </div>
        <p className="text-[11px] leading-4 text-subtle">{t('usage.price.cacheHint')}</p>
        <Button type="submit" size="sm" variant="outline" loading={upsert.isPending} disabled={!model.trim()}><Plus size={13} /> {t('usage.price.saveOverride')}</Button>
      </form>
      <div className="mt-3 max-h-44 space-y-1 overflow-auto">
        {(prices.data?.overrides ?? []).map((price) => (
          <div key={price.id} className="flex items-center gap-2 rounded-md bg-surface-soft px-2 py-1.5 text-xs">
            <Badge tone="brand">{channelName.get(price.channel_id) || `#${price.channel_id}`}</Badge>
            <span className="min-w-0 flex-1 truncate font-medium text-foreground-soft">{price.model}</span>
            <span className="text-muted">{fmtPriceMicros(price.input_micros_per_million)} / {fmtPriceMicros(price.output_micros_per_million)}{cacheSummary(price)}</span>
            <IconButton danger label={t('usage.price.deleteOverride')} loading={remove.isPending && remove.variables === price.id} onClick={() => remove.mutate(price.id, { onSuccess: () => notify.success(t('toast.price.deleted')) })}>
              <Trash2 size={13} />
            </IconButton>
          </div>
        ))}
      </div>
      <div className="mt-4 rounded-lg border border-line bg-surface-soft p-3">
        <div className="text-xs font-medium text-muted">{t('usage.price.byPreset')}</div>
        <p className="mt-1 text-[11px] text-subtle">{t('usage.price.byPresetHint')}</p>
        {presetChannels.length === 0 ? (
          <p className="mt-2 text-xs text-subtle">{t('usage.price.byPresetNone')}</p>
        ) : (
          <>
            <div className="mt-2 flex flex-wrap items-end gap-2">
              <div className="min-w-40 flex-1">
                <Field label={t('table.channel')}>
                  <Select
                    value={presetChannelID}
                    onValueChange={(next) => {
                      setPresetChannelID(next)
                      applyPreset.reset()
                    }}
                    options={presetChannels.map((ch) => ({ value: String(ch.id), label: ch.name }))}
                  />
                </Field>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                loading={applyPreset.isPending}
                disabled={!presetChannelID}
                onClick={() => applyPreset.mutate(Number(presetChannelID), {
                  onSuccess: (resp) => {
                    if (resp.created > 0) notify.success(tf('usage.price.applyDone', { n: resp.created }))
                    else notify.info(t('usage.price.applyNone'))
                  }
                })}
              >
                {t('usage.price.applyPreset')}
              </Button>
            </div>
            {prices.data?.catalog && (
              <div className="mt-2 text-[11px] text-subtle">{tf('usage.price.asOf', { asOf: prices.data.catalog.as_of })}</div>
            )}
            <div className="mt-2">
              {presetDetail.isError ? (
                <Alert tone="danger">{errorMessage(presetDetail.error)}</Alert>
              ) : presetDetail.isPending ? (
                <div className="flex justify-center py-3"><Spinner /></div>
              ) : presetDetail.data && (presetDetail.data.prices?.length ?? 0) > 0 ? (
                <PriceTable prices={presetDetail.data.prices ?? []} />
              ) : (
                <p className="text-xs text-subtle">{t('usage.price.byPresetEmpty')}</p>
              )}
            </div>
          </>
        )}
      </div>
    </Card>
  )
}

function toMicros(value: string): number {
  return Math.max(0, Math.round((Number(value) || 0) * 1_000_000))
}

function toPriceInput(micros: number | undefined): string {
  return ((micros || 0) / 1_000_000).toFixed(6).replace(/\.?0+$/, '')
}

/** 缓存价输入框初值：未设置（0）留空，对应「按输入价计」。 */
function toOptionalPriceInput(micros: number | undefined): string {
  return micros && micros > 0 ? toPriceInput(micros) : ''
}

/** 覆盖列表里的缓存价摘要：只列出已设置的一侧，未设置的不占位。 */
function cacheSummary(price: ModelPrice): string {
  const parts: string[] = []
  if (price.cache_read_micros_per_million > 0) parts.push(`${t('usage.price.cacheRead')} ${fmtPriceMicros(price.cache_read_micros_per_million)}`)
  if (price.cache_write_micros_per_million > 0) parts.push(`${t('usage.price.cacheWrite')} ${fmtPriceMicros(price.cache_write_micros_per_million)}`)
  return parts.length ? ` · ${parts.join(' · ')}` : ''
}


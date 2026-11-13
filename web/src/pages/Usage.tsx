import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { BarChart3, CircleDollarSign, Plus, RefreshCw, Trash2 } from 'lucide-react'
import {
  useChannels,
  useDeletePriceOverride,
  useSetDefaultPrice,
  useTokens,
  useUpsertPriceOverride,
  useUsagePrices,
  useUsageReport,
} from '../api/hooks'
import type { UsageDimensionRow } from '../api/types'
import { Badge, Button, Card, Empty, ErrorText, Field, Input, PageHeader, Select, Spinner, Td, Th } from '../components/ui'
import { fmtInt } from '../lib/format'

const today = () => new Date().toISOString().slice(0, 10)
const daysAgo = (days: number) => new Date(Date.now() - days * 86_400_000).toISOString().slice(0, 10)

export default function Usage() {
  const [start, setStart] = useState(daysAgo(6))
  const [end, setEnd] = useState(today())
  const [channelID, setChannelID] = useState('')
  const [model, setModel] = useState('')
  const [tokenID, setTokenID] = useState('')
  const channels = useChannels()
  const tokens = useTokens()
  const report = useUsageReport({ start, end, channel_id: channelID, model, token_id: tokenID })

  const chs = channels.data?.items ?? []
  const tks = tokens.data?.items ?? []
  const data = report.data
  const successRate = data?.totals.requests ? (data.totals.success / data.totals.requests) * 100 : null

  return (
    <div className="space-y-5">
      <PageHeader
        title="用量成本"
        description="按 UTC 日聚合请求、token 与价格快照。"
        actions={
          <Button variant="outline" onClick={() => report.refetch()} disabled={report.isFetching}>
            <RefreshCw size={14} className={report.isFetching ? 'animate-spin' : ''} /> 刷新
          </Button>
        }
      />

      <Card className="p-4">
        <div className="grid gap-3 md:grid-cols-5">
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

      {report.isPending ? (
        <div className="flex justify-center py-16"><Spinner className="h-6 w-6" /></div>
      ) : !data ? null : (
        <>
          <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
            <Metric label="请求数" value={fmtInt(data.totals.requests)} hint={`成功 ${fmtInt(data.totals.success)}`} />
            <Metric label="成功率" value={successRate == null ? '—' : `${successRate.toFixed(1)}%`} hint="2xx / 3xx" />
            <Metric label="Token" value={fmtInt(data.totals.tokens_in + data.totals.tokens_out)} hint={`输入 ${fmtInt(data.totals.tokens_in)} · 输出 ${fmtInt(data.totals.tokens_out)}`} />
            <Metric
              label="成本"
              value={fmtMoney(data.totals.cost_micros)}
              hint={data.totals.unpriced_requests > 0 ? `${fmtInt(data.totals.unpriced_requests)} 条未计价` : '全部已计价'}
              warning={data.totals.unpriced_requests > 0}
            />
          </div>

          <div className="grid gap-5 xl:grid-cols-[1.3fr_1fr]">
            <TrendCard daily={data.daily} />
            <PriceCard />
          </div>

          <div className="grid gap-5 lg:grid-cols-3">
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
    <Card className="p-4">
      <div className="text-xs text-slate-500">{label}</div>
      <div className={`mt-1 text-2xl font-bold ${warning ? 'text-amber-600' : 'text-slate-900'}`}>{value}</div>
      <div className="mt-1 text-xs text-slate-400">{hint}</div>
    </Card>
  )
}

function TrendCard({ daily }: { daily: { date: string; requests: number; success: number; cost_micros: number }[] }) {
  const max = Math.max(1, ...daily.map((point) => point.requests))
  return (
    <Card className="p-4">
      <div className="flex items-center justify-between">
        <h3 className="flex items-center gap-2 text-sm font-semibold"><BarChart3 size={15} className="text-indigo-500" /> 日趋势</h3>
        <span className="text-xs text-slate-400">柱高 = 请求数</span>
      </div>
      {daily.length === 0 ? <Empty title="所选范围暂无数据" /> : (
        <div className="mt-5 flex h-44 items-end gap-1.5">
          {daily.map((point) => (
            <div key={point.date} className="group flex min-w-0 flex-1 flex-col items-center justify-end gap-1" title={`${point.date} · ${point.requests} 请求 · ${fmtMoney(point.cost_micros)}`}>
              <div
                className="w-full max-w-12 rounded-t bg-indigo-500/80 transition-colors group-hover:bg-indigo-600"
                style={{ height: `${Math.max(3, (point.requests / max) * 145)}px` }}
              />
              <span className="max-w-full truncate text-[10px] text-slate-400">{point.date.slice(5)}</span>
            </div>
          ))}
        </div>
      )}
    </Card>
  )
}

function TopCard({ title, rows }: { title: string; rows: UsageDimensionRow[] }) {
  return (
    <Card>
      <h3 className="border-b border-slate-100 px-4 py-3 text-sm font-semibold">{title}</h3>
      {rows.length === 0 ? <Empty title="暂无数据" /> : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[300px]">
            <thead className="bg-slate-50/60"><tr><Th>名称</Th><Th>请求</Th><Th>成本</Th></tr></thead>
            <tbody className="divide-y divide-slate-50">
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
    <Card className="p-4">
      <h3 className="flex items-center gap-2 text-sm font-semibold"><CircleDollarSign size={15} className="text-emerald-500" /> 模型价格</h3>
      <p className="mt-1 text-xs text-slate-400">单位：USD / 百万 token；价格只影响之后的新请求。</p>
      <form onSubmit={submitDefault} className="mt-4 rounded-lg border border-slate-200 bg-slate-50/50 p-3">
        <div className="mb-2 text-xs font-medium text-slate-600">全局默认</div>
        <div className="grid grid-cols-2 gap-2">
          <Field label="输入"><Input type="number" min={0} step="0.000001" value={defaultIn} onChange={(e) => setDefaultIn(e.target.value)} /></Field>
          <Field label="输出"><Input type="number" min={0} step="0.000001" value={defaultOut} onChange={(e) => setDefaultOut(e.target.value)} /></Field>
        </div>
        <Button type="submit" size="sm" className="mt-2" disabled={setDefault.isPending}>保存默认价格</Button>
      </form>

      <form onSubmit={submitOverride} className="mt-4 space-y-2 rounded-lg border border-dashed border-slate-300 p-3">
        <div className="text-xs font-medium text-slate-600">渠道 + 模型覆盖</div>
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
          <div key={price.id} className="flex items-center gap-2 rounded-md bg-slate-50 px-2 py-1.5 text-xs">
            <Badge tone="bg-indigo-100 text-indigo-700">{channelName.get(price.channel_id) || `#${price.channel_id}`}</Badge>
            <span className="min-w-0 flex-1 truncate font-medium text-slate-700">{price.model}</span>
            <span className="text-slate-500">{toPriceInput(price.input_micros_per_million)} / {toPriceInput(price.output_micros_per_million)}</span>
            <button onClick={() => remove.mutate(price.id)} className="text-slate-400 hover:text-red-500" title="删除覆盖"><Trash2 size={13} /></button>
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

function fmtMoney(micros: number): string {
  return `$${(micros / 1_000_000).toFixed(6).replace(/\.?0+$/, '')}`
}

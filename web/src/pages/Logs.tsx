import { type ReactNode, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, ChevronDown, ChevronUp, Download, Filter, RefreshCw, RotateCcw, ScrollText, Search } from 'lucide-react'
import { buildLogQuery, downloadAdminFile, useChannels, useLogs, useTokens } from '../api/hooks'
import { Alert, Badge, Button, Card, ContentSwap, CopyButton, Disclosure, Drawer, Empty, Field, Input, PageHeader, Pagination, Select, SkeletonRows, StatusBadge, Switch, Td, Th } from '../components/ui'
import { notify } from '../components/Toaster'
import RetryTimeline, { lastAttempt } from '../components/RetryTimeline'
import { PriceSourceBadge } from '../components/PriceTable'
import type { RequestLog } from '../api/types'
import { describeUpstreamError, fmtInt, fmtMs, fmtRequestCost, fmtTime, httpStatusTone, kindText, statusTone } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'

const AUTO_REFRESH_MS = 5000

// 日志时间以 UTC 存储；筛选输入用本地日期，提交时转 RFC3339 区间。
function startOfDayRFC3339(date: string): string | undefined {
  if (!date) return undefined
  const d = new Date(`${date}T00:00:00`)
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString()
}

function endOfDayRFC3339(date: string): string | undefined {
  if (!date) return undefined
  const d = new Date(`${date}T23:59:59.999`)
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString()
}

/** 详情里「按此筛选」只改对应条件，其余保留。 */
type DetailFilter = { channel_id?: string; token_id?: string; model?: string }

export default function Logs() {
  usePageTitle(t('page.logs.title'))
  const [model, setModel] = useState('')
  const [status, setStatus] = useState('')
  const [channelId, setChannelId] = useState('')
  const [tokenId, setTokenId] = useState('')
  const [startDate, setStartDate] = useState('')
  const [endDate, setEndDate] = useState('')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [exporting, setExporting] = useState(false)
  const [selected, setSelected] = useState<RequestLog | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const channels = useChannels()
  const tokens = useTokens()

  const filters = {
    model: model || undefined,
    status: status || undefined,
    channel_id: channelId || undefined,
    token_id: tokenId || undefined,
    start: startOfDayRFC3339(startDate),
    end: endOfDayRFC3339(endDate),
  }
  const logs = useLogs({ ...filters, page, page_size: pageSize }, autoRefresh ? AUTO_REFRESH_MS : false)

  const total = logs.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const resetPage = <T,>(setter: (v: T) => void) => (v: T) => { setter(v); setPage(1) }
  const hasFilters = Boolean(model || status || channelId || tokenId || startDate || endDate)
  const clearFilters = () => {
    setModel(''); setStatus(''); setChannelId(''); setTokenId(''); setStartDate(''); setEndDate(''); setPage(1)
  }
  // 轮询期间的 isFetching 不算「手动刷新」：只有用户点了按钮才转圈
  const refresh = async () => {
    setRefreshing(true)
    try { await logs.refetch() } finally { setRefreshing(false) }
  }
  const applyDetailFilter = (patch: DetailFilter) => {
    if (patch.channel_id !== undefined) resetPage(setChannelId)(patch.channel_id)
    if (patch.token_id !== undefined) resetPage(setTokenId)(patch.token_id)
    if (patch.model !== undefined) resetPage(setModel)(patch.model)
    setSelected(null)
  }

  const exportCsv = async () => {
    setExporting(true)
    try {
      await downloadAdminFile(`/admin/api/logs/export?${buildLogQuery(filters)}`, 'evergate-logs.csv')
    } catch (err) {
      notify.error(err, t('logs.exportFailed'))
    } finally {
      setExporting(false)
    }
  }

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.logs.title')}
        description={t('page.logs.desc')}
        actions={(
          <>
            <label htmlFor="logs-auto-refresh" className="flex h-9 cursor-pointer items-center gap-2 rounded-md border border-line-strong bg-surface px-2.5 text-xs text-foreground-soft transition-colors hover:bg-surface-soft" title={t('logs.autoRefreshHint')}>
              <Switch id="logs-auto-refresh" checked={autoRefresh} onChange={setAutoRefresh} label={t('logs.autoRefresh')} />
              {t('logs.autoRefresh')}
            </label>
            <Button variant="outline" loading={exporting} disabled={total === 0} onClick={() => void exportCsv()} title={t('logs.exportTitle')}>
              <Download size={14} /> {t('common.export')}
            </Button>
            <Button variant="outline" className="min-w-[92px]" loading={refreshing} onClick={() => void refresh()}>
              <RefreshCw size={14} />
              {t('common.refresh')}
            </Button>
          </>
        )}
      />
      {/* 筛选栏（gpt-load LogsFilterForm 模式）：渠道/令牌用下拉而不是让用户记数字 ID */}
      <Card>
        <div className="eg-grid eg-grid-auto items-end">
          <Field label={t('logs.filter.model')}><Input placeholder={t('logs.filter.modelPlaceholder')} value={model} onChange={(e) => resetPage(setModel)(e.target.value)} /></Field>
          <Field label={t('logs.filter.status')}>
            <Select
              value={status}
              onValueChange={resetPage(setStatus)}
              aria-label={t('logs.filter.status')}
              options={[
                { value: '', label: t('logs.filter.all') },
                { value: '200', label: t('logs.filter.200') },
                { value: '401', label: t('logs.filter.401') },
                { value: '429', label: t('logs.filter.429') },
                { value: '503', label: t('logs.filter.503') },
              ]}
            />
          </Field>
          <Field label={t('logs.filter.channel')}>
            <Select
              value={channelId}
              onValueChange={resetPage(setChannelId)}
              aria-label={t('logs.filter.channel')}
              options={[
                { value: '', label: t('logs.filter.allChannels') },
                ...(channels.data?.items ?? []).map((c) => ({ value: String(c.id), label: `${c.name}（#${c.id}）` })),
              ]}
            />
          </Field>
          <Field label={t('logs.filter.token')}>
            <Select
              value={tokenId}
              onValueChange={resetPage(setTokenId)}
              aria-label={t('logs.filter.token')}
              options={[
                { value: '', label: t('logs.filter.allTokens') },
                ...(tokens.data?.items ?? []).map((token) => ({ value: String(token.id), label: `${token.name}（#${token.id}）` })),
              ]}
            />
          </Field>
          <Field label={t('logs.filter.start')}><Input type="date" value={startDate} onChange={(e) => resetPage(setStartDate)(e.target.value)} /></Field>
          <Field label={t('logs.filter.end')}><Input type="date" value={endDate} onChange={(e) => resetPage(setEndDate)(e.target.value)} /></Field>
          <span className="pb-2 text-xs text-subtle"><Search size={14} className="inline" aria-hidden="true" /> {t('common.search')} · {t('logs.filter.tzHint')}</span>
        </div>
      </Card>

      <Card>
        <ContentSwap loading={logs.isPending} skeleton={<SkeletonRows rows={8} className="eg-card-fluid" />}>
          {logs.data?.items.length === 0 ? (
            <Empty
              icon={<ScrollText size={36} />}
              filtered={hasFilters}
              title={t(hasFilters ? 'empty.filtered.logs.title' : 'empty.logs.title')}
              hint={t(hasFilters ? 'empty.filtered.logs.hint' : 'empty.logs.hint')}
              action={hasFilters ? <Button size="sm" variant="outline" onClick={clearFilters}>{t('common.clearFilters')}</Button> : undefined}
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[720px]">
                <thead className="border-b border-line-soft bg-surface-soft">
                  <tr>
                    <Th>{t('table.time')}</Th><Th>{t('table.model')}</Th><Th className="hidden min-[1120px]:table-cell">{t('table.channel')}</Th><Th>{t('table.status')}</Th>
                    <Th className="hidden min-[1080px]:table-cell">{t('table.firstToken')}</Th><Th>{t('table.totalMs')}</Th><Th title={t('table.tokensTitle')}>{t('table.tokens')}</Th><Th>{t('table.cost')}</Th>
                    <Th className="hidden min-[1180px]:table-cell">{t('table.token')}</Th><Th className="hidden min-[1240px]:table-cell">{t('table.retry')}</Th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line-soft">
                  {logs.data?.items.map((l) => {
                    // 失败请求没有「命中渠道」：列表直接给最后尝试的渠道，不必点开详情
                    const last = l.status_code >= 400 ? lastAttempt(l.retry_trace) : null
                    return (
                      <tr
                        key={l.id}
                        tabIndex={0}
                        aria-haspopup="dialog"
                        className={`cursor-pointer outline-none hover:bg-surface-soft focus-visible:bg-brand-subtle ${selected?.id === l.id ? 'bg-brand-subtle' : ''}`}
                        onClick={() => setSelected(l)}
                        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); setSelected(l) } }}
                      >
                        <Td className="whitespace-nowrap text-xs text-muted">{fmtTime(l.created_at)}</Td>
                        <Td className="font-medium">
                          {l.model}
                          {l.original_model && l.original_model !== l.model && (
                            <Badge tone="warning" className="ml-2">{tf('logs.originalModel', { model: l.original_model })}</Badge>
                          )}
                          {l.source === 'playground' && <Badge tone="brand" className="ml-2">{t('log.source.playground')}</Badge>}
                        </Td>
                        <Td className="hidden min-[1120px]:table-cell">
                          {l.channel_name ? (
                            <span className="text-muted">{l.channel_name}</span>
                          ) : last?.channel_name ? (
                            <span className="inline-flex items-center gap-1 text-warning-text" title={t('logs.list.lastAttempt')}>
                              <RotateCcw size={12} aria-hidden="true" />{last.channel_name}
                            </span>
                          ) : '—'}
                        </Td>
                        <Td><span className={`font-semibold ${statusTone(l.status_code)}`}>{l.status_code}</span></Td>
                        <Td className="hidden min-[1080px]:table-cell">{fmtMs(l.first_token_ms || undefined)}</Td>
                        <Td>{fmtMs(l.total_ms || undefined)}</Td>
                        <Td className="text-xs text-muted">{l.prompt_tokens}/{l.completion_tokens}</Td>
                        <Td className="whitespace-nowrap text-xs">{fmtRequestCost(l.cost_micros, l.cost_status)}</Td>
                        <Td className="hidden text-xs text-muted min-[1180px]:table-cell">{l.token_name || '—'}</Td>
                        <Td className="hidden min-[1240px]:table-cell">{(l.retry_trace?.length ?? 0) > 1 ? <Badge tone="warning">{tf('logs.retryCount', { n: l.retry_trace!.length })}</Badge> : '—'}</Td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </ContentSwap>
        <Pagination
          page={page}
          totalPages={totalPages}
          total={total}
          pageSize={pageSize}
          onPageChange={setPage}
          onPageSizeChange={(size) => { setPageSize(size); setPage(1) }}
        />
      </Card>

      <LogDrawer log={selected} onClose={() => setSelected(null)} onFilter={applyDetailFilter} />
    </div>
  )
}

function LogDrawer({ log, onClose, onFilter }: { log: RequestLog | null; onClose: () => void; onFilter: (patch: DetailFilter) => void }) {
  return (
    <Drawer open={!!log} onClose={onClose} title={tf('logs.detail.title', { id: log?.id ?? '' })} wide>
      {log && <LogDetail key={log.id} log={log} onFilter={onFilter} />}
    </Drawer>
  )
}

// 日志详情：顶部摘要带先回答「成败 / 模型 / 渠道 / 耗时 / 成本」，再按请求 / 路由 / 用量 / 错误 / 轨迹分组。
function LogDetail({ log, onFilter }: { log: RequestLog; onFilter: (patch: DetailFilter) => void }) {
  const ok = log.status_code >= 200 && log.status_code < 400
  const tone = httpStatusTone(log.status_code)
  const last = lastAttempt(log.retry_trace)
  const channelID = log.channel_id || last?.channel_id
  const channelName = log.channel_name || last?.channel_name || ''
  const keyID = log.key_id || last?.key_id
  const chain = modelChain(log)
  const upstream = describeUpstreamError(log.error_message)
  const priced = log.cost_status === 'priced'
  const attempts = log.retry_trace ?? []
  const tint = tone === 'success' ? 'border-success-border bg-success-subtle' : tone === 'warning' ? 'border-warning-border bg-warning-subtle' : 'border-danger-border bg-danger-subtle'
  const channelLabel = log.channel_name || (last?.channel_name ? `${last.channel_name}（${t('logs.detail.lastAttempt')}）` : '')

  return (
    <div className="space-y-[var(--eg-control-gap)]">
      <section className={`rounded-lg border p-3 ${tint}`}>
        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge tone={tone}>{log.status_code} {ok ? t('logs.detail.success') : t('logs.detail.failed')}</StatusBadge>
          {log.stream && <Badge tone="neutral" compact>{t('log.field.stream')}</Badge>}
          <span className="text-xs text-muted">{fmtTime(log.created_at)}</span>
          <span className="ml-auto font-mono text-xs text-subtle">#{log.id}</span>
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-1.5">
          {chain.map((seg, i) => (
            <span key={`${seg.role}-${seg.model}`} className="inline-flex items-center gap-1">
              {i > 0 && <ArrowRight size={12} className="text-subtle" aria-hidden="true" />}
              <span title={seg.role === 'original' ? t('log.field.originalModel') : seg.role === 'upstream' ? t('table.upstreamModel') : t('table.model')} className={`break-all font-mono text-sm ${seg.role === 'public' ? 'font-semibold text-foreground' : 'text-muted'}`}>{seg.model}</span>
            </span>
          ))}
          <CopyButton compact value={log.upstream_model || log.model} label={t('common.copy')} side="bottom" />
        </div>
        <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
          <span>{t('table.channel')}：<span className="font-medium text-foreground-soft">{channelName || '—'}</span></span>
          <span>{t('log.field.keyId')} <span className="font-medium text-foreground-soft">{keyID ? `#${keyID}` : '—'}</span></span>
          <span>{t('table.token')}：<span className="font-medium text-foreground-soft">{log.token_name || '—'}</span></span>
        </div>
        <dl className="mt-3 grid grid-cols-3 gap-2">
          <SummaryStat label={t('table.firstToken')} value={fmtMs(log.first_token_ms || undefined)} />
          <SummaryStat label={t('table.totalMs')} value={fmtMs(log.total_ms || undefined)} />
          <SummaryStat label={t('table.cost')} value={fmtRequestCost(log.cost_micros, log.cost_status)} muted={!priced} />
        </dl>
        <div className="mt-3 flex flex-wrap gap-2 border-t border-line-strong/50 pt-3">
          {channelID ? (
            <Button size="sm" variant="outline" className="bg-surface" onClick={() => onFilter({ channel_id: String(channelID) })}>
              <Filter size={12} /> {t('logs.detail.filterChannel')}
            </Button>
          ) : null}
          {log.token_id ? (
            <Button size="sm" variant="outline" className="bg-surface" onClick={() => onFilter({ token_id: String(log.token_id) })}>
              <Filter size={12} /> {t('logs.detail.filterToken')}
            </Button>
          ) : null}
          <Button size="sm" variant="outline" className="bg-surface" onClick={() => onFilter({ model: log.model })}>
            <Filter size={12} /> {t('logs.detail.filterModel')}
          </Button>
        </div>
      </section>

      <Disclosure title={t('logs.section.request')}>
        <FieldGrid rows={[
          { label: t('table.time'), value: fmtTime(log.created_at) },
          { label: t('log.field.source'), value: log.source === 'playground' ? t('log.source.playground') : t('log.source.api') },
          { label: t('log.field.stream'), value: log.stream ? t('common.yes') : t('common.no') },
          { label: t('table.token'), value: log.token_name || '—' },
          { label: t('log.field.originalModel'), value: log.original_model || '—', mono: true, copy: log.original_model || undefined },
          { label: t('table.model'), value: log.model, mono: true, copy: log.model },
          { label: t('table.upstreamModel'), value: log.upstream_model || '—', mono: true, copy: log.upstream_model || undefined },
          { label: t('log.field.clientIP'), value: log.ip || '—', mono: true, copy: log.ip || undefined },
        ]} />
      </Disclosure>

      <Disclosure title={t('logs.section.routing')}>
        <FieldGrid rows={[
          { label: t('table.channel'), value: channelLabel || '—', copy: channelName || undefined },
          { label: t('logs.detail.retryCount'), value: attempts.length > 1 ? tf('logs.retryCount', { n: attempts.length - 1 }) : '0' },
          { label: t('table.firstToken'), value: fmtMs(log.first_token_ms || undefined) },
          { label: t('table.totalMs'), value: fmtMs(log.total_ms || undefined) },
        ]} />
      </Disclosure>

      <Disclosure title={t('logs.section.usage')} badge={priced ? undefined : <Badge tone="warning" compact>{fmtRequestCost(log.cost_micros, log.cost_status)}</Badge>}>
        <div className="grid grid-cols-3 gap-2">
          <TokenStat label={t('log.field.promptTokens')} value={fmtInt(log.prompt_tokens)} />
          <TokenStat label={t('log.field.completionTokens')} value={fmtInt(log.completion_tokens)} />
          <TokenStat label={t('log.field.totalTokens')} value={fmtInt(log.total_tokens || 0)} />
          {!!log.cached_tokens && <TokenStat label={t('log.field.cachedTokens')} value={fmtInt(log.cached_tokens)} />}
          {!!log.cache_write_tokens && <TokenStat label={t('log.field.cacheWriteTokens')} value={fmtInt(log.cache_write_tokens)} />}
          {!!log.reasoning_tokens && <TokenStat label={t('log.field.reasoningTokens')} value={fmtInt(log.reasoning_tokens)} />}
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line-soft pt-3 text-xs">
          <span className="text-subtle">{t('table.cost')}</span>
          <span className={`text-sm font-semibold tabular-nums ${priced ? 'text-foreground' : 'text-warning-text'}`}>{fmtRequestCost(log.cost_micros, log.cost_status)}</span>
          {log.price_source && <PriceSourceBadge source={log.price_source} />}
        </div>
        {!priced && (
          <Alert
            tone={log.cost_status === 'unpriced' ? 'warning' : 'neutral'}
            className="mt-2"
            action={log.cost_status === 'unpriced' ? <Link to="/usage" className="text-xs font-medium underline underline-offset-2">{t('cost.setPrices')}</Link> : undefined}
          >
            {log.cost_status === 'unpriced' ? tf('cost.unpricedReason', { model: log.upstream_model || log.model })
              : log.cost_status === 'no_usage' ? t('cost.noUsageReason')
                : t('cost.unknownReason')}
          </Alert>
        )}
      </Disclosure>

      {(log.error_message || log.error_type) && (
        <Disclosure title={t('logs.section.error')} badge={<Badge tone="danger" compact>{kindText(log.error_type)}</Badge>}>
          <Alert tone="danger" data-upstream="true" action={<CopyButton compact value={log.error_message} label={t('logs.error.copy')} />}>
            {upstream.code && <div className="mb-1"><Badge tone="neutral" compact>{tf('retry.upstreamCode', { code: upstream.code })}</Badge></div>}
            <ErrorBlock text={upstream.text} />
          </Alert>
        </Disclosure>
      )}

      <Disclosure title={t('logs.trace.title')} badge={attempts.length > 1 ? <Badge tone="warning" compact>{tf('logs.retryCount', { n: attempts.length })}</Badge> : undefined}>
        <RetryTimeline attempts={log.retry_trace} />
      </Disclosure>

      <Disclosure title={t('logs.section.secondary')} defaultOpen={false}>
        <FieldGrid rows={[
          { label: t('log.field.requestId'), value: `#${log.id}` },
          { label: t('log.field.tokenId'), value: log.token_id ? `#${log.token_id}` : '—' },
          { label: t('log.field.keyId'), value: keyID ? `#${keyID}` : '—' },
          { label: t('log.field.userAgent'), value: log.user_agent || '—', mono: true, copy: log.user_agent || undefined, wide: true },
          { label: t('log.field.configOverride'), value: log.config_override || '—', mono: true, wide: true },
        ]} />
      </Disclosure>
    </div>
  )
}

/** 模型映射链：原始模型（仅降级时存在）→ 对外模型 → 上游模型；相邻重名合并。 */
function modelChain(log: RequestLog): { model: string; role: 'original' | 'public' | 'upstream' }[] {
  const chain: { model: string; role: 'original' | 'public' | 'upstream' }[] = [{ model: log.model, role: 'public' }]
  if (log.original_model && log.original_model !== log.model) chain.unshift({ model: log.original_model, role: 'original' })
  if (log.upstream_model && log.upstream_model !== log.model) chain.push({ model: log.upstream_model, role: 'upstream' })
  return chain
}

function SummaryStat({ label, value, muted = false }: { label: string; value: string; muted?: boolean }) {
  return (
    <div className="rounded-md bg-surface px-2.5 py-1.5">
      <dt className="text-[11px] text-subtle">{label}</dt>
      <dd className={`mt-0.5 text-sm font-semibold tabular-nums ${muted ? 'text-warning-text' : 'text-foreground'}`}>{value}</dd>
    </div>
  )
}

function TokenStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md bg-surface-soft px-2.5 py-1.5">
      <div className="text-[11px] text-subtle">{label}</div>
      <div className="mt-0.5 text-sm font-semibold tabular-nums text-foreground">{value}</div>
    </div>
  )
}

type FieldRow = { label: string; value: ReactNode; mono?: boolean; copy?: string; wide?: boolean }

/** 键值网格：单列标签 / 单列值，窄屏单列、宽屏两列；带 copy 的行在值旁给出复制图标。 */
function FieldGrid({ rows }: { rows: FieldRow[] }) {
  return (
    <dl className="grid grid-cols-1 gap-x-4 text-xs sm:grid-cols-2">
      {rows.map((row) => (
        <div key={row.label} className={`group flex min-w-0 items-center justify-between gap-3 border-b border-line-soft py-1.5 ${row.wide ? 'sm:col-span-2' : ''}`}>
          <dt className="shrink-0 text-subtle">{row.label}</dt>
          <dd className="flex min-w-0 items-center gap-1">
            <span className={`min-w-0 break-all text-right text-foreground-soft ${row.mono ? 'font-mono text-[11px]' : 'tabular-nums'}`}>{row.value}</span>
            {row.copy && <CopyButton compact value={row.copy} className="opacity-60 group-hover:opacity-100 group-focus-within:opacity-100" />}
          </dd>
        </div>
      ))}
    </dl>
  )
}

/** 错误正文：超过 3 行先折叠，可展开。 */
function ErrorBlock({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(false)
  const long = text.length > 160
  return (
    <>
      <div className={`whitespace-pre-wrap break-all font-mono text-[11px] leading-5 ${long && !expanded ? 'line-clamp-3' : ''}`}>{text}</div>
      {long && (
        <Button size="sm" variant="ghost" className="mt-1 -ml-2 h-7 px-2 text-[11px]" aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>
          {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
          {expanded ? t('logs.error.collapse') : t('logs.error.expand')}
        </Button>
      )}
    </>
  )
}

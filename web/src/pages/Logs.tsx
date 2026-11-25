import { useState } from 'react'
import { Download, RefreshCw, Search, ScrollText } from 'lucide-react'
import { buildLogQuery, downloadAdminFile, useChannels, useLogs, useTokens } from '../api/hooks'
import { Badge, Button, Card, Drawer, Empty, Field, Input, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import RetryTimeline from '../components/RetryTimeline'
import type { RequestLog } from '../api/types'
import { describeUpstreamError, fmtInt, fmtMs, fmtRequestCost, fmtTime, kindText, statusTone } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'

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
  const [exportError, setExportError] = useState('')
  const [selected, setSelected] = useState<RequestLog | null>(null)
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
  const logs = useLogs({ ...filters, page, page_size: pageSize })

  const total = logs.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const resetPage = <T,>(setter: (v: T) => void) => (v: T) => { setter(v); setPage(1) }

  const exportCsv = async () => {
    setExporting(true)
    setExportError('')
    try {
      await downloadAdminFile(`/admin/api/logs/export?${buildLogQuery(filters)}`, 'evergate-logs.csv')
    } catch (err) {
      setExportError((err as Error).message || t('logs.exportFailed'))
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
            <Button size="sm" variant="outline" disabled={exporting || total === 0} onClick={() => void exportCsv()} title={t('logs.exportTitle')}>
              <Download size={14} /> {exporting ? t('common.exporting') : t('common.export')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="w-[78px]"
              disabled={logs.isFetching}
              aria-busy={logs.isFetching}
              onClick={() => void logs.refetch()}
            >
              <RefreshCw size={14} className={logs.isFetching ? 'animate-spin' : ''} />
              {t('common.refresh')}
            </Button>
          </>
        )}
      />
      {exportError && <p role="alert" className="text-xs text-danger-text">{exportError}</p>}

      {/* 筛选栏（gpt-load LogsFilterForm 模式）：渠道/令牌用下拉而不是让用户记数字 ID */}
      <Card>
        <div className="eg-grid eg-grid-auto items-end">
          <Field label={t('logs.filter.model')}><Input placeholder={t('logs.filter.modelPlaceholder')} value={model} onChange={(e) => resetPage(setModel)(e.target.value)} /></Field>
          <Field label={t('logs.filter.status')}>
            <Select value={status} onChange={(e) => resetPage(setStatus)(e.target.value)} aria-label={t('logs.filter.status')}>
              <option value="">{t('logs.filter.all')}</option>
              <option value="200">{t('logs.filter.200')}</option>
              <option value="401">{t('logs.filter.401')}</option>
              <option value="429">{t('logs.filter.429')}</option>
              <option value="503">{t('logs.filter.503')}</option>
            </Select>
          </Field>
          <Field label={t('logs.filter.channel')}>
            <Select value={channelId} onChange={(e) => resetPage(setChannelId)(e.target.value)} aria-label={t('logs.filter.channel')}>
              <option value="">{t('logs.filter.allChannels')}</option>
              {(channels.data?.items ?? []).map((c) => <option key={c.id} value={c.id}>{c.name}（#{c.id}）</option>)}
            </Select>
          </Field>
          <Field label={t('logs.filter.token')}>
            <Select value={tokenId} onChange={(e) => resetPage(setTokenId)(e.target.value)} aria-label={t('logs.filter.token')}>
              <option value="">{t('logs.filter.allTokens')}</option>
              {(tokens.data?.items ?? []).map((t) => <option key={t.id} value={t.id}>{t.name}（#{t.id}）</option>)}
            </Select>
          </Field>
          <Field label={t('logs.filter.start')}><Input type="date" value={startDate} onChange={(e) => resetPage(setStartDate)(e.target.value)} /></Field>
          <Field label={t('logs.filter.end')}><Input type="date" value={endDate} onChange={(e) => resetPage(setEndDate)(e.target.value)} /></Field>
          <span className="pb-2 text-xs text-subtle"><Search size={14} className="inline" aria-hidden="true" /> {t('common.search')} · {t('logs.filter.tzHint')}</span>
        </div>
      </Card>

      <Card>
        {logs.isPending ? (
          <SkeletonRows rows={8} className="eg-card-fluid" />
        ) : logs.data?.items.length === 0 ? (
          <Empty icon={<ScrollText size={36} />} title={t('empty.logs.title')} hint={t('empty.logs.hint')} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[820px]">
              <thead className="border-b border-line-soft bg-surface-soft">
                <tr>
                  <Th>{t('table.time')}</Th><Th>{t('table.model')}</Th><Th className="hidden min-[1120px]:table-cell">{t('table.channel')}</Th><Th>{t('table.status')}</Th>
          <Th>{t('table.firstToken')}</Th><Th>{t('table.totalMs')}</Th><Th title={t('table.tokensTitle')}>{t('table.tokens')}</Th><Th>{t('table.cost')}</Th>
                  <Th className="hidden min-[1180px]:table-cell">{t('table.token')}</Th><Th className="hidden min-[1240px]:table-cell">{t('table.retry')}</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {logs.data?.items.map((l) => (
                  <tr key={l.id} className="cursor-pointer hover:bg-surface-soft" onClick={() => setSelected(l)}>
                    <Td className="whitespace-nowrap text-xs text-muted">{fmtTime(l.created_at)}</Td>
                    <Td className="font-medium">
                      {l.model}
                      {l.original_model && l.original_model !== l.model && (
                        <Badge tone="warning" className="ml-2">{tf('logs.originalModel', { model: l.original_model })}</Badge>
                      )}
                      {l.source === 'playground' && <Badge tone="brand" className="ml-2">{t('log.source.playground')}</Badge>}
                    </Td>
                    <Td className="hidden text-muted min-[1120px]:table-cell">{l.channel_name || '—'}</Td>
        <Td><span className={`font-semibold ${statusTone(l.status_code)}`}>{l.status_code}</span></Td>
                    <Td>{fmtMs(l.first_token_ms || undefined)}</Td>
                    <Td>{fmtMs(l.total_ms || undefined)}</Td>
                    <Td className="text-xs text-muted">{l.prompt_tokens}/{l.completion_tokens}</Td>
                    <Td className="whitespace-nowrap text-xs">{fmtRequestCost(l.cost_micros, l.cost_status)}</Td>
                    <Td className="hidden text-xs text-muted min-[1180px]:table-cell">{l.token_name || '—'}</Td>
                    <Td className="hidden min-[1240px]:table-cell">{(l.retry_trace?.length ?? 0) > 1 ? <Badge tone="warning">{tf('logs.retryCount', { n: l.retry_trace!.length })}</Badge> : '—'}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <Pagination
          page={page}
          totalPages={totalPages}
          total={total}
          pageSize={pageSize}
          onPageChange={setPage}
          onPageSizeChange={(size) => { setPageSize(size); setPage(1) }}
        />
      </Card>

      <LogDrawer log={selected} onClose={() => setSelected(null)} />
    </div>
  )
}

// 日志详情抽屉 + 重试轨迹时间线（任务书 4.6 核心，gpt-load LogDetailDrawer 模式）
function LogDrawer({ log, onClose }: { log: RequestLog | null; onClose: () => void }) {
  return (
    <Drawer open={!!log} onClose={onClose} title={tf('logs.detail.title', { id: log?.id ?? '' })} wide>
      {log && (
        <div className="space-y-[var(--eg-section-gap)]">
          <div className="grid grid-cols-1 gap-x-[var(--eg-grid-gap)] gap-y-2 text-sm sm:grid-cols-2">
            {[
              [t('table.time'), fmtTime(log.created_at)],
              [t('table.model'), log.model + (log.upstream_model && log.upstream_model !== log.model ? ` → ${log.upstream_model}` : '')],
              [t('log.field.originalModel'), log.original_model || '—'],
              [t('table.channel'), log.channel_name || '—'],
              ['key ID', log.key_id || '—'],
              [t('log.field.statusCode'), <span className={`font-semibold ${statusTone(log.status_code)}`}>{log.status_code}</span>],
              [t('log.field.stream'), log.stream ? t('common.yes') : t('common.no')],
              [t('table.firstToken'), fmtMs(log.first_token_ms || undefined)],
              [t('table.totalMs'), fmtMs(log.total_ms || undefined)],
              [t('log.field.promptTokens'), fmtInt(log.prompt_tokens)],
              [t('log.field.completionTokens'), fmtInt(log.completion_tokens)],
              [t('log.field.totalTokens'), fmtInt(log.total_tokens || 0)],
              [t('log.field.cachedTokens'), fmtInt(log.cached_tokens || 0)],
              [t('log.field.reasoningTokens'), fmtInt(log.reasoning_tokens || 0)],
              [t('table.cost'), fmtRequestCost(log.cost_micros, log.cost_status)],
              [t('table.token'), log.token_name || '—'],
              [t('log.field.source'), log.source === 'playground' ? t('log.source.playground') : t('log.source.api')],
              [t('log.field.clientIP'), log.ip || '—'],
            ].map(([k, v]) => (
              <div key={String(k)} className="flex justify-between border-b border-line-soft py-1">
                <span className="text-subtle">{k}</span>
                <span className="text-foreground-soft">{v}</span>
              </div>
            ))}
          </div>

          {log.error_message && (
            <div className="rounded-lg bg-danger-subtle p-3">
              <div className="flex flex-wrap items-center gap-2 text-xs font-semibold text-danger-text">
                <span>{kindText(log.error_type)}</span>
                {describeUpstreamError(log.error_message).code && <Badge tone="neutral">{tf('retry.upstreamCode', { code: describeUpstreamError(log.error_message).code ?? '' })}</Badge>}
              </div>
              <div className="mt-1 break-words text-xs text-danger-text">{describeUpstreamError(log.error_message).text}</div>
            </div>
          )}

          <div>
            <h4 className="mb-3 text-sm font-semibold">{t('logs.trace.title')}</h4>
            <RetryTimeline attempts={log.retry_trace} />
          </div>
        </div>
      )}
    </Drawer>
  )
}

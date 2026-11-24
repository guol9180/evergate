import { useState } from 'react'
import { Download, RefreshCw, Search, ScrollText } from 'lucide-react'
import { buildLogQuery, downloadAdminFile, useChannels, useLogs, useTokens } from '../api/hooks'
import { Badge, Button, Card, Drawer, Empty, Field, Input, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import RetryTimeline from '../components/RetryTimeline'
import type { RequestLog } from '../api/types'
import { fmtInt, fmtMs, fmtRequestCost, fmtTime, kindText, statusTone } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'

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
      setExportError((err as Error).message || '导出失败')
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
            <Button size="sm" variant="outline" disabled={exporting || total === 0} onClick={() => void exportCsv()} title="导出当前筛选结果（最多 10,000 行）">
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
          <Field label="模型"><Input placeholder="精确匹配，如 gpt-4o" value={model} onChange={(e) => resetPage(setModel)(e.target.value)} /></Field>
          <Field label="状态码">
            <Select value={status} onChange={(e) => resetPage(setStatus)(e.target.value)} aria-label="状态码">
              <option value="">全部</option>
              <option value="200">200 成功</option>
              <option value="401">401 鉴权失败</option>
              <option value="429">429 限流</option>
              <option value="503">503 渠道耗尽</option>
            </Select>
          </Field>
          <Field label="渠道">
            <Select value={channelId} onChange={(e) => resetPage(setChannelId)(e.target.value)} aria-label="渠道">
              <option value="">全部渠道</option>
              {(channels.data?.items ?? []).map((c) => <option key={c.id} value={c.id}>{c.name}（#{c.id}）</option>)}
            </Select>
          </Field>
          <Field label="令牌">
            <Select value={tokenId} onChange={(e) => resetPage(setTokenId)(e.target.value)} aria-label="令牌">
              <option value="">全部令牌</option>
              {(tokens.data?.items ?? []).map((t) => <option key={t.id} value={t.id}>{t.name}（#{t.id}）</option>)}
            </Select>
          </Field>
          <Field label="开始日期"><Input type="date" value={startDate} onChange={(e) => resetPage(setStartDate)(e.target.value)} /></Field>
          <Field label="结束日期"><Input type="date" value={endDate} onChange={(e) => resetPage(setEndDate)(e.target.value)} /></Field>
          <span className="pb-2 text-xs text-subtle"><Search size={14} className="inline" aria-hidden="true" /> {t('common.search')} · 日期按本地时区</span>
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
                  <Th>时间</Th><Th>模型</Th><Th className="hidden min-[1120px]:table-cell">渠道</Th><Th>状态</Th>
                  <Th>首 token</Th><Th>总耗时</Th><Th>tokens</Th><Th>成本</Th>
                  <Th className="hidden min-[1180px]:table-cell">令牌</Th><Th className="hidden min-[1240px]:table-cell">重试</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {logs.data?.items.map((l) => (
                  <tr key={l.id} className="cursor-pointer hover:bg-surface-soft" onClick={() => setSelected(l)}>
                    <Td className="whitespace-nowrap text-xs text-muted">{fmtTime(l.created_at)}</Td>
                    <Td className="font-medium">
                      {l.model}
                      {l.original_model && l.original_model !== l.model && (
                        <Badge tone="warning" className="ml-2">原 {l.original_model}</Badge>
                      )}
                      {l.source === 'playground' && <Badge tone="brand" className="ml-2">Playground</Badge>}
                    </Td>
                    <Td className="hidden text-muted min-[1120px]:table-cell">{l.channel_name || '—'}</Td>
                    <Td className={`font-semibold ${statusTone(l.status_code)}`}>{l.status_code}</Td>
                    <Td>{fmtMs(l.first_token_ms || undefined)}</Td>
                    <Td>{fmtMs(l.total_ms || undefined)}</Td>
                    <Td className="text-xs text-muted">{l.prompt_tokens}/{l.completion_tokens}</Td>
                    <Td className="whitespace-nowrap text-xs">{fmtRequestCost(l.cost_micros, l.cost_status)}</Td>
                    <Td className="hidden text-xs text-muted min-[1180px]:table-cell">{l.token_name || '—'}</Td>
                    <Td className="hidden min-[1240px]:table-cell">{(l.retry_trace?.length ?? 0) > 1 ? <Badge tone="warning">{l.retry_trace!.length} 次</Badge> : '—'}</Td>
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
    <Drawer open={!!log} onClose={onClose} title={`请求日志 #${log?.id ?? ''}`} wide>
      {log && (
        <div className="space-y-[var(--eg-section-gap)]">
          <div className="grid grid-cols-1 gap-x-[var(--eg-grid-gap)] gap-y-2 text-sm sm:grid-cols-2">
            {[
              ['时间', fmtTime(log.created_at)],
              ['模型', log.model + (log.upstream_model && log.upstream_model !== log.model ? ` → ${log.upstream_model}` : '')],
              ['原始模型', log.original_model || '—'],
              ['渠道', log.channel_name || '—'],
              ['key ID', log.key_id || '—'],
              ['状态码', <span className={`font-semibold ${statusTone(log.status_code)}`}>{log.status_code}</span>],
              ['流式', log.stream ? '是' : '否'],
              ['首 token', fmtMs(log.first_token_ms || undefined)],
              ['总耗时', fmtMs(log.total_ms || undefined)],
              ['prompt tokens', fmtInt(log.prompt_tokens)],
              ['completion tokens', fmtInt(log.completion_tokens)],
              ['total tokens', fmtInt(log.total_tokens || 0)],
              ['cached tokens', fmtInt(log.cached_tokens || 0)],
              ['reasoning tokens', fmtInt(log.reasoning_tokens || 0)],
              ['成本', fmtRequestCost(log.cost_micros, log.cost_status)],
              ['令牌', log.token_name || '—'],
              ['来源', log.source === 'playground' ? 'Playground' : 'API'],
              ['客户端 IP', log.ip || '—'],
            ].map(([k, v]) => (
              <div key={String(k)} className="flex justify-between border-b border-line-soft py-1">
                <span className="text-subtle">{k}</span>
                <span className="text-foreground-soft">{v}</span>
              </div>
            ))}
          </div>

          {log.error_message && (
            <div className="rounded-lg bg-danger-subtle p-3">
              <div className="text-xs font-semibold text-danger-text">{kindText(log.error_type)}</div>
              <div className="mt-1 break-all text-xs text-danger-text">{log.error_message}</div>
            </div>
          )}

          <div>
            <h4 className="mb-3 text-sm font-semibold">重试轨迹</h4>
            <RetryTimeline attempts={log.retry_trace} />
          </div>
        </div>
      )}
    </Drawer>
  )
}

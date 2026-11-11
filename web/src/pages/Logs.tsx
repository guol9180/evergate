import { useState } from 'react'
import { ChevronLeft, ChevronRight, Search, ScrollText } from 'lucide-react'
import { useLogs } from '../api/hooks'
import { Badge, Button, Card, Drawer, Empty, Input, Spinner, Td, Th } from '../components/ui'
import type { RequestLog } from '../api/types'
import { fmtInt, fmtMs, fmtTime, kindText, statusTone } from '../lib/format'

export default function Logs() {
  const [model, setModel] = useState('')
  const [status, setStatus] = useState('')
  const [channelId, setChannelId] = useState('')
  const [tokenId, setTokenId] = useState('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<RequestLog | null>(null)

  const logs = useLogs({
    model: model || undefined,
    status: status || undefined,
    channel_id: channelId || undefined,
    token_id: tokenId || undefined,
    page,
    page_size: 20,
  })

  const total = logs.data?.total ?? 0
  const pageSize = logs.data?.page_size ?? 20
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <div className="space-y-4">
      <h1 className="text-lg font-bold">请求日志</h1>

      {/* 筛选栏（gpt-load LogsFilterForm 模式） */}
      <Card className="flex flex-wrap items-end gap-3 p-4">
        <div className="w-44"><Input placeholder="模型" value={model} onChange={(e) => { setModel(e.target.value); setPage(1) }} /></div>
        <div className="w-28"><Input placeholder="状态码" value={status} onChange={(e) => { setStatus(e.target.value); setPage(1) }} /></div>
        <div className="w-28"><Input placeholder="渠道 ID" value={channelId} onChange={(e) => { setChannelId(e.target.value); setPage(1) }} /></div>
        <div className="w-28"><Input placeholder="令牌 ID" value={tokenId} onChange={(e) => { setTokenId(e.target.value); setPage(1) }} /></div>
        <span className="pb-2 text-xs text-slate-400"><Search size={14} className="inline" /> 输入即筛选</span>
      </Card>

      <Card>
        {logs.isPending ? (
          <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>
        ) : logs.data?.items.length === 0 ? (
          <Empty icon={<ScrollText size={36} />} title="暂无日志" hint="有请求经过网关后，这里会显示每次调用的完整轨迹" />
        ) : (
          <table className="w-full">
            <thead className="border-b border-slate-100 bg-slate-50/50">
              <tr><Th>时间</Th><Th>模型</Th><Th>渠道</Th><Th>状态</Th><Th>首 token</Th><Th>总耗时</Th><Th>tokens</Th><Th>令牌</Th><Th>重试</Th></tr>
            </thead>
            <tbody className="divide-y divide-slate-50">
              {logs.data?.items.map((l) => (
                <tr key={l.id} className="cursor-pointer hover:bg-slate-50/50" onClick={() => setSelected(l)}>
                  <Td className="whitespace-nowrap text-xs text-slate-500">{fmtTime(l.created_at)}</Td>
                  <Td className="font-medium">{l.model}</Td>
                  <Td className="text-slate-500">{l.channel_name || '—'}</Td>
                  <Td className={`font-semibold ${statusTone(l.status_code)}`}>{l.status_code}</Td>
                  <Td>{fmtMs(l.first_token_ms || undefined)}</Td>
                  <Td>{fmtMs(l.total_ms || undefined)}</Td>
                  <Td className="text-xs text-slate-500">{l.prompt_tokens}/{l.completion_tokens}</Td>
                  <Td className="text-xs text-slate-500">{l.token_name || '—'}</Td>
                  <Td>{(l.retry_trace?.length ?? 0) > 1 ? <Badge tone="bg-amber-100 text-amber-700">{l.retry_trace!.length} 次</Badge> : '—'}</Td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {/* 分页 */}
        <div className="flex items-center justify-between border-t border-slate-100 px-4 py-3 text-xs text-slate-500">
          <span>共 {fmtInt(total)} 条 · 第 {page}/{totalPages} 页</span>
          <div className="flex gap-1">
            <Button size="sm" variant="outline" disabled={page <= 1} onClick={() => setPage(page - 1)}><ChevronLeft size={13} /></Button>
            <Button size="sm" variant="outline" disabled={page >= totalPages} onClick={() => setPage(page + 1)}><ChevronRight size={13} /></Button>
          </div>
        </div>
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
        <div className="space-y-5">
          <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
            {[
              ['时间', fmtTime(log.created_at)],
              ['模型', log.model + (log.upstream_model && log.upstream_model !== log.model ? ` → ${log.upstream_model}` : '')],
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
              ['令牌', log.token_name || '—'],
              ['客户端 IP', log.ip || '—'],
            ].map(([k, v]) => (
              <div key={String(k)} className="flex justify-between border-b border-slate-50 py-1">
                <span className="text-slate-400">{k}</span>
                <span className="text-slate-700">{v}</span>
              </div>
            ))}
          </div>

          {log.error_message && (
            <div className="rounded-lg bg-red-50 p-3">
              <div className="text-xs font-semibold text-red-700">{kindText(log.error_type)}</div>
              <div className="mt-1 break-all text-xs text-red-600">{log.error_message}</div>
            </div>
          )}

          <div>
            <h4 className="mb-3 text-sm font-semibold">重试轨迹</h4>
            {(log.retry_trace?.length ?? 0) === 0 ? (
              <p className="text-xs text-slate-400">无尝试记录</p>
            ) : (
              <div className="space-y-0">
                {log.retry_trace!.map((a, i) => {
                  const ok = !a.error_type && !a.error_message
                  return (
                    <div key={i} className="flex gap-3">
                      {/* 时间线竖线 */}
                      <div className="flex flex-col items-center">
                        <span className={`mt-1 h-2.5 w-2.5 rounded-full ${ok ? 'bg-emerald-500' : 'bg-red-500'}`} />
                        {i < log.retry_trace!.length - 1 && <span className="w-px flex-1 bg-slate-200" />}
                      </div>
                      <div className="flex-1 pb-5">
                        <div className="flex items-center gap-2 text-sm">
                          <span className="font-medium">尝试 {a.attempt}</span>
                          <span className="text-slate-500">→ {a.channel_name} · key #{a.key_id}</span>
                          {!!a.status_code && <span className="text-xs text-slate-400">HTTP {a.status_code}</span>}
                          <span className="ml-auto text-xs text-slate-400">{a.elapsed_ms}ms</span>
                        </div>
                        {ok ? (
                          <div className="mt-0.5 text-xs text-emerald-600">成功</div>
                        ) : (
                          <div className="mt-0.5 text-xs">
                            <Badge tone="bg-red-100 text-red-700">{kindText(a.error_type)}</Badge>
                            <span className="ml-2 break-all text-slate-500">{a.error_message?.slice(0, 120)}</span>
                          </div>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        </div>
      )}
    </Drawer>
  )
}

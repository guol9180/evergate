import { useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import type { TestAllItem } from '../api/types'
import { useChannelsTestAll } from '../api/hooks'
import { Button, Card, StatusBadge } from './ui'
import { fmtMs } from '../lib/format'

type SortKey = 'latency' | 'name' | 'result'

/** 渠道批量探测结果：一键摸底、按延迟排序；只调用模型列表 GET，不触发推理。 */
export default function TestAllPanel() {
  const testAll = useChannelsTestAll()
  const [sortKey, setSortKey] = useState<SortKey>('latency')
  // 面板打开即开始探测：此前首屏是另一个同名「测试全部」按钮，用户点两次才知道要再点一次
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return
    started.current = true
    if (!testAll.data && !testAll.isPending) testAll.mutate()
  }, [testAll])

  const items = useMemo(() => {
    const list = [...(testAll.data?.items ?? [])]
    switch (sortKey) {
      case 'name':
        return list.sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))
      case 'result':
        return list.sort((a, b) => Number(b.ok) - Number(a.ok) || a.latency_ms - b.latency_ms)
      default:
        return list.sort((a, b) => {
          if (a.ok !== b.ok) return Number(b.ok) - Number(a.ok)
          return (a.latency_ms || Number.MAX_SAFE_INTEGER) - (b.latency_ms || Number.MAX_SAFE_INTEGER)
        })
    }
  }, [testAll.data, sortKey])

  if (testAll.isPending && !testAll.data) {
    return (
      <div role="status">
        <Card className="eg-card-fluid text-sm text-muted">
          <RefreshCw size={14} className="mr-2 inline animate-spin" aria-hidden="true" />
          正在探测全部启用渠道（只请求模型列表，最长 30s）…
        </Card>
      </div>
    )
  }
  if (testAll.isError && !testAll.data) {
    return <div role="alert"><Card className="eg-card-fluid text-sm text-danger-text">测试全部失败：{(testAll.error as Error).message}</Card></div>
  }
  if (!testAll.data) {
    return (
      <Button size="sm" variant="outline" onClick={() => testAll.mutate()} disabled={testAll.isPending}>
        <RefreshCw size={14} className={testAll.isPending ? 'animate-spin' : ''} /> 测试全部
      </Button>
    )
  }

  const okCount = items.filter((i) => i.ok).length
  const header = (
    <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line-soft">
      <div className="text-sm font-semibold text-foreground">
        批量探测结果 <span className="ml-2 text-xs font-normal text-muted">{okCount}/{items.length} 成功</span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <label className="flex items-center gap-1.5 text-xs text-muted">
          排序
          <select
            value={sortKey}
            onChange={(e) => setSortKey(e.target.value as SortKey)}
            className="h-7 rounded-md border border-line-strong bg-surface px-2 text-xs text-foreground-soft outline-none focus:border-brand"
          >
            <option value="latency">延迟升序</option>
            <option value="name">名称</option>
            <option value="result">结果</option>
          </select>
        </label>
        <Button size="sm" variant="outline" onClick={() => testAll.mutate()} disabled={testAll.isPending}>
          <RefreshCw size={14} className={testAll.isPending ? 'animate-spin' : ''} /> 重新测试
        </Button>
      </div>
    </div>
  )

  if (items.length === 0) {
    return <Card className="eg-card-fluid">{header}<p className="py-6 text-center text-sm text-muted">没有启用的渠道可测试。</p></Card>
  }

  return (
    <Card flush>
      <div className="eg-card-fluid">{header}</div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[560px]">
          <thead className="border-b border-line-soft bg-surface-soft">
            <tr>
              <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">渠道</th>
              <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">结果</th>
              <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">延迟</th>
              <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">说明</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line-soft">
            {items.map((item) => (
              <tr key={item.channel_id}>
                <td className="eg-cell-x eg-cell-y text-sm font-medium text-foreground">{item.name}</td>
                <td className="eg-cell-x eg-cell-y"><StatusBadge tone={item.ok ? 'success' : item.result === 'inconclusive' ? 'neutral' : 'danger'}>{item.ok ? '可用' : item.result === 'inconclusive' ? '无法判定' : '失败'}</StatusBadge></td>
                <td className="eg-cell-x eg-cell-y text-sm tabular-nums text-foreground-soft">{item.latency_ms ? fmtMs(item.latency_ms) : '—'}</td>
                <td className="eg-cell-x eg-cell-y max-w-md truncate text-xs text-muted" title={item.error}>{item.error || '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

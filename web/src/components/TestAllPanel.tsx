import { useEffect, useMemo, useRef, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { useChannelsTestAll } from '../api/hooks'
import { Alert, Button, Card, Select, StatusBadge, Td, Th } from './ui'
import { fmtMs } from '../lib/format'
import { t, tf } from '../lib/i18n'

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
          {t('testAll.probing')}
        </Card>
      </div>
    )
  }
  if (testAll.isError && !testAll.data) {
    return <Alert tone="danger">{tf('testAll.failed', { message: (testAll.error as Error).message })}</Alert>
  }
  if (!testAll.data) {
    return (
      <Button size="sm" variant="outline" onClick={() => testAll.mutate()} loading={testAll.isPending}>
        <RefreshCw size={14} /> {t('common.testAll')}
      </Button>
    )
  }

  const okCount = items.filter((i) => i.ok).length
  const header = (
    <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line-soft">
      <div className="text-sm font-semibold text-foreground">
        {t('testAll.title')} <span className="ml-2 text-xs font-normal text-muted">{tf('testAll.summary', { ok: okCount, total: items.length })}</span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs text-muted">{t('testAll.sort')}</span>
        <Select
          size="sm"
          aria-label={t('testAll.sort')}
          value={sortKey}
          onValueChange={(next) => setSortKey(next as SortKey)}
          options={[
            { value: 'latency', label: t('testAll.sort.latency') },
            { value: 'name', label: t('testAll.sort.name') },
            { value: 'result', label: t('testAll.sort.result') },
          ]}
        />
        <Button size="sm" variant="outline" onClick={() => testAll.mutate()} loading={testAll.isPending}>
          <RefreshCw size={14} /> {t('testAll.retry')}
        </Button>
      </div>
    </div>
  )

  if (items.length === 0) {
    return <Card className="eg-card-fluid">{header}<p className="py-6 text-center text-sm text-muted">{t('testAll.none')}</p></Card>
  }

  return (
    <Card flush>
      <div className="eg-card-fluid">{header}</div>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[560px]">
          <thead className="border-b border-line-soft bg-surface-soft">
            <tr>
              <Th>{t('table.channel')}</Th>
              <Th>{t('testAll.sort.result')}</Th>
              <Th>{t('testAll.col.latency')}</Th>
              <Th>{t('testAll.col.error')}</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line-soft">
            {items.map((item) => (
              <tr key={item.channel_id}>
                <Td className="font-medium text-foreground">{item.name}</Td>
                <Td><StatusBadge tone={item.ok ? 'success' : item.result === 'inconclusive' ? 'neutral' : 'danger'}>{item.ok ? t('testAll.ok') : item.result === 'inconclusive' ? t('keyState.inconclusive') : t('testAll.fail')}</StatusBadge></Td>
                <Td className="tabular-nums">{item.latency_ms ? fmtMs(item.latency_ms) : '—'}</Td>
                <Td className="max-w-md truncate text-xs text-muted" title={item.error}>{item.error || '—'}</Td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

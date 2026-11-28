import { useEffect, useRef } from 'react'
import { RefreshCw } from 'lucide-react'
import { useChannelsTestAll } from '../api/hooks'
import { Alert, Button, Card, DataTable, type DataTableColumn, StatusBadge } from './ui'
import type { TestAllItem } from '../api/types'
import { fmtMs } from '../lib/format'
import { t, tf } from '../lib/i18n'
import { errorMessage } from '../lib/errors'

/** 渠道批量探测结果：一键摸底、按延迟排序；只调用模型列表 GET，不触发推理。 */
export default function TestAllPanel() {
  const testAll = useChannelsTestAll()
  // 面板打开即开始探测：此前首屏是另一个同名「测试全部」按钮，用户点两次才知道要再点一次
  const started = useRef(false)
  useEffect(() => {
    if (started.current) return
    started.current = true
    if (!testAll.data && !testAll.isPending) testAll.mutate()
  }, [testAll])

  const items = testAll.data?.items ?? []
  // 默认按延迟升序（无延迟的失败项沉底）；表头可切换为名称 / 结果
  const columns: DataTableColumn<TestAllItem>[] = [
      { id: 'name', header: t('table.channel'), className: 'font-medium text-foreground', sortKey: (item) => item.name, cell: (item) => item.name },
      {
        id: 'result',
        header: t('testAll.sort.result'),
        sortKey: (item) => (item.ok ? 0 : item.result === 'inconclusive' ? 1 : 2),
        cell: (item) => <StatusBadge tone={item.ok ? 'success' : item.result === 'inconclusive' ? 'neutral' : 'danger'}>{item.ok ? t('testAll.ok') : item.result === 'inconclusive' ? t('keyState.inconclusive') : t('testAll.fail')}</StatusBadge>,
      },
      { id: 'latency', header: t('testAll.col.latency'), className: 'tabular-nums', sortKey: (item) => item.latency_ms || Number.MAX_SAFE_INTEGER, cell: (item) => (item.latency_ms ? fmtMs(item.latency_ms) : '—') },
      { id: 'error', header: t('testAll.col.error'), className: 'max-w-md text-xs text-muted-foreground', cell: (item) => <span className="block truncate" title={item.error}>{item.error || '—'}</span> },
  ]

  if (testAll.isPending && !testAll.data) {
    return (
      <div role="status">
        <Card className="eg-card-fluid text-sm text-muted-foreground">
          <RefreshCw size={14} className="mr-2 inline animate-spin" aria-hidden="true" />
          {t('testAll.probing')}
        </Card>
      </div>
    )
  }
  if (testAll.isError && !testAll.data) {
    return <Alert tone="danger">{tf('testAll.failed', { message: errorMessage(testAll.error) })}</Alert>
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
        {t('testAll.title')} <span className="ml-2 text-xs font-normal text-muted-foreground">{tf('testAll.summary', { ok: okCount, total: items.length })}</span>
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" onClick={() => testAll.mutate()} loading={testAll.isPending}>
          <RefreshCw size={14} /> {t('testAll.retry')}
        </Button>
      </div>
    </div>
  )

  if (items.length === 0) {
    return <Card className="eg-card-fluid">{header}<p className="py-6 text-center text-sm text-muted-foreground">{t('testAll.none')}</p></Card>
  }

  return (
    <Card flush>
      <div className="eg-card-fluid">{header}</div>
      <DataTable columns={columns} rows={items} rowKey={(item) => item.channel_id} minWidth={560} defaultSorting={[{ id: 'latency', desc: false }]} />
    </Card>
  )
}

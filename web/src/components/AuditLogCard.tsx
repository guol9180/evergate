import { useState } from 'react'
import { ScrollText } from 'lucide-react'
import { useAuditLogs } from '../api/hooks'
import type { AuditLog } from '../api/types'
import { Card, ContentSwap, DataTable, type DataTableColumn, Empty, Field, LoadError, Pagination, Select, SkeletonRows } from './ui'
import { fmtTime } from '../lib/format'
import { t, type MessageKey } from '../lib/i18n'

// 动作名 → 文案键；未知动作原样显示，保证新增审计动作不会显示为空白。
const ACTION_KEYS: Record<string, MessageKey> = {
  'channel.create': 'audit.action.channel.create',
  'channel.update': 'audit.action.channel.update',
  'channel.delete': 'audit.action.channel.delete',
  'channel.toggle': 'audit.action.channel.toggle',
  'key.batch_add': 'audit.action.key.batch_add',
  'key.update': 'audit.action.key.update',
  'key.delete': 'audit.action.key.delete',
  'token.create': 'audit.action.token.create',
  'token.update': 'audit.action.token.update',
  'token.delete': 'audit.action.token.delete',
  'settings.update': 'audit.action.settings.update',
  'fallback.replace': 'audit.action.fallback.replace',
  'price.default_set': 'audit.action.price.default_set',
  'price.upsert': 'audit.action.price.upsert',
  'price.delete': 'audit.action.price.delete',
  'price.apply_preset': 'audit.action.price.apply_preset',
  'admin.change_password': 'audit.action.admin.change_password',
  'system.update_apply': 'audit.action.system.update_apply',
  'system.update_cancel': 'audit.action.system.update_cancel',
  'config.export': 'audit.action.config.export',
  'config.import': 'audit.action.config.import',
  'backup.create': 'audit.action.backup.create',
  'backup.delete': 'audit.action.backup.delete',
  'backup.restore': 'audit.action.backup.restore',
  'alerts.update': 'audit.action.alerts.update',
  setup_admin: 'audit.action.setup_admin',
}

/** 筛选栏的动作选项：按文案排序，用户看到的是本地化名称而不是内部动作名。 */
function actionOptions(): { value: string; label: string }[] {
  return Object.entries(ACTION_KEYS)
    .map(([value, key]) => ({ value, label: t(key) }))
    .sort((a, b) => a.label.localeCompare(b.label, 'zh'))
}

export default function AuditLogCard() {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [action, setAction] = useState('')
  const logs = useAuditLogs(page, pageSize, action || undefined)
  const columns: DataTableColumn<AuditLog>[] = [
      { id: 'time', header: t('table.time'), className: 'whitespace-nowrap text-xs text-muted-foreground', sortKey: (log) => log.created_at, cell: (log) => fmtTime(log.created_at) },
      { id: 'user', header: t('table.user'), className: 'text-xs', sortKey: (log) => log.username, cell: (log) => log.username || '—' },
      { id: 'action', header: t('table.action'), className: 'whitespace-nowrap text-xs font-medium', sortKey: (log) => log.action, cell: (log) => (ACTION_KEYS[log.action] ? t(ACTION_KEYS[log.action]) : log.action) },
      { id: 'target', header: t('table.target'), className: 'text-xs text-muted-foreground', cell: (log) => log.target || '—' },
      { id: 'summary', header: t('table.summary'), className: 'max-w-sm text-xs text-muted-foreground', cell: (log) => <span className="block truncate" title={log.summary}>{log.summary || '—'}</span> },
      { id: 'ip', header: 'IP', className: 'hidden text-xs text-muted-foreground min-[1100px]:table-cell', hideable: true, cell: (log) => log.ip },
  ]
  const items = logs.data?.items ?? []
  const total = logs.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold text-foreground">{t('audit.title')}</h2>
      <p className="mb-3 text-xs text-muted-foreground">{t('audit.desc')}</p>
      <div className="mb-3 sm:max-w-xs">
        <Field label={t('audit.filter.action')}>
          <Select
            value={action}
            onValueChange={(next) => { setAction(next); setPage(1) }}
            options={[{ value: '', label: t('audit.filter.allActions') }, ...actionOptions()]}
          />
        </Field>
      </div>
      <ContentSwap loading={logs.isPending} skeleton={<SkeletonRows rows={3} />}>
        {logs.isError && !logs.data ? (
          <LoadError error={logs.error} onRetry={() => void logs.refetch()} />
        ) : items.length === 0 ? (
          <Empty icon={<ScrollText size={32} />} title={t('empty.audit.title')} hint={t('empty.audit.hint')} />
        ) : (
          <DataTable columns={columns} rows={items} rowKey={(log) => log.id} minWidth={680} scrollToTopOn={page} />
        )}
      </ContentSwap>
      {items.length > 0 && (
        <Pagination
          page={page}
          totalPages={totalPages}
          total={total}
          pageSize={pageSize}
          sizes={[10, 20, 50]}
          onPageChange={setPage}
          onPageSizeChange={(size) => { setPageSize(size); setPage(1) }}
          className="mt-2"
        />
      )}
    </Card>
  )
}

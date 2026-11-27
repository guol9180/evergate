import { useState } from 'react'
import { ScrollText } from 'lucide-react'
import { useAuditLogs } from '../api/hooks'
import { Card, ContentSwap, Empty, LoadError, Pagination, SkeletonRows, Td, Th } from './ui'
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
  'admin.change_password': 'audit.action.admin.change_password',
  'system.update_apply': 'audit.action.system.update_apply',
  'system.update_cancel': 'audit.action.system.update_cancel',
  'config.export': 'audit.action.config.export',
  'config.import': 'audit.action.config.import',
  setup_admin: 'audit.action.setup_admin',
}

export default function AuditLogCard() {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const logs = useAuditLogs(page, pageSize)
  const items = logs.data?.items ?? []
  const total = logs.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / pageSize))

  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold text-foreground">{t('audit.title')}</h2>
      <p className="mb-3 text-xs text-muted-foreground">{t('audit.desc')}</p>
      <ContentSwap loading={logs.isPending} skeleton={<SkeletonRows rows={3} />}>
        {logs.isError && !logs.data ? (
          <LoadError error={logs.error} onRetry={() => void logs.refetch()} />
        ) : items.length === 0 ? (
          <Empty icon={<ScrollText size={32} />} title={t('empty.audit.title')} hint={t('empty.audit.hint')} />
        ) : (
          <div className="overflow-x-auto">
          <table className="w-full min-w-[680px]">
            <thead className="border-b border-line-soft bg-surface-soft">
              <tr>
                <Th>{t('table.time')}</Th><Th>{t('table.user')}</Th><Th>{t('table.action')}</Th><Th>{t('table.target')}</Th><Th>{t('table.summary')}</Th><Th className="hidden min-[1100px]:table-cell">IP</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {items.map((log) => (
                <tr key={log.id}>
                  <Td className="whitespace-nowrap text-xs text-muted-foreground">{fmtTime(log.created_at)}</Td>
                  <Td className="text-xs">{log.username || '—'}</Td>
                  <Td className="whitespace-nowrap text-xs font-medium">{ACTION_KEYS[log.action] ? t(ACTION_KEYS[log.action]) : log.action}</Td>
                  <Td className="text-xs text-muted-foreground">{log.target || '—'}</Td>
                  <Td className="max-w-sm truncate text-xs text-muted-foreground" title={log.summary}>{log.summary || '—'}</Td>
                  <Td className="hidden text-xs text-muted-foreground min-[1100px]:table-cell">{log.ip}</Td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
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

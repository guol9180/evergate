import { useState } from 'react'
import { ScrollText } from 'lucide-react'
import { useAuditLogs } from '../api/hooks'
import { Card, Empty, Pagination, SkeletonRows, Td, Th } from './ui'
import { fmtTime } from '../lib/format'
import { t } from '../lib/i18n'

// 动作名 → 中文；未知动作原样显示，保证新增审计动作不会显示为空白。
const ACTION_LABEL: Record<string, string> = {
  'channel.create': '创建渠道',
  'channel.update': '更新渠道',
  'channel.delete': '删除渠道',
  'channel.toggle': '启停渠道',
  'key.batch_add': '导入 key',
  'key.update': '更新 key',
  'key.delete': '删除 key',
  'token.create': '创建令牌',
  'token.update': '更新令牌',
  'token.delete': '删除令牌',
  'settings.update': '修改系统设置',
  'fallback.replace': '替换降级链',
  'price.default_set': '设置默认价格',
  'price.upsert': '设置价格覆盖',
  'price.delete': '删除价格覆盖',
  'admin.change_password': '修改管理员密码',
  'system.update_apply': '应用系统更新',
  'system.update_cancel': '取消系统更新',
  'config.export': '导出配置',
  'config.import': '导入配置',
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
      <h2 className="mb-1 text-sm font-semibold text-foreground">审计日志</h2>
      <p className="mb-3 text-xs text-muted">管理台写操作记录（渠道 / 密钥 / 令牌 / 设置 / 配置导入导出）；不记录任何密钥明文。</p>
      {logs.isPending ? (
        <SkeletonRows rows={3} />
      ) : items.length === 0 ? (
        <Empty icon={<ScrollText size={32} />} title={t('empty.audit.title')} hint={t('empty.audit.hint')} />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[680px]">
            <thead className="border-b border-line-soft bg-surface-soft">
              <tr>
                <Th>时间</Th><Th>用户</Th><Th>动作</Th><Th>目标</Th><Th>摘要</Th><Th className="hidden min-[1100px]:table-cell">IP</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {items.map((log) => (
                <tr key={log.id}>
                  <Td className="whitespace-nowrap text-xs text-muted">{fmtTime(log.created_at)}</Td>
                  <Td className="text-xs">{log.username || '—'}</Td>
                  <Td className="whitespace-nowrap text-xs font-medium">{ACTION_LABEL[log.action] ?? log.action}</Td>
                  <Td className="text-xs text-muted">{log.target || '—'}</Td>
                  <Td className="max-w-sm truncate text-xs text-muted" title={log.summary}>{log.summary || '—'}</Td>
                  <Td className="hidden text-xs text-muted min-[1100px]:table-cell">{log.ip}</Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
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

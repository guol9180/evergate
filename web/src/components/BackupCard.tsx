import { useRef, useState } from 'react'
import { DatabaseBackup, Download, Trash2, Upload } from 'lucide-react'
import { downloadAdminFile, useBackups, useCreateBackup, useDeleteBackup, useRestoreBackup } from '../api/hooks'
import { Alert, Button, Card, ConfirmDialog, Empty, ErrorText, IconButton, SkeletonRows, Td, Th } from './ui'
import { notify } from './Toaster'
import { fmtBytes, fmtTime } from '../lib/format'
import { t, tf } from '../lib/i18n'
import { errorMessage } from '../lib/errors'

/** 整库备份：创建 / 下载 / 删除，以及上传备份暂存恢复（下次启动生效）。 */
export default function BackupCard() {
  const backups = useBackups()
  const create = useCreateBackup()
  const remove = useDeleteBackup()
  const restore = useRestoreBackup()
  const fileRef = useRef<HTMLInputElement>(null)
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null)
  const [restoreTarget, setRestoreTarget] = useState<{ file: File } | null>(null)
  const [error, setError] = useState('')
  const [staged, setStaged] = useState('')

  const onCreate = () => create.mutate(undefined, {
    onSuccess: (resp) => notify.success(tf('backups.created', { name: resp.name })),
    onError: (err) => setError(errorMessage(err)),
  })
  const onDownload = async (name: string) => {
    try {
      await downloadAdminFile(`/admin/api/backups/${encodeURIComponent(name)}`, name)
    } catch (err) {
      notify.error(err, t('backups.downloadFailed'))
    }
  }
  const onPickFile = (file: File | undefined) => {
    if (fileRef.current) fileRef.current.value = ''
    if (!file) return
    setError('')
    setRestoreTarget({ file })
  }
  const confirmRestore = () => {
    if (!restoreTarget) return
    restore.mutate(restoreTarget.file, {
      onSuccess: () => {
        setStaged(restoreTarget.file.name)
        setRestoreTarget(null)
      },
      onError: (err) => setError(errorMessage(err)),
    })
  }

  const items = backups.data?.items ?? []
  const pending = backups.data?.restart_pending ?? false

  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold text-foreground">
          <DatabaseBackup size={15} /> {t('backups.title')}
        </h2>
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" variant="outline" onClick={() => fileRef.current?.click()}>
            <Upload size={14} /> {t('backups.restore')}
          </Button>
          <Button type="button" size="sm" loading={create.isPending} onClick={onCreate}>
            {t('backups.create')}
          </Button>
          <input
            ref={fileRef}
            type="file"
            accept=".db,application/octet-stream"
            className="sr-only"
            aria-label={t('backups.pickFile')}
            onChange={(e) => onPickFile(e.target.files?.[0])}
          />
        </div>
      </div>
      <p className="mb-2 text-xs text-muted">{t('backups.desc')}</p>
      <p className="mb-4 text-xs text-subtle">{t('backups.masterKeyHint')}</p>

      {(pending || staged) && (
        <Alert tone="warning" className="mb-4">
          {staged ? tf('backups.staged', { name: staged }) : t('backups.pendingRestart')}
        </Alert>
      )}
      <ErrorText>{error}</ErrorText>

      {backups.isPending ? (
        <SkeletonRows rows={3} />
      ) : backups.isError ? (
        <Alert tone="danger">{errorMessage(backups.error)}</Alert>
      ) : items.length === 0 ? (
        <Empty
          icon={<DatabaseBackup size={36} />}
          title={t('backups.empty')}
          hint={backups.data?.dir}
        />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[560px]">
            <thead className="border-b border-line-soft bg-surface-soft">
              <tr>
                <Th>{t('backups.col.time')}</Th>
                <Th>{t('backups.col.size')}</Th>
                <Th>{t('backups.col.version')}</Th>
                <Th className="w-24"><span className="block pr-2.5 text-right">{t('table.actions')}</span></Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line-soft">
              {items.map((item) => (
                <tr key={item.name} className="hover:bg-surface-soft">
                  <Td>
                    <div className="text-xs">{fmtTime(item.time)}</div>
                    <div className="font-mono text-[11px] text-subtle">{item.name}</div>
                  </Td>
                  <Td className="text-xs text-muted">{fmtBytes(item.size)}</Td>
                  <Td className="text-xs text-muted">{item.version || '—'}</Td>
                  <Td>
                    <div className="flex justify-end gap-1">
                      <IconButton label={t('backups.download')} side="top" onClick={() => void onDownload(item.name)}>
                        <Download size={13} />
                      </IconButton>
                      <IconButton label={t('backups.delete')} side="top" danger onClick={() => setDeleteTarget(item.name)}>
                        <Trash2 size={13} />
                      </IconButton>
                    </div>
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {backups.data?.dir && items.length > 0 && (
        <div className="mt-3 break-all text-[11px] text-subtle">{tf('backups.dir', { dir: backups.data.dir })}</div>
      )}

      <ConfirmDialog
        open={deleteTarget != null}
        title={t('backups.deleteTitle')}
        danger
        confirmText={t('backups.delete')}
        loading={remove.isPending}
        message={tf('backups.deleteMessage', { name: deleteTarget ?? '' })}
        onConfirm={() => deleteTarget && remove.mutate(deleteTarget, {
          onSuccess: () => { notify.success(t('backups.deleted')); setDeleteTarget(null) },
          onError: (err) => setError(errorMessage(err)),
        })}
        onCancel={() => setDeleteTarget(null)}
      />
      <ConfirmDialog
        open={restoreTarget != null}
        title={t('backups.restoreTitle')}
        danger
        confirmText={t('backups.restoreConfirm')}
        loading={restore.isPending}
        message={restoreTarget ? tf('backups.restoreMessage', { name: restoreTarget.file.name }) : ''}
        onConfirm={confirmRestore}
        onCancel={() => setRestoreTarget(null)}
      />
    </Card>
  )
}

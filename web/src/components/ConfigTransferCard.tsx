import { useRef, useState } from 'react'
import { Download, Upload } from 'lucide-react'
import { downloadAdminFile, useImportConfig } from '../api/hooks'
import type { ConfigImportResult } from '../api/types'
import { Button, Card, ConfirmDialog, ErrorText, SuccessText } from './ui'
import { t, tf } from '../lib/i18n'

/** 配置导出 / 导入：导出永不含上游 key 与令牌哈希；导入按名称幂等 upsert，导入前需确认。 */
export default function ConfigTransferCard() {
  const importConfig = useImportConfig()
  const fileRef = useRef<HTMLInputElement>(null)
  const [pending, setPending] = useState<unknown>(null)
  const [pendingName, setPendingName] = useState('')
  const [result, setResult] = useState<ConfigImportResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const exportConfig = async () => {
    setBusy(true)
    setError('')
    try {
      await downloadAdminFile('/admin/api/config/export', 'evergate-config.json')
    } catch (err) {
      setError((err as Error).message || t('config.exportFailed'))
    } finally {
      setBusy(false)
    }
  }

  const onPickFile = async (file: File | undefined) => {
    if (!file) return
    setError('')
    setResult(null)
    try {
      const text = await file.text()
      const parsed: unknown = JSON.parse(text)
      const payload = parsed && typeof parsed === 'object' && 'data' in (parsed as Record<string, unknown>)
        ? (parsed as { data: unknown }).data
        : parsed
      const version = (payload as { version?: number } | null)?.version
      if (version !== 1) throw new Error(t('config.notEverGate'))
      setPending(payload)
      setPendingName(file.name)
    } catch (err) {
      setError(tf('config.readFailed', { message: (err as Error).message }))
    } finally {
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const confirmImport = () => {
    if (!pending) return
    setBusy(true)
    importConfig.mutate(pending, {
      onSuccess: (resp) => {
        setResult(resp.result)
        setPending(null)
      },
      onError: (err) => setError((err as Error).message || t('config.importFailed')),
      onSettled: () => setBusy(false),
    })
  }

  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold text-foreground">{t('config.title')}</h2>
      <p className="mb-4 text-xs text-muted">{t('config.desc')}</p>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" size="sm" variant="outline" onClick={() => void exportConfig()} disabled={busy}>
          <Download size={14} /> {busy ? t('config.exporting') : t('config.export')}
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={() => fileRef.current?.click()} disabled={busy}>
          <Upload size={14} /> {t('config.import')}
        </Button>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          className="sr-only"
          aria-label={t('config.pickFile')}
          onChange={(e) => void onPickFile(e.target.files?.[0])}
        />
      </div>
      <ErrorText>{error}</ErrorText>
      {result && (
        <div role="status" className="mt-3 rounded-md bg-success-subtle px-3 py-2 text-xs text-success-text">
          {tf('config.importResult', { created: result.created, updated: result.updated, skipped: result.skipped })}
          {result.errors && result.errors.length > 0 && (
            <ul className="mt-1 list-disc space-y-0.5 pl-4">
              {result.errors.map((line, i) => <li key={i}>{line}</li>)}
            </ul>
          )}
        </div>
      )}
      {result && result.errors?.length === 0 && <SuccessText>{t('config.importDone')}</SuccessText>}

      <ConfirmDialog
        open={pending != null}
        title={t('config.import')}
        confirmText={t('config.importConfirm')}
        loading={busy}
        message={tf('config.importMessage', { name: pendingName })}
        onConfirm={confirmImport}
        onCancel={() => setPending(null)}
      />
    </Card>
  )
}

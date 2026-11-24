import { useRef, useState } from 'react'
import { Download, Upload } from 'lucide-react'
import { downloadAdminFile, useImportConfig } from '../api/hooks'
import type { ConfigImportResult } from '../api/types'
import { Button, Card, ConfirmDialog, ErrorText, SuccessText } from './ui'

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
      setError((err as Error).message || '导出失败')
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
      if (version !== 1) throw new Error('不是 EverGate 配置文件（version 应为 1）')
      setPending(payload)
      setPendingName(file.name)
    } catch (err) {
      setError(`读取失败：${(err as Error).message}`)
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
      onError: (err) => setError((err as Error).message || '导入失败'),
      onSettled: () => setBusy(false),
    })
  }

  return (
    <Card>
      <h2 className="mb-1 text-sm font-semibold text-foreground">配置导出 / 导入</h2>
      <p className="mb-4 text-xs text-muted">
        导出渠道（不含上游 key）、模型降级链、系统设置、价格与令牌治理字段为 JSON；导入按名称幂等更新，令牌需重新创建以获得新 key。
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" size="sm" variant="outline" onClick={() => void exportConfig()} disabled={busy}>
          <Download size={14} /> {busy ? '导出中…' : '导出配置'}
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={() => fileRef.current?.click()} disabled={busy}>
          <Upload size={14} /> 导入配置
        </Button>
        <input
          ref={fileRef}
          type="file"
          accept="application/json,.json"
          className="sr-only"
          aria-label="选择配置文件"
          onChange={(e) => void onPickFile(e.target.files?.[0])}
        />
      </div>
      <ErrorText>{error}</ErrorText>
      {result && (
        <div role="status" className="mt-3 rounded-md bg-success-subtle px-3 py-2 text-xs text-success-text">
          导入完成：新增 {result.created} · 更新 {result.updated} · 跳过 {result.skipped}
          {result.errors && result.errors.length > 0 && (
            <ul className="mt-1 list-disc space-y-0.5 pl-4">
              {result.errors.map((line, i) => <li key={i}>{line}</li>)}
            </ul>
          )}
        </div>
      )}
      {result && result.errors?.length === 0 && <SuccessText>全部条目已生效。</SuccessText>}

      <ConfirmDialog
        open={pending != null}
        title="导入配置"
        confirmText="开始导入"
        loading={busy}
        message={<>将按名称更新已有渠道与降级链、覆盖系统设置与价格；不会触碰任何上游 key。文件：{pendingName}。此操作会覆盖当前同名配置。</>}
        onConfirm={confirmImport}
        onCancel={() => setPending(null)}
      />
    </Card>
  )
}

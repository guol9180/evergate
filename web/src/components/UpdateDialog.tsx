import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, CheckCircle2, Download, ExternalLink, Loader2, RefreshCw, ShieldCheck } from 'lucide-react'
import { useApplyUpdate, useUpdateProgress } from '../api/hooks'
import type { UpdateJob, UpdateStatus } from '../api/types'
import { Badge, Button, Dialog, Field, Select } from './ui'

const stageLabel: Record<string, string> = {
  downloading: '下载新版本',
  verifying: '校验完整性',
  replacing: '替换二进制',
  restarting: '重启服务',
  failed: '更新失败',
}

const busyStages = new Set(['downloading', 'verifying', 'replacing', 'restarting'])

export function UpdateDialog({ open, onClose, status }: { open: boolean; onClose: () => void; status?: UpdateStatus }) {
  const [source, setSource] = useState('auto')
  const [started, setStarted] = useState(false)
  const [snapshot, setSnapshot] = useState<UpdateJob | null>(null)
  const [restartTimedOut, setRestartTimedOut] = useState(false)
  const progress = useUpdateProgress(open)
  const apply = useApplyUpdate()

  const liveJob = progress.data?.job
  const job = liveJob ?? snapshot
  const capability = progress.data?.capability
  const current = progress.data?.current_version ?? status?.current_version ?? ''
  const target = job?.target_version ?? status?.latest_version ?? ''
  const normalizedCurrent = current.replace(/^v/, '')
  const normalizedTarget = target.replace(/^v/, '')
  const busy = started && !!job && busyStages.has(job.stage)
  const restarting = started && job?.stage === 'restarting'
  const failed = job?.stage === 'failed' || !!apply.error

  useEffect(() => {
    if (liveJob) setSnapshot(liveJob)
  }, [liveJob])

  useEffect(() => {
    if (!started || !normalizedTarget) return
    if (normalizedCurrent === normalizedTarget) window.location.reload()
  }, [started, normalizedCurrent, normalizedTarget])

  useEffect(() => {
    if (!restarting) return
    const timer = window.setTimeout(() => setRestartTimedOut(true), 45_000)
    return () => window.clearTimeout(timer)
  }, [restarting])

  useEffect(() => {
    if (open) return
    setStarted(false)
    setSnapshot(null)
    setRestartTimedOut(false)
    apply.reset()
  }, [open])

  const stages = useMemo(
    () => ['downloading', 'verifying', 'replacing', 'restarting'],
    [],
  )

  const begin = () => {
    setStarted(true)
    setSnapshot(null)
    setRestartTimedOut(false)
    apply.mutate({ version: status?.latest_version, source })
  }

  const close = () => {
    if (busy || restarting) return
    onClose()
  }

  return (
    <Dialog
      open={open}
      onClose={close}
      title="更新 EverGate"
      footer={
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-slate-400">
            {status?.release_url && (
              <a href={status.release_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:text-indigo-600">
                更新说明 <ExternalLink size={12} />
              </a>
            )}
          </span>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={close} disabled={busy || restarting}>关闭</Button>
            <Button onClick={begin} disabled={busy || restarting || !status?.latest_version || capability?.available === false || !status?.update_available}>
              <Download size={14} /> 立即更新
            </Button>
          </div>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="rounded-lg border border-slate-200 bg-slate-50/60 px-3 py-3 text-sm">
          <div className="flex items-center justify-between gap-3">
            <span className="text-slate-500">当前版本</span>
            <span className="font-medium tabular-nums text-slate-800">{current || '未知'}</span>
          </div>
          <div className="mt-2 flex items-center justify-between gap-3">
            <span className="text-slate-500">目标版本</span>
            <span className="font-medium tabular-nums text-indigo-600">{target || '—'}</span>
          </div>
          {job?.source && (
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-slate-500">下载通道</span>
              <Badge tone="bg-indigo-100 text-indigo-700">{job.source}</Badge>
            </div>
          )}
        </div>

        {capability && !capability.available && (
          <div className="flex gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
            <AlertTriangle size={15} className="mt-0.5 shrink-0" />
            <span>{capability.reason || '当前环境不支持自动更新'}</span>
          </div>
        )}

        {!started && (
          <Field label="下载通道" hint="自动模式先走 npm registry，失败后回退 GitHub Releases。">
            <Select value={source} onChange={(e) => setSource(e.target.value)}>
              <option value="auto">自动（npm → GitHub）</option>
              <option value="npm">npm registry</option>
              <option value="github">GitHub Releases</option>
            </Select>
          </Field>
        )}

        {started && (
          <div className="space-y-3">
            <div className="flex items-center justify-between text-xs text-slate-500">
              <span className="font-medium text-slate-700">{stageLabel[job?.stage || 'downloading'] || job?.stage}</span>
              <span className="tabular-nums">{job?.progress ?? 0}%</span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-slate-100">
              <div
                className={`h-full rounded-full transition-all ${failed ? 'bg-red-500' : 'bg-indigo-500'}`}
                style={{ width: `${job?.progress ?? 0}%` }}
              />
            </div>
            <ol className="space-y-2">
              {stages.map((stage, index) => {
                const failedHere = job?.stage === 'failed' && job.failed_stage === stage
                const failedAfter = job?.stage === 'failed' && !!job.failed_stage && stages.indexOf(job.failed_stage) > index
                const done = job ? stages.indexOf(job.stage) > index || (job.stage === 'restarting' && index < 3) || failedAfter : false
                const active = job?.stage === stage
                return (
                  <li key={stage} className="flex items-center gap-2 text-xs">
                    {failedHere ? <AlertTriangle size={14} className="text-red-500" />
                      : done ? <CheckCircle2 size={14} className="text-emerald-500" />
                      : active ? <Loader2 size={14} className="animate-spin text-indigo-500" />
                        : <span className="h-3.5 w-3.5 rounded-full border border-slate-300" />}
                    <span className={failedHere ? 'font-medium text-red-600' : active ? 'font-medium text-slate-700' : 'text-slate-500'}>{stageLabel[stage]}</span>
                  </li>
                )
              })}
            </ol>
            <div className="flex items-start gap-2 rounded-lg bg-slate-50 px-3 py-2 text-xs text-slate-500">
              {restarting ? <RefreshCw size={14} className="mt-0.5 shrink-0 animate-spin text-indigo-500" /> : <ShieldCheck size={14} className="mt-0.5 shrink-0 text-emerald-500" />}
              <div className="min-w-0 space-y-1">
                <div>
                  {restarting
                    ? '服务正在优雅重启，新版本就绪后页面会自动刷新。'
                    : job?.message || '下载内容会先做完整性校验，校验失败不会替换当前版本。'}
                </div>
                {job?.backup_path && <div className="break-all text-slate-400">旧版本备份：{job.backup_path}</div>}
              </div>
            </div>
          </div>
        )}

        {failed && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
            <div className="font-medium">更新未完成</div>
            <div className="mt-1 max-h-28 overflow-auto break-words">{job?.error || (apply.error as Error)?.message || '未知错误'}</div>
            {job?.backup_path && (
              <div className="mt-1 break-all text-red-600/80">旧版本备份：{job.backup_path}</div>
            )}
          </div>
        )}

        {restartTimedOut && restarting && (
          <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
            重启耗时超过 45 秒。请检查进程是否仍在运行；若新版本启动失败，可使用上方备份路径手动回滚。
          </div>
        )}
      </div>
    </Dialog>
  )
}

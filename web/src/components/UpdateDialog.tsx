import { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, CheckCircle2, Download, ExternalLink, Loader2, RefreshCw, ShieldCheck } from 'lucide-react'
import { useApplyUpdate, useCancelUpdate, useUpdateProgress } from '../api/hooks'
import type { UpdateJob, UpdateStatus } from '../api/types'
import { Alert, Badge, Button, Dialog, Field, Progress, Select } from './ui'
import { t, tf, type MessageKey } from '../lib/i18n'

const stageKey: Record<string, MessageKey> = {
  downloading: 'update.stage.downloading',
  verifying: 'update.stage.verifying',
  installing: 'update.stage.installing',
  restarting: 'update.stage.restarting',
  failed: 'update.stage.failed',
}

const busyStages = new Set(['downloading', 'verifying', 'installing', 'restarting'])

function sourceLabel(source?: string) {
  if (!source) return ''
  if (source === 'github') return 'GitHub Releases'
  if (source.startsWith('npm:')) return `npm · ${source.slice(4)}`
  return source
}

export function UpdateDialog({ open, onClose, status }: { open: boolean; onClose: () => void; status?: UpdateStatus }) {
  const [source, setSource] = useState('auto')
  const [started, setStarted] = useState(false)
  const [snapshot, setSnapshot] = useState<UpdateJob | null>(null)
  const [restartTimedOut, setRestartTimedOut] = useState(false)
  const progress = useUpdateProgress(open)
  const apply = useApplyUpdate()
  const cancel = useCancelUpdate()

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
  }, [apply, open])

  const stages = useMemo(
    () => ['downloading', 'verifying', 'installing', 'restarting'],
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
      title={t('update.title')}
      footer={
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-subtle">
            {status?.release_url && (
              <a href={status.release_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:text-brand-text">
                {t('update.notes')} <ExternalLink size={12} />
              </a>
            )}
          </span>
          <div className="flex items-center gap-2">
            {job?.cancelable && (
              <Button variant="outline" onClick={() => cancel.mutate()} loading={cancel.isPending}>
                {t('update.cancel')}
              </Button>
            )}
            <Button variant="outline" onClick={close} disabled={busy || restarting}>{t('common.close')}</Button>
            <Button onClick={begin} loading={apply.isPending} disabled={busy || restarting || !status?.latest_version || capability?.available === false || !status?.update_available}>
              <Download size={14} /> {t('update.now')}
            </Button>
          </div>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="rounded-lg border border-line bg-surface-soft px-3 py-3 text-sm">
          <div className="flex items-center justify-between gap-3">
            <span className="text-muted-foreground">{t('update.currentVersion')}</span>
            <span className="font-medium tabular-nums text-foreground-soft">{current || t('common.unknown')}</span>
          </div>
          <div className="mt-2 flex items-center justify-between gap-3">
            <span className="text-muted-foreground">{t('update.targetVersion')}</span>
            <span className="font-medium tabular-nums text-brand-text">{target || '—'}</span>
          </div>
          {status?.source && (
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-muted-foreground">{t('update.checkSource')}</span>
              <Badge tone="neutral">
                <span className="max-w-[220px] truncate" title={sourceLabel(status.source)}>{sourceLabel(status.source)}</span>
              </Badge>
            </div>
          )}
          {job?.source && (
            <div className="mt-2 flex items-center justify-between gap-3">
              <span className="text-muted-foreground">{t('update.channel')}</span>
              <Badge tone="brand">
                <span className="max-w-[220px] truncate" title={sourceLabel(job.source)}>{sourceLabel(job.source)}</span>
              </Badge>
            </div>
          )}
        </div>

        {capability && !capability.available && (
          <Alert tone="warning">{capability.reason || t('update.capability.unsupported')}</Alert>
        )}

        {!started && (
          <Field label={t('update.channel')} hint={t('update.channelHint')}>
            <Select
              value={source}
              onValueChange={setSource}
              options={[
                { value: 'auto', label: t('update.channel.auto') },
                { value: 'npm', label: t('update.channel.npm') },
                { value: 'github', label: t('update.channel.github') },
              ]}
            />
          </Field>
        )}

        {started && (
          <div className="space-y-3">
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span className="font-medium text-foreground-soft">{t(stageKey[job?.stage || 'downloading'] ?? 'update.stage.failed')}</span>
              <span className="tabular-nums">{job?.progress ?? 0}%</span>
            </div>
            <Progress value={job?.progress ?? 0} tone={failed ? 'danger' : 'brand'} />
            <ol className="space-y-2">
              {stages.map((stage, index) => {
                const failedHere = job?.stage === 'failed' && job.failed_stage === stage
                const failedAfter = job?.stage === 'failed' && !!job.failed_stage && stages.indexOf(job.failed_stage) > index
                const done = job ? stages.indexOf(job.stage) > index || (job.stage === 'restarting' && index < 3) || failedAfter : false
                const active = job?.stage === stage
                return (
                  <li key={stage} className="flex items-center gap-2 text-xs">
                    {failedHere ? <AlertTriangle size={14} className="text-danger-text" />
                      : done ? <CheckCircle2 size={14} className="text-success-text" />
                      : active ? <Loader2 size={14} className="animate-spin text-brand-text" />
                        : <span className="h-3.5 w-3.5 rounded-full border border-line-strong" />}
                    <span className={failedHere ? 'font-medium text-danger-text' : active ? 'font-medium text-foreground-soft' : 'text-muted-foreground'}>{t(stageKey[stage] ?? 'update.stage.failed')}</span>
                  </li>
                )
              })}
            </ol>
            <div className="flex items-start gap-2 rounded-lg bg-surface-soft px-3 py-2 text-xs text-muted-foreground">
              {restarting ? <RefreshCw size={14} className="mt-0.5 shrink-0 animate-spin text-brand-text" /> : <ShieldCheck size={14} className="mt-0.5 shrink-0 text-success-text" />}
              <div className="min-w-0 space-y-1">
                <div>
                  {restarting
                    ? t('update.restarting')
                    : job?.message || t('update.installing')}
                </div>
                {job?.backup_path && <div className="break-all text-subtle">{tf('update.backupPath', { path: job.backup_path })}</div>}
              </div>
            </div>
          </div>
        )}

        {failed && (
          <Alert tone="danger" title={t('update.incomplete')}>
            <div className="max-h-28 overflow-auto break-words">{job?.error || (apply.error as Error)?.message || t('update.unknownError')}</div>
            {job?.backup_path && (
              <div className="mt-1 break-all">{tf('update.backupPath', { path: job.backup_path })}</div>
            )}
          </Alert>
        )}

        {restartTimedOut && restarting && (
          <Alert tone="warning">{t('update.restartTimeout')}</Alert>
        )}
      </div>
    </Dialog>
  )
}

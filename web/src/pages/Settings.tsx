import { useEffect, useState, type FormEvent } from 'react'
import { Save } from 'lucide-react'
import { useSystemSettings, useUpdateSystemSettings } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'
import type { SystemSettings, SystemSettingsValues } from '../api/types'
import { Alert, Badge, Button, Card, Field, Input, PageHeader, Spinner } from '../components/ui'
import { notify } from '../components/Toaster'
import ModelFallbackCard from '../components/ModelFallbackCard'
import ConfigTransferCard from '../components/ConfigTransferCard'
import AuditLogCard from '../components/AuditLogCard'
import { fmtTime } from '../lib/format'

type FormState = {
  connectTimeoutSec: string
  firstTokenTimeoutSec: string
  overallTimeoutSec: string
  retryChannels: string
  retryKeysPerChannel: string
  retryTotalSec: string
  cooldownFailThreshold: string
  cooldownBaseSec: string
  cooldownMaxSec: string
  logRetentionDays: string
  healthProbeIntervalSec: string
  healthProbeConcurrency: string
  healthProbeTimeoutSec: string
}

export default function Settings() {
  usePageTitle(t('page.settings.title'))
  const settings = useSystemSettings()
  const save = useUpdateSystemSettings()
  const [form, setForm] = useState<FormState | null>(null)

  useEffect(() => {
    if (settings.data?.settings && form === null) {
      setForm(toForm(settings.data.settings))
    }
  }, [settings.data, form])

  if (settings.isPending) {
    return <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>
  }
  if (settings.isError || !form) {
    return (
      <div className="eg-page-stack">
        <PageHeader title={t('page.settings.title')} description={t('page.settings.desc')} />
        <Alert tone="danger">{settings.isError ? (settings.error as Error).message : t('common.settingsMissing')}</Alert>
      </div>
    )
  }

  const set = (key: keyof FormState) => (value: string) => setForm({ ...form, [key]: value })
  const current = settings.data?.settings

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(toSettings(form), {
      onSuccess: () => notify.success(t('settings.saved')),
      onError: (err) => notify.error(err),
    })
  }

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.settings.title')}
        description={t('page.settings.desc')}
        actions={
          <div className="text-left text-xs text-muted sm:text-right">
            <div>{t('settings.source')} <Badge tone="brand">{current?.source === 'database' ? t('settings.source.database') : 'evergate.yml'}</Badge></div>
            <div className="mt-1 break-words">{tf('settings.initialized', { init: fmtTime(current?.initialized_at), updated: fmtTime(current?.updated_at) })}</div>
          </div>
        }
      />

      {settings.data?.paths && (
        <Card>
          <h2 className="mb-3 text-sm font-semibold">{t('settings.paths')}</h2>
          <dl className="eg-grid eg-grid-auto-wide text-xs">
            <div className="min-w-0">
              <dt className="text-subtle">{t('settings.configFile')}</dt>
              <dd className="mt-1 break-all font-mono text-muted">{settings.data.paths.config_path || '—'}</dd>
            </div>
            <div className="min-w-0">
              <dt className="text-subtle">{t('settings.dataDir')}</dt>
              <dd className="mt-1 break-all font-mono text-muted">{settings.data.paths.data_dir || '—'}</dd>
            </div>
            <div className="min-w-0">
              <dt className="text-subtle">{t('settings.configSource')}</dt>
              <dd className="mt-1"><Badge tone="neutral">{settings.data.paths.config_source || t('cost.unknown')}</Badge></dd>
            </div>
          </dl>
        </Card>
      )}

      <form onSubmit={onSubmit} className="space-y-[var(--eg-section-gap)]">
        <Card>
          <h2 className="mb-4 text-sm font-semibold">{t('settings.upstreamTimeout')}</h2>
          <div className="eg-grid eg-grid-auto-wide">
        <NumberField label={t('channels.form.connectTimeout')} hint={t('settings.connectTimeoutHint')} value={form.connectTimeoutSec} onChange={set('connectTimeoutSec')} />
        <NumberField label={t('channels.form.firstTokenTimeout')} hint={t('settings.firstTokenTimeoutHint')} value={form.firstTokenTimeoutSec} onChange={set('firstTokenTimeoutSec')} />
        <NumberField label={t('channels.form.overallTimeout')} hint={t('settings.overallTimeoutHint')} value={form.overallTimeoutSec} onChange={set('overallTimeoutSec')} />
          </div>
        </Card>

        <Card>
          <h2 className="mb-4 text-sm font-semibold">{t('settings.retryCooldown')}</h2>
          <div className="eg-grid eg-grid-auto-wide">
        <NumberField label={t('settings.retryChannels')} hint={t('settings.range1to10')} value={form.retryChannels} onChange={set('retryChannels')} />
        <NumberField label={t('settings.retryKeysPerChannel')} hint={t('settings.range1to10')} value={form.retryKeysPerChannel} onChange={set('retryKeysPerChannel')} />
        <NumberField label={t('settings.retryTotalSec')} hint={t('settings.retryTotalHint')} value={form.retryTotalSec} onChange={set('retryTotalSec')} />
        <NumberField label={t('settings.cooldownFailThreshold')} hint={t('settings.range1to100')} value={form.cooldownFailThreshold} onChange={set('cooldownFailThreshold')} />
        <NumberField label={t('settings.cooldownBaseSec')} hint={t('settings.cooldownBaseHint')} value={form.cooldownBaseSec} onChange={set('cooldownBaseSec')} />
        <NumberField label={t('settings.cooldownMaxSec')} hint={t('settings.cooldownMaxHint')} value={form.cooldownMaxSec} onChange={set('cooldownMaxSec')} />
          </div>
        </Card>

        <Card>
          <h2 className="mb-4 text-sm font-semibold">{t('settings.maintenance')}</h2>
          <div className="eg-grid eg-grid-auto-wide">
        <NumberField label={t('settings.logRetentionDays')} hint={t('settings.retentionHint')} value={form.logRetentionDays} onChange={set('logRetentionDays')} />
        <NumberField label={t('settings.healthProbeInterval')} hint={t('settings.probeIntervalHint')} value={form.healthProbeIntervalSec} onChange={set('healthProbeIntervalSec')} />
        <NumberField label={t('settings.healthProbeConcurrency')} hint={t('settings.probeConcurrencyHint')} value={form.healthProbeConcurrency} onChange={set('healthProbeConcurrency')} />
        <NumberField label={t('settings.healthProbeTimeout')} hint={t('settings.probeTimeoutHint')} value={form.healthProbeTimeoutSec} onChange={set('healthProbeTimeoutSec')} />
          </div>
        </Card>

        <div className="flex justify-end">
          <Button type="submit" loading={save.isPending}>
            <Save size={15} /> {t('settings.save')}
          </Button>
        </div>
      </form>

      <ModelFallbackCard />
      <ConfigTransferCard />
      <AuditLogCard />
    </div>
  )
}

function NumberField({ label, hint, value, onChange }: { label: string; hint?: string; value: string; onChange: (value: string) => void }) {
  return (
    <Field label={label} hint={hint}>
      <Input type="number" min="0" step="0.1" value={value} onChange={(e) => onChange(e.target.value)} required />
    </Field>
  )
}

function toForm(s: SystemSettings): FormState {
  return {
    connectTimeoutSec: msToSec(s.connect_timeout_ms),
    firstTokenTimeoutSec: msToSec(s.first_token_timeout_ms),
    overallTimeoutSec: msToSec(s.overall_timeout_ms),
    retryChannels: String(s.retry_channels),
    retryKeysPerChannel: String(s.retry_keys_per_channel),
    retryTotalSec: msToSec(s.retry_total_ms),
    cooldownFailThreshold: String(s.cooldown_fail_threshold),
    cooldownBaseSec: msToSec(s.cooldown_base_ms),
    cooldownMaxSec: msToSec(s.cooldown_max_ms),
    logRetentionDays: String(s.log_retention_days),
    healthProbeIntervalSec: msToSec(s.health_probe_interval_ms),
    healthProbeConcurrency: String(s.health_probe_concurrency),
    healthProbeTimeoutSec: msToSec(s.health_probe_timeout_ms),
  }
}

function toSettings(f: FormState): SystemSettingsValues {
  return {
    connect_timeout_ms: secToMS(f.connectTimeoutSec),
    first_token_timeout_ms: secToMS(f.firstTokenTimeoutSec),
    overall_timeout_ms: secToMS(f.overallTimeoutSec),
    retry_channels: number(f.retryChannels),
    retry_keys_per_channel: number(f.retryKeysPerChannel),
    retry_total_ms: secToMS(f.retryTotalSec),
    cooldown_fail_threshold: number(f.cooldownFailThreshold),
    cooldown_base_ms: secToMS(f.cooldownBaseSec),
    cooldown_max_ms: secToMS(f.cooldownMaxSec),
    log_retention_days: number(f.logRetentionDays),
    health_probe_interval_ms: secToMS(f.healthProbeIntervalSec),
    health_probe_concurrency: number(f.healthProbeConcurrency),
    health_probe_timeout_ms: secToMS(f.healthProbeTimeoutSec),
  }
}

function msToSec(ms: number): string {
  return String(ms / 1000)
}

function secToMS(value: string): number {
  return Math.round(Number(value) * 1000)
}

function number(value: string): number {
  return Math.round(Number(value))
}

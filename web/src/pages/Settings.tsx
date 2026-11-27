import { zodResolver } from '@hookform/resolvers/zod'
import { useForm, type UseFormRegisterReturn } from 'react-hook-form'
import { z } from 'zod'
import { Save } from 'lucide-react'
import { useSystemSettings, useUpdateSystemSettings } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'
import type { SystemSettings, SystemSettingsValues } from '../api/types'
import { Alert, Badge, Button, Card, Disclosure, Field, Input, PageHeader, Spinner } from '../components/ui'
import { notify } from '../components/Toaster'
import ModelFallbackCard from '../components/ModelFallbackCard'
import ConfigTransferCard from '../components/ConfigTransferCard'
import AuditLogCard from '../components/AuditLogCard'
import AlertCard from '../components/AlertCard'
import BackupCard from '../components/BackupCard'
import { fmtTime } from '../lib/format'
import { errorMessage } from '../lib/errors'

// 全部字段按字符串编辑、提交时转数字；前端只拦非正数，整数与上限由后端校验并带 code 返回
const positive = (message: string) => z.string().trim().refine((value) => value !== '' && Number.isFinite(Number(value)) && Number(value) > 0, message)
const schema = z.object({
  connectTimeoutSec: positive(t('settings.positiveNumber')),
  firstTokenTimeoutSec: positive(t('settings.positiveNumber')),
  overallTimeoutSec: positive(t('settings.positiveNumber')),
  retryChannels: positive(t('settings.positiveNumber')),
  retryKeysPerChannel: positive(t('settings.positiveNumber')),
  retryTotalSec: positive(t('settings.positiveNumber')),
  cooldownFailThreshold: positive(t('settings.positiveNumber')),
  cooldownBaseSec: positive(t('settings.positiveNumber')),
  cooldownMaxSec: positive(t('settings.positiveNumber')),
  logRetentionDays: positive(t('settings.positiveNumber')),
  healthProbeIntervalSec: positive(t('settings.positiveNumber')),
  healthProbeConcurrency: positive(t('settings.positiveNumber')),
  healthProbeTimeoutSec: positive(t('settings.positiveNumber')),
})
type FormState = z.infer<typeof schema>

export default function Settings() {
  usePageTitle(t('page.settings.title'))
  const settings = useSystemSettings()
  const save = useUpdateSystemSettings()
  const form = useForm<FormState>({
    resolver: zodResolver(schema),
    // values 随服务端数据重置：保存后重新拉取即同步，不需要手动 useEffect 回填
    values: settings.data?.settings ? toForm(settings.data.settings) : undefined,
  })
  const { register, handleSubmit, formState, setFocus } = form

  if (settings.isPending) {
    return <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>
  }
  if (settings.isError) {
    return (
      <div className="eg-page-stack">
        <PageHeader title={t('page.settings.title')} description={t('page.settings.desc')} />
        <Alert tone="danger">{errorMessage(settings.error)}</Alert>
      </div>
    )
  }

  const current = settings.data?.settings
  const onSubmit = handleSubmit(
    (values) => save.mutate(toSettings(values), { onSuccess: () => notify.success(t('settings.saved')) }),
    (errors) => setFocus(Object.keys(errors)[0] as keyof FormState),
  )

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
        <Disclosure title={t('settings.upstreamTimeout')} defaultOpen={true}>
          <div className="eg-grid eg-grid-auto-wide">        <NumberField label={t('channels.form.connectTimeout')} hint={t('settings.connectTimeoutHint')} error={formState.errors.connectTimeoutSec?.message} registration={register('connectTimeoutSec')} />
        <NumberField label={t('channels.form.firstTokenTimeout')} hint={t('settings.firstTokenTimeoutHint')} error={formState.errors.firstTokenTimeoutSec?.message} registration={register('firstTokenTimeoutSec')} />
        <NumberField label={t('channels.form.overallTimeout')} hint={t('settings.overallTimeoutHint')} error={formState.errors.overallTimeoutSec?.message} registration={register('overallTimeoutSec')} />
          </div>
        </Disclosure>

        <Disclosure title={t('settings.retryCooldown')} defaultOpen={true}>
          <div className="eg-grid eg-grid-auto-wide">        <NumberField label={t('settings.retryChannels')} hint={t('settings.range1to10')} error={formState.errors.retryChannels?.message} registration={register('retryChannels')} />
        <NumberField label={t('settings.retryKeysPerChannel')} hint={t('settings.range1to10')} error={formState.errors.retryKeysPerChannel?.message} registration={register('retryKeysPerChannel')} />
        <NumberField label={t('settings.retryTotalSec')} hint={t('settings.retryTotalHint')} error={formState.errors.retryTotalSec?.message} registration={register('retryTotalSec')} />
        <NumberField label={t('settings.cooldownFailThreshold')} hint={t('settings.range1to100')} error={formState.errors.cooldownFailThreshold?.message} registration={register('cooldownFailThreshold')} />
        <NumberField label={t('settings.cooldownBaseSec')} hint={t('settings.cooldownBaseHint')} error={formState.errors.cooldownBaseSec?.message} registration={register('cooldownBaseSec')} />
        <NumberField label={t('settings.cooldownMaxSec')} hint={t('settings.cooldownMaxHint')} error={formState.errors.cooldownMaxSec?.message} registration={register('cooldownMaxSec')} />
          </div>
        </Disclosure>

        <Disclosure title={t('settings.maintenance')} defaultOpen={false}>
          <div className="eg-grid eg-grid-auto-wide">        <NumberField label={t('settings.logRetentionDays')} hint={t('settings.retentionHint')} error={formState.errors.logRetentionDays?.message} registration={register('logRetentionDays')} />
        <NumberField label={t('settings.healthProbeInterval')} hint={t('settings.probeIntervalHint')} error={formState.errors.healthProbeIntervalSec?.message} registration={register('healthProbeIntervalSec')} />
        <NumberField label={t('settings.healthProbeConcurrency')} hint={t('settings.probeConcurrencyHint')} error={formState.errors.healthProbeConcurrency?.message} registration={register('healthProbeConcurrency')} />
        <NumberField label={t('settings.healthProbeTimeout')} hint={t('settings.probeTimeoutHint')} error={formState.errors.healthProbeTimeoutSec?.message} registration={register('healthProbeTimeoutSec')} />
          </div>
        </Disclosure>

        <div className="flex justify-end">
          <Button type="submit" loading={save.isPending}>
            <Save size={15} /> {t('settings.save')}
          </Button>
        </div>
      </form>

      <ModelFallbackCard />
      <ConfigTransferCard />
      <BackupCard />
      <AlertCard />
      <AuditLogCard />
    </div>
  )
}

function NumberField({ label, hint, error, registration }: { label: string; hint?: string; error?: string; registration: UseFormRegisterReturn }) {
  return (
    <Field label={label} hint={hint} error={error}>
      <Input type="number" min="0" step="0.1" {...registration} />
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

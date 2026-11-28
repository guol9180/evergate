import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { Save } from 'lucide-react'
import { useSystemSettings, useUpdateSystemSettings } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'
import type { SystemSettings, SystemSettingsValues } from '../api/types'
import { Alert, Badge, Button, Card, Disclosure, NumberFormField, PageHeader, SettingRow, Spinner } from '../components/ui'
import { notify } from '../components/Toaster'
import ModelFallbackCard from '../components/ModelFallbackCard'
import ConfigTransferCard from '../components/ConfigTransferCard'
import AuditLogCard from '../components/AuditLogCard'
import AlertCard from '../components/AlertCard'
import BackupCard from '../components/BackupCard'
import { fmtTime } from '../lib/format'
import { errorMessage } from '../lib/errors'

// 数值字段直接以 number 编辑；范围与后端 settings.Validate 对齐，输入框本身也会夹取到范围。
// 秒字段来自毫秒换算，步长取 0.001：React Aria 失焦会按步长精度吸附，0.25s 不能被吸成 0.2。
// 本页数字字段统一不带步进按钮：秒字段的步进无法既精确又有意义，混排会显得不一致（↑/↓ 仍可步进）
const seconds = (min: number, max: number) => z.number({ message: t('settings.positiveNumber') }).min(min, t('settings.rangeInvalid')).max(max, t('settings.rangeInvalid'))
const count = (min: number, max: number) => z.number({ message: t('settings.positiveNumber') }).int(t('settings.intRequired')).min(min, t('settings.rangeInvalid')).max(max, t('settings.rangeInvalid'))
const schema = z.object({
  connectTimeoutSec: seconds(0.1, 300),
  firstTokenTimeoutSec: seconds(0.1, 600),
  overallTimeoutSec: seconds(1, 86_400),
  retryChannels: count(1, 10),
  retryKeysPerChannel: count(1, 10),
  retryTotalSec: seconds(1, 900),
  cooldownFailThreshold: count(1, 100),
  cooldownBaseSec: seconds(1, 3600),
  cooldownMaxSec: seconds(1, 86_400),
  logRetentionDays: count(1, 3650),
  healthProbeIntervalSec: seconds(10, 86_400),
  healthProbeConcurrency: count(1, 32),
  healthProbeTimeoutSec: seconds(1, 120),
  logUnknownTokens: z.boolean(),
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
  const { handleSubmit, setFocus, control } = form

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
          <div className="text-left text-xs text-muted-foreground sm:text-right">
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
              <dd className="mt-1 break-all font-mono text-muted-foreground">{settings.data.paths.config_path || '—'}</dd>
            </div>
            <div className="min-w-0">
              <dt className="text-subtle">{t('settings.dataDir')}</dt>
              <dd className="mt-1 break-all font-mono text-muted-foreground">{settings.data.paths.data_dir || '—'}</dd>
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
          <div className="eg-grid eg-grid-auto-wide">
            <NumberFormField control={control} name="connectTimeoutSec" label={t('channels.form.connectTimeout')} hint={t('settings.connectTimeoutHint')} minValue={0.1} maxValue={300} step={0.001} hideStepper />
            <NumberFormField control={control} name="firstTokenTimeoutSec" label={t('channels.form.firstTokenTimeout')} hint={t('settings.firstTokenTimeoutHint')} minValue={0.1} maxValue={600} step={0.001} hideStepper />
            <NumberFormField control={control} name="overallTimeoutSec" label={t('channels.form.overallTimeout')} hint={t('settings.overallTimeoutHint')} minValue={1} maxValue={86_400} step={0.001} hideStepper />
          </div>
        </Disclosure>

        <Disclosure title={t('settings.retryCooldown')} defaultOpen={true}>
          <div className="eg-grid eg-grid-auto-wide">
            <NumberFormField control={control} name="retryChannels" label={t('settings.retryChannels')} hint={t('settings.range1to10')} minValue={1} maxValue={10} step={1} hideStepper />
            <NumberFormField control={control} name="retryKeysPerChannel" label={t('settings.retryKeysPerChannel')} hint={t('settings.range1to10')} minValue={1} maxValue={10} step={1} hideStepper />
            <NumberFormField control={control} name="retryTotalSec" label={t('settings.retryTotalSec')} hint={t('settings.retryTotalHint')} minValue={1} maxValue={900} step={0.001} hideStepper />
            <NumberFormField control={control} name="cooldownFailThreshold" label={t('settings.cooldownFailThreshold')} hint={t('settings.range1to100')} minValue={1} maxValue={100} step={1} hideStepper />
            <NumberFormField control={control} name="cooldownBaseSec" label={t('settings.cooldownBaseSec')} hint={t('settings.cooldownBaseHint')} minValue={1} maxValue={3600} step={0.001} hideStepper />
            <NumberFormField control={control} name="cooldownMaxSec" label={t('settings.cooldownMaxSec')} hint={t('settings.cooldownMaxHint')} minValue={1} maxValue={86_400} step={0.001} hideStepper />
          </div>
        </Disclosure>

        <Disclosure title={t('settings.maintenance')} defaultOpen={false}>
          <div className="eg-grid eg-grid-auto-wide">
            <NumberFormField control={control} name="logRetentionDays" label={t('settings.logRetentionDays')} hint={t('settings.retentionHint')} minValue={1} maxValue={3650} step={1} hideStepper />
            <NumberFormField control={control} name="healthProbeIntervalSec" label={t('settings.healthProbeInterval')} hint={t('settings.probeIntervalHint')} minValue={10} maxValue={86_400} step={0.001} hideStepper />
            <NumberFormField control={control} name="healthProbeConcurrency" label={t('settings.healthProbeConcurrency')} hint={t('settings.probeConcurrencyHint')} minValue={1} maxValue={32} step={1} hideStepper />
            <NumberFormField control={control} name="healthProbeTimeoutSec" label={t('settings.healthProbeTimeout')} hint={t('settings.probeTimeoutHint')} minValue={1} maxValue={120} step={0.001} hideStepper />
          </div>
          <div className="mt-3">
            <Controller
              name="logUnknownTokens"
              control={control}
              render={({ field }) => (
                <SettingRow
                  title={t('settings.logUnknownTokens')}
                  description={t('settings.logUnknownTokensHint')}
                  label={t('settings.logUnknownTokens')}
                  checked={field.value}
                  onChange={field.onChange}
                />
              )}
            />
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

function toForm(s: SystemSettings): FormState {
  return {
    connectTimeoutSec: msToSec(s.connect_timeout_ms),
    firstTokenTimeoutSec: msToSec(s.first_token_timeout_ms),
    overallTimeoutSec: msToSec(s.overall_timeout_ms),
    retryChannels: s.retry_channels,
    retryKeysPerChannel: s.retry_keys_per_channel,
    retryTotalSec: msToSec(s.retry_total_ms),
    cooldownFailThreshold: s.cooldown_fail_threshold,
    cooldownBaseSec: msToSec(s.cooldown_base_ms),
    cooldownMaxSec: msToSec(s.cooldown_max_ms),
    logRetentionDays: s.log_retention_days,
    healthProbeIntervalSec: msToSec(s.health_probe_interval_ms),
    healthProbeConcurrency: s.health_probe_concurrency,
    healthProbeTimeoutSec: msToSec(s.health_probe_timeout_ms),
    logUnknownTokens: s.log_unknown_tokens,
  }
}

function toSettings(f: FormState): SystemSettingsValues {
  return {
    connect_timeout_ms: secToMS(f.connectTimeoutSec),
    first_token_timeout_ms: secToMS(f.firstTokenTimeoutSec),
    overall_timeout_ms: secToMS(f.overallTimeoutSec),
    retry_channels: f.retryChannels,
    retry_keys_per_channel: f.retryKeysPerChannel,
    retry_total_ms: secToMS(f.retryTotalSec),
    cooldown_fail_threshold: f.cooldownFailThreshold,
    cooldown_base_ms: secToMS(f.cooldownBaseSec),
    cooldown_max_ms: secToMS(f.cooldownMaxSec),
    log_retention_days: f.logRetentionDays,
    health_probe_interval_ms: secToMS(f.healthProbeIntervalSec),
    health_probe_concurrency: f.healthProbeConcurrency,
    health_probe_timeout_ms: secToMS(f.healthProbeTimeoutSec),
    log_unknown_tokens: f.logUnknownTokens,
  }
}

function msToSec(ms: number): number {
  return ms / 1000
}

function secToMS(value: number): number {
  return Math.round(value * 1000)
}

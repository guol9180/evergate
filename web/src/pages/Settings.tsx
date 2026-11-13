import { useEffect, useState, type FormEvent } from 'react'
import { Save } from 'lucide-react'
import { useSystemSettings, useUpdateSystemSettings } from '../api/hooks'
import type { SystemSettings, SystemSettingsValues } from '../api/types'
import { Badge, Button, Card, ErrorText, Field, Input, PageHeader, Spinner } from '../components/ui'
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
      <Card className="p-6">
        <h1 className="text-lg font-bold">系统设置</h1>
        <ErrorText>{settings.isError ? (settings.error as Error).message : '设置不存在'}</ErrorText>
      </Card>
    )
  }

  const set = (key: keyof FormState) => (value: string) => setForm({ ...form, [key]: value })
  const current = settings.data?.settings

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    save.mutate(toSettings(form))
  }

  return (
    <div className="space-y-4">
      <PageHeader
        title="系统设置"
        description="SQLite 是运行时配置的唯一事实源；首次初始化来自 evergate.yml，保存后无需重启。"
        actions={
          <div className="text-left text-xs text-slate-500 sm:text-right">
            <div>来源 <Badge tone="bg-indigo-100 text-indigo-700">{current?.source === 'database' ? '管理台数据库' : 'evergate.yml'}</Badge></div>
            <div className="mt-1 break-words">初始化：{fmtTime(current?.initialized_at)} · 更新：{fmtTime(current?.updated_at)}</div>
          </div>
        }
      />

      <form onSubmit={onSubmit} className="space-y-4">
        <Card className="p-4 sm:p-5">
          <h2 className="mb-4 text-sm font-semibold">上游超时</h2>
          <div className="grid gap-4 md:grid-cols-3">
            <NumberField label="连接超时（秒）" hint="100ms - 5m" value={form.connectTimeoutSec} onChange={set('connectTimeoutSec')} />
            <NumberField label="首 token 超时（秒）" hint="100ms - 10m" value={form.firstTokenTimeoutSec} onChange={set('firstTokenTimeoutSec')} />
            <NumberField label="整体超时（秒）" hint="1s - 24h，不小于前两项" value={form.overallTimeoutSec} onChange={set('overallTimeoutSec')} />
          </div>
        </Card>

        <Card className="p-4 sm:p-5">
          <h2 className="mb-4 text-sm font-semibold">重试与 cooldown</h2>
          <div className="grid gap-4 md:grid-cols-3">
            <NumberField label="最多重试渠道数" hint="1-10" value={form.retryChannels} onChange={set('retryChannels')} />
            <NumberField label="每渠道最多 key 数" hint="1-10" value={form.retryKeysPerChannel} onChange={set('retryKeysPerChannel')} />
            <NumberField label="重试总时长（秒）" hint="1s - 15m" value={form.retryTotalSec} onChange={set('retryTotalSec')} />
            <NumberField label="连续失败阈值" hint="1-100" value={form.cooldownFailThreshold} onChange={set('cooldownFailThreshold')} />
            <NumberField label="cooldown 基础（秒）" hint="1s - 1h" value={form.cooldownBaseSec} onChange={set('cooldownBaseSec')} />
            <NumberField label="cooldown 上限（秒）" hint="不小于基础值，最大 24h" value={form.cooldownMaxSec} onChange={set('cooldownMaxSec')} />
          </div>
        </Card>

        <Card className="p-4 sm:p-5">
          <h2 className="mb-4 text-sm font-semibold">维护任务</h2>
          <div className="grid gap-4 md:grid-cols-4">
            <NumberField label="日志保留天数" hint="1-3650" value={form.logRetentionDays} onChange={set('logRetentionDays')} />
            <NumberField label="健康探测间隔（秒）" hint="10s - 24h" value={form.healthProbeIntervalSec} onChange={set('healthProbeIntervalSec')} />
            <NumberField label="健康探测并发" hint="1-32" value={form.healthProbeConcurrency} onChange={set('healthProbeConcurrency')} />
            <NumberField label="健康探测超时（秒）" hint="1s - 2m" value={form.healthProbeTimeoutSec} onChange={set('healthProbeTimeoutSec')} />
          </div>
        </Card>

        <div className="space-y-3">
          {save.isSuccess && <span className="text-xs text-emerald-600">已保存，新请求立即使用新配置</span>}
          <ErrorText>{save.isError ? (save.error as Error).message : null}</ErrorText>
          <div className="flex justify-end">
            <Button type="submit" disabled={save.isPending}>
              <Save size={15} /> {save.isPending ? '保存中…' : '保存设置'}
            </Button>
          </div>
        </div>
      </form>
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

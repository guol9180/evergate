import { useEffect, useState } from 'react'
import { BellRing, Send } from 'lucide-react'
import { useAlerts, useSaveAlerts, useTestAlert } from '../api/hooks'
import type { AlertSetting } from '../api/types'
import { Alert, Badge, Button, Card, Checkbox, Field, Input, Select, SettingRow, SkeletonRows, Td, Th } from './ui'
import { notify } from './Toaster'
import { fmtTime } from '../lib/format'
import { t, type MessageKey } from '../lib/i18n'
import { errorMessage } from '../lib/errors'

const PRESETS = ['generic', 'wecom', 'feishu', 'telegram', 'bark'] as const
const eventLabel = (event: string) => t(`alerts.event.${event}` as MessageKey)
const resultLabel = (result: string) => t(`alerts.result.${result}` as MessageKey)

/** 事件与 Webhook 告警：配置、测试发送与最近记录。 */
export default function AlertCard() {
  const alerts = useAlerts()
  const save = useSaveAlerts()
  const test = useTestAlert()
  const [draft, setDraft] = useState<AlertSetting | null>(null)

  // 服务端数据视为唯一事实源：首次加载与保存后回填
  useEffect(() => {
    if (alerts.data) setDraft(alerts.data.setting)
  }, [alerts.data])

  if (alerts.isPending) {
    return <Card><SkeletonRows rows={3} /></Card>
  }
  if (alerts.isError || !draft) {
    return <Card><Alert tone="danger">{errorMessage(alerts.error)}</Alert></Card>
  }

  const events = alerts.data?.events ?? []
  const records = alerts.data?.records ?? []
  const toggleEvent = (event: string) => {
    setDraft({
      ...draft,
      events: draft.events.includes(event) ? draft.events.filter((item) => item !== event) : [...draft.events, event],
    })
  }
  const onSave = () => save.mutate(
    { enabled: draft.enabled, preset: draft.preset, url: draft.url, events: draft.events },
    { onSuccess: () => notify.success(t('alerts.saved')) },
  )

  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold"><BellRing size={15} /> {t('alerts.title')}</h2>
        <Badge tone={draft.enabled ? 'success' : 'neutral'}>
          {draft.enabled ? t('alerts.enabled') : t('alerts.disabled')}
        </Badge>
      </div>
      <p className="mb-4 text-xs text-muted">{t('alerts.desc')}</p>

      <div className="space-y-[var(--eg-section-gap)]">
        <SettingRow
          title={t('alerts.enable')}
          description={t('alerts.enableDesc')}
          checked={draft.enabled}
          onChange={(enabled) => setDraft({ ...draft, enabled })}
          label={t('alerts.enable')}
        />
        <div className="eg-grid eg-grid-auto-wide">
          <Field label={t('alerts.preset')} hint={t('alerts.presetHint')}>
            <Select
              value={draft.preset}
              onValueChange={(preset) => setDraft({ ...draft, preset: preset as AlertSetting['preset'] })}
              options={PRESETS.map((preset) => ({ value: preset, label: t(`alerts.preset.${preset}`) }))}
              aria-label={t('alerts.preset')}
            />
          </Field>
          <Field label={t('alerts.url')} hint={t('alerts.urlHint')}>
            <Input
              value={draft.url}
              onChange={(event) => setDraft({ ...draft, url: event.target.value })}
              placeholder="https://example.com/webhook"
              spellCheck={false}
            />
          </Field>
        </div>
        <div>
          <div className="mb-2 text-sm font-medium text-foreground-soft">{t('alerts.events')}</div>
          <div className="eg-grid eg-grid-auto-wide gap-2">
            {events.map((event) => (
              <label key={event} className="flex items-center gap-2 text-xs text-muted">
                <Checkbox checked={draft.events.includes(event)} onCheckedChange={() => toggleEvent(event)} aria-label={eventLabel(event)} />
                {eventLabel(event)}
              </label>
            ))}
          </div>
        </div>
        {save.isError && <Alert tone="danger">{errorMessage(save.error)}</Alert>}
        {test.isError && <Alert tone="danger">{errorMessage(test.error)}</Alert>}
        <div className="flex flex-wrap justify-end gap-2">
          <Button variant="outline" loading={test.isPending} onClick={() => test.mutate(undefined, {
            onSuccess: () => notify.success(t('alerts.testSent')),
          })}>
            <Send size={14} /> {t('alerts.test')}
          </Button>
          <Button loading={save.isPending} onClick={onSave}>{t('common.save')}</Button>
        </div>

        <div>
          <div className="mb-2 text-sm font-medium text-foreground-soft">{t('alerts.recent')}</div>
          {records.length === 0 ? (
            <p className="text-xs text-subtle">{t('alerts.recentEmpty')}</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[520px]">
                <thead className="border-b border-line-soft bg-surface-soft">
                  <tr>
                    <Th>{t('alerts.record.time')}</Th><Th>{t('alerts.record.event')}</Th><Th>{t('alerts.record.result')}</Th><Th>{t('alerts.record.detail')}</Th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line-soft">
                  {records.map((record) => (
                    <tr key={record.id}>
                      <Td className="whitespace-nowrap text-xs text-subtle">{fmtTime(record.created_at)}</Td>
                      <Td className="text-xs">{eventLabel(record.event)}</Td>
                      <Td>
                        <Badge tone={record.result === 'failed' ? 'danger' : record.result === 'test' ? 'brand' : 'success'}>
                          {resultLabel(record.result)}
                        </Badge>
                      </Td>
                      <Td className="max-w-72 truncate text-xs text-muted" title={record.error || record.message}>
                        {record.error || record.message || '—'}
                      </Td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </Card>
  )
}

import { useEffect, useState } from 'react'
import { BellRing, Send } from 'lucide-react'
import { useAlerts, useSaveAlerts, useTestAlert } from '../api/hooks'
import type { AlertRecord, AlertSetting } from '../api/types'
import { Alert, Badge, Button, Card, Checkbox, DataTable, type DataTableColumn, Field, Input, Select, SettingRow, SkeletonRows } from './ui'
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

  // 草稿只在首次加载时从服务端回填；保存成功用响应回填。测试发送会刷新记录但不能清掉未保存的编辑
  useEffect(() => {
    const setting = alerts.data?.setting
    if (setting) setDraft((current) => current ?? setting)
  }, [alerts.data])

  if (alerts.isPending) {
    return <Card><SkeletonRows rows={3} /></Card>
  }
  if (alerts.isError || !draft) {
    return <Card><Alert tone="danger">{errorMessage(alerts.error)}</Alert></Card>
  }

  const events = alerts.data?.events ?? []
  const records = alerts.data?.records ?? []
  const columns: DataTableColumn<AlertRecord>[] = [
    { id: 'time', header: t('alerts.record.time'), className: 'whitespace-nowrap text-xs text-subtle', sortKey: (record) => record.created_at, cell: (record) => fmtTime(record.created_at) },
    { id: 'event', header: t('alerts.record.event'), className: 'text-xs', sortKey: (record) => record.event, cell: (record) => eventLabel(record.event) },
    {
      id: 'result',
      header: t('alerts.record.result'),
      sortKey: (record) => record.result,
      cell: (record) => (
        <span className="flex flex-wrap items-center gap-1">
          <Badge tone={record.result === 'failed' ? 'danger' : record.result === 'test' ? 'brand' : 'success'}>
            {resultLabel(record.result)}
          </Badge>
          {record.draft && <Badge compact tone="neutral">{t('alerts.record.draft')}</Badge>}
        </span>
      ),
    },
    { id: 'detail', header: t('alerts.record.detail'), className: 'max-w-72 text-xs text-muted-foreground', cell: (record) => <span className="block truncate" title={record.error || record.message}>{record.error || record.message || '—'}</span> },
  ]
  const toggleEvent = (event: string) => {
    setDraft({
      ...draft,
      events: draft.events.includes(event) ? draft.events.filter((item) => item !== event) : [...draft.events, event],
    })
  }
  const payload = { enabled: draft.enabled, preset: draft.preset, url: draft.url, events: draft.events }
  const onSave = () => save.mutate(payload, {
    onSuccess: (resp) => {
      setDraft(resp.setting)
      notify.success(t('alerts.saved'))
    },
  })

  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="flex items-center gap-2 text-sm font-semibold"><BellRing size={15} /> {t('alerts.title')}</h2>
        <Badge tone={draft.enabled ? 'success' : 'neutral'}>
          {draft.enabled ? t('alerts.enabled') : t('alerts.disabled')}
        </Badge>
      </div>
      <p className="mb-4 text-xs text-muted-foreground">{t('alerts.desc')}</p>

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
              <label key={event} className="flex items-center gap-2 text-xs text-muted-foreground">
                <Checkbox checked={draft.events.includes(event)} onCheckedChange={() => toggleEvent(event)} aria-label={eventLabel(event)} />
                {eventLabel(event)}
              </label>
            ))}
          </div>
        </div>
        {save.isError && <Alert tone="danger">{errorMessage(save.error)}</Alert>}
        {test.isError && <Alert tone="danger">{errorMessage(test.error)}</Alert>}
        <div className="flex flex-wrap justify-end gap-2">
          {/* 按当前草稿发送：未保存的地址 / 预设也能先测，不会顺带保存 */}
          <Button variant="outline" loading={test.isPending} onClick={() => test.mutate(payload, {
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
            <DataTable columns={columns} rows={records} rowKey={(record) => record.id} minWidth={520} />
          )}
        </div>
      </div>
    </Card>
  )
}

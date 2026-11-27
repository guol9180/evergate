import { useEffect, useId, useRef, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { AlertTriangle, ChevronDown, ChevronRight, Pencil, Plug, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useBatchAddKeys, useChannel, useChannels, useDeleteChannel, useDeleteKey, usePreviewChannelModels, useSaveChannel, useSyncChannelCatalog, useTestChannel, useToggleChannel, useUpdateKey, useUpsertPriceOverride, useUsagePrices } from '../api/hooks'
import { Alert, Badge, Button, Card, ConfirmDialog, ContentSwap, Dialog, Empty, ErrorText, Field, IconButton, Input, PageHeader, Select, SettingRow, SkeletonRows, Spinner, StatusBadge, Switch, Td, Textarea, Th, Tooltip } from '../components/ui'
import { notify } from '../components/Toaster'
import ModelPicker from '../components/ModelPicker'
import type { Channel, ChannelCatalogDiff, ChannelTimeouts, EffectivePrice } from '../api/types'
import { CHANNEL_PRESETS, CUSTOM_CHANNEL_PRESET, findChannelPreset, uniquePresetName } from '../lib/channelPresets'
import { channelStateTone, fmtInt, fmtMs, fmtTime, formatRemainingSeconds, keyImportMessage, stateLabel, statusRank } from '../lib/format'
import { applyModelSelection } from '../lib/modelSelection'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'
import ModelMappingEditor from '../components/ModelMappingEditor'
import PriceTable from '../components/PriceTable'
import TestAllPanel from '../components/TestAllPanel'
import { errorMessage } from '../lib/errors'
import { useUnsavedGuard } from '../lib/useUnsavedGuard'

export default function Channels() {
  usePageTitle(t('page.channels.title'))
  const channels = useChannels()
  const delChannel = useDeleteChannel()
  const delKey = useDeleteKey()
  const [editing, setEditing] = useState<Channel | 'new' | null>(null)
  const [expanded, setExpanded] = useState<number | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Channel | null>(null)
  const [deleteKeyTarget, setDeleteKeyTarget] = useState<{ id: number; channelId: number; label: string } | null>(null)
  const [showTestAll, setShowTestAll] = useState(false)

  const items = [...(channels.data?.items ?? [])].sort((a, b) => statusRank(a.status) - statusRank(b.status) || a.priority - b.priority || a.id - b.id)

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.channels.title')}
        description={t('page.channels.desc')}
        actions={(
          <>
            {/* 与「新建渠道」同尺寸：同一操作行里的按钮高度必须一致 */}
            <Button variant={showTestAll ? 'default' : 'outline'} onClick={() => setShowTestAll((v) => !v)} aria-pressed={showTestAll}>
              <Plug size={14} /> {t('common.testAll')}
            </Button>
            <Button onClick={() => setEditing('new')}><Plus size={15} /> {t('page.channels.create')}</Button>
          </>
        )}
      />
      {showTestAll && <TestAllPanel />}

      <Card>
        <ContentSwap loading={channels.isPending} skeleton={<SkeletonRows rows={4} className="eg-card-fluid" />}>
          {channels.data?.items.length === 0 ? (
            <Empty
              icon={<Plug size={36} />}
              title={t('empty.channels.title')}
              hint={t('empty.channels.hint')}
              action={<Button size="sm" onClick={() => setEditing('new')}><Plus size={14} /> {t('page.channels.create')}</Button>}
            />
          ) : (
            <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] lg:min-w-0">
              <thead className="border-b border-line-soft bg-surface-soft">
                <tr>
          <Th className="w-8" /><Th>{t('table.name')}</Th><Th>{t('table.status')}</Th><Th className="hidden min-[1120px]:table-cell">{t('table.type')}</Th>
          <Th className="hidden min-[1180px]:table-cell">{t('table.priority')}</Th><Th className="hidden min-[1220px]:table-cell">{t('table.weight')}</Th>
          <Th>{t('table.model')}</Th><Th>{t('table.keys')}</Th><Th className="hidden min-[1360px]:table-cell">{t('table.lastProbe')}</Th>
          <Th>{t('table.enabled')}</Th><Th className="w-40"><span className="block pr-2.5 text-right">{t('table.actions')}</span></Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {items.map((c) => (
                  <ChannelRow
                    key={c.id}
                    channel={c}
                    expanded={expanded === c.id}
                    onToggleExpand={() => setExpanded(expanded === c.id ? null : c.id)}
                    onEdit={() => setEditing(c)}
                    onDelete={() => setDeleteTarget(c)}
                  />
                ))}
              </tbody>
            </table>
            </div>
          )}
        </ContentSwap>
      </Card>

      {expanded != null && (
        <KeysPanel channelId={expanded} onDeleteKey={(id, label) => setDeleteKeyTarget({ id, channelId: expanded, label })} />
      )}
      {editing && <ChannelForm channel={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}

      <ConfirmDialog
        open={deleteTarget != null}
        title={t('channels.delete.title')}
        danger
        confirmText={t('channels.delete.title')}
        loading={delChannel.isPending}
        message={tf('channels.delete.message', { name: deleteTarget?.name ?? '' })}
        onConfirm={() => deleteTarget && delChannel.mutate(deleteTarget.id, {
          onSuccess: () => {
            if (expanded === deleteTarget.id) setExpanded(null)
            notify.success(tf('toast.channel.deleted', { name: deleteTarget.name }))
            setDeleteTarget(null)
          }
        })}
        onCancel={() => setDeleteTarget(null)}
      />
      <ConfirmDialog
        open={deleteKeyTarget != null}
        title={t('channels.deleteKey.title')}
        danger
        confirmText={t('channels.deleteKey.confirm')}
        loading={delKey.isPending}
        message={tf('channels.deleteKey.message', { label: deleteKeyTarget?.label ?? '' })}
        onConfirm={() => deleteKeyTarget && delKey.mutate(
          { id: deleteKeyTarget.id, channelId: deleteKeyTarget.channelId },
          {
            onSuccess: () => {
              notify.success(t('toast.key.deleted'))
              setDeleteKeyTarget(null)
            }
          },
        )}
        onCancel={() => setDeleteKeyTarget(null)}
      />
    </div>
  )
}

function ChannelRow({ channel: c, expanded, onToggleExpand, onEdit, onDelete }: { channel: Channel; expanded: boolean; onToggleExpand: () => void; onEdit: () => void; onDelete: () => void }) {
  const toggle = useToggleChannel()
  const test = useTestChannel()
  const syncCatalog = useSyncChannelCatalog()
  const result = test.data && test.variables === c.id ? test.data : null
  const drift = catalogDrift(c.catalog_diff)
  // 从未探测过的新渠道不能显示"正常"：状态列只反映健康度，探测前如实标注"未探测"
  const untested = !c.last_probe_at && c.status === 'active'

  return (
    <>
      <tr className="hover:bg-surface-soft">
        <Td>
          <IconButton label={expanded ? t('channels.collapseKeys') : t('channels.expandKeys')} onClick={onToggleExpand}>
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </IconButton>
        </Td>
        <Td className="max-w-[156px] font-medium"><span className="block truncate" title={c.name}>{c.name}</span></Td>
        <Td>
          <div className="flex flex-col items-start gap-1">
            <StatusBadge tone={untested ? 'neutral' : channelStateTone(c.status)}>
              <span title={c.last_probe_error || undefined}>{untested ? t('common.notProbed') : stateLabel(c.status)}</span>
            </StatusBadge>
            {result && (
              <Badge
                compact
                tone={result.result === 'success' ? 'success' : result.result === 'inconclusive' ? 'neutral' : 'danger'}
                className="cursor-help"
                title={result.error || result.error_type || undefined}
              >
                {result.result === 'success' ? tf('channels.test.success', { ms: result.latency_ms }) : result.result === 'inconclusive' ? t('channels.test.inconclusive') : t('channels.test.failed')}
              </Badge>
            )}
          </div>
        </Td>
        <Td className="hidden min-[1120px]:table-cell">
          <div className="flex flex-col gap-1">
            <span className="inline-block max-w-[96px] truncate text-muted" title={c.type}>{c.type}</span>
            {(c.supports_responses || c.supports_anthropic || c.supports_gemini) && (
              <div className="flex items-center gap-1 whitespace-nowrap">
                {c.supports_responses && <Badge compact tone="info">Responses</Badge>}
                {c.supports_anthropic && <Badge compact tone="brand">Messages</Badge>}
                {c.supports_gemini && <Badge compact tone="success">Gemini</Badge>}
              </div>
            )}
          </div>
        </Td>
        <Td className="hidden min-[1180px]:table-cell">{c.priority}</Td>
        <Td className="hidden min-[1220px]:table-cell">{c.weight}</Td>
        <Td>
          <div>{(c.models || []).length}</div>
          {drift && (
            <Tooltip side="top" content={<CatalogDriftDetail diff={c.catalog_diff} />}>
              <Badge compact tone="warning" className="mt-1 cursor-help" data-catalog-drift="true">
                <AlertTriangle size={11} /> {driftLabel(drift)}
              </Badge>
            </Tooltip>
          )}
        </Td>
        <Td>
          <span className="font-medium">{c.available_key_count}</span>
          <span className="text-subtle"> / {c.key_count}</span>
        </Td>
        <Td className="hidden max-w-44 min-[1360px]:table-cell">
          <div className="text-xs text-muted">
          {c.last_probe_at ? `${fmtTime(c.last_probe_at)} · ${fmtMs(c.last_probe_latency_ms)}` : t('common.notProbed')}
          </div>
          {c.last_probe_error && (
            <div className="truncate text-xs text-subtle" title={c.last_probe_error} data-upstream="true">{c.last_probe_error}</div>
          )}
        </Td>
        <Td>
          <Switch checked={c.enabled} disabled={toggle.isPending && toggle.variables?.id === c.id} onChange={(enabled) => toggle.mutate({ id: c.id, enabled })} label={tf('channels.enableAria', { name: c.name })} />
        </Td>
        <Td>
          <div className="flex justify-end gap-1 whitespace-nowrap">
            <IconButton
              variant="outline"
              label={tf('channels.testAria', { name: c.name })}
              loading={test.isPending && test.variables === c.id}
              onClick={() => test.mutate(c.id)}
            >
              <Plug size={13} />
            </IconButton>
            <IconButton
              variant="outline"
              label={tf('channels.catalog.syncAria', { name: c.name })}
              loading={syncCatalog.isPending && syncCatalog.variables === c.id}
              onClick={() => syncCatalog.mutate(c.id, {
                onSuccess: ({ catalog_diff }) => {
                  const d = catalogDrift(catalog_diff)
                  if (!d) notify.success(t('channels.catalog.upToDate'))
                  else notify.info(tf('channels.catalog.synced', { added: d.added, removed: d.removed }))
                },
              })}
            >
              <RefreshCw size={13} />
            </IconButton>
            <IconButton label={tf('channels.editAria', { name: c.name })} onClick={onEdit}><Pencil size={13} /></IconButton>
            <IconButton label={tf('channels.deleteAria', { name: c.name })} danger onClick={onDelete}><Trash2 size={13} /></IconButton>
          </div>
        </Td>
      </tr>
    </>
  )
}

/** 有差异才提示：新增 / 下线任一非零。 */
function catalogDrift(diff: ChannelCatalogDiff | null | undefined): { added: number; removed: number } | null {
  if (!diff || !diff.checked_at) return null
  const added = (diff.added ?? []).length
  const removed = (diff.removed ?? []).length
  return added + removed > 0 ? { added, removed } : null
}

/** 只显示非零部分：+1、−2 或 +2 / −1。 */
function driftLabel(drift: { added: number; removed: number }): string {
  const parts: string[] = []
  if (drift.added > 0) parts.push(`+${drift.added}`)
  if (drift.removed > 0) parts.push(`−${drift.removed}`)
  return parts.join(' / ')
}

function CatalogDriftDetail({ diff }: { diff: ChannelCatalogDiff | null | undefined }) {
  if (!diff) return null
  const added = diff.added ?? []
  const removed = diff.removed ?? []
  return (
    <div className="max-w-xs space-y-1.5 text-xs">
      <div className="font-semibold text-foreground">{t('channels.catalog.driftTitle')}</div>
      {added.length > 0 && (
        <div>
          <div className="text-subtle">{tf('channels.catalog.added', { n: added.length })}</div>
          <div className="break-all font-mono text-[11px] text-foreground-soft">{added.join(', ')}</div>
        </div>
      )}
      {removed.length > 0 && (
        <div>
          <div className="text-subtle">{tf('channels.catalog.removed', { n: removed.length })}</div>
          <div className="break-all font-mono text-[11px] text-foreground-soft">{removed.join(', ')}</div>
        </div>
      )}
      <div className="pt-1 text-subtle">{tf('channels.catalog.checkedAt', { time: fmtTime(diff.checked_at), count: fmtInt(diff.upstream_count ?? 0) })}</div>
      <div className="text-subtle">{t('channels.catalog.hint')}</div>
    </div>
  )
}

// 渠道详情：key 管理（gpt-load GroupCredentialsTab 模式：展开行 + 批量导入 + 状态徽标）
function KeysPanel({ channelId, onDeleteKey }: { channelId: number; onDeleteKey: (id: number, label: string) => void }) {
  const detail = useChannel(channelId)
  const addKeys = useBatchAddKeys()
  const updateKey = useUpdateKey()
  const [keysText, setKeysText] = useState('')

  if (detail.isPending) return <Card className="eg-card-fluid"><SkeletonRows rows={3} /></Card>
  if (detail.isError) return <Card className="p-4"><ErrorText>{errorMessage(detail.error)}</ErrorText></Card>

  return (
    <Card className="eg-expand-in">
      <h3 className="mb-3 text-sm font-semibold">{tf('channels.keys.title', { name: detail.data.channel.name })}</h3>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[700px] lg:min-w-0">
          <thead className="border-b border-line-soft bg-surface-soft">
            <tr>
              <Th>#</Th><Th>{t('table.status')}</Th><Th>{t('table.weight')}</Th><Th>{t('table.consecutiveFails')}</Th>
              <Th className="hidden min-[1180px]:table-cell">{t('table.backoffLevel')}</Th><Th>{t('table.cooldownRemaining')}</Th>
                  <Th className="hidden min-[1220px]:table-cell">{t('table.successFail')}</Th><Th className="hidden min-[1260px]:table-cell">{t('channels.keys.rpm')}</Th><Th className="hidden min-[1360px]:table-cell">{t('table.lastError')}</Th>
              <Th>{t('table.enabled')}</Th><Th className="w-16"><span className="block pr-2.5 text-right">{t('table.actions')}</span></Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line-soft">
            {detail.data.keys.map((k, i) => {
              // 冷却到期后状态要等下次调度才切回，这里按实时剩余时间判断：已到期就按"正常"展示
              const cooling = k.state === 'cooldown' && k.cooldown_remaining_sec > 0
              const shownState = k.state === 'cooldown' && !cooling ? 'active' : k.state
              return (
                <tr key={k.id}>
                  <Td className="text-subtle">{i + 1}</Td>
                  <Td>
                    <StatusBadge tone={channelStateTone(shownState)}>{stateLabel(shownState)}</StatusBadge>
                  </Td>
                  <Td>{k.weight}</Td>
                  <Td className={k.consecutive_fails > 0 ? 'text-warning-text font-medium' : ''}>{k.consecutive_fails}</Td>
                  <Td className="hidden min-[1180px]:table-cell">L{k.level}</Td>
                  <Td>{cooling ? <span className="text-warning-text">{formatRemainingSeconds(k.cooldown_remaining_sec)}</span> : '—'}</Td>
                  <Td className="hidden text-xs text-muted min-[1220px]:table-cell">{k.success_count}/{k.fail_count}</Td>
                  <Td className="hidden min-[1260px]:table-cell">
                    <KeyRpmInput
                      value={k.rpm_limit}
                      disabled={updateKey.isPending && updateKey.variables?.id === k.id}
                      label={tf('channels.keys.rpmAria', { n: i + 1 })}
                      onCommit={(rpm_limit) => updateKey.mutate({ id: k.id, rpm_limit })}
                    />
                  </Td>
                  <Td className="hidden max-w-48 truncate text-xs text-subtle min-[1360px]:table-cell" title={k.last_error}>{k.last_error || '—'}</Td>
                  <Td>
                  <Switch checked={k.enabled} disabled={updateKey.isPending && updateKey.variables?.id === k.id} onChange={(enabled) => updateKey.mutate({ id: k.id, enabled })} label={tf('channels.enableKeyAria', { n: i + 1 })} />
                  </Td>
                  <Td>
                    <div className="flex justify-end">
                      <IconButton label={tf('channels.deleteKeyAria', { n: i + 1 })} danger onClick={() => onDeleteKey(k.id, tf('channels.deleteKeyLabel', { n: i + 1 }))}>
                        <Trash2 size={13} />
                      </IconButton>
                    </div>
                  </Td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <div className="mt-4 rounded-lg border border-dashed border-line-strong p-3">
        <div className="mb-2 text-xs font-medium text-muted">{t('channels.import.title')}</div>
        <Textarea rows={3} value={keysText} onChange={(e) => setKeysText(e.target.value)} placeholder={'sk-xxxxxxx\nsk-yyyyyyy'} />
        <div className="mt-2 flex items-center gap-3">
          <Button size="sm" loading={addKeys.isPending} disabled={!keysText.trim()} onClick={() => addKeys.mutate(
            { channelId, keys: keysText },
            {
              onSuccess: (resp) => {
                const result = keyImportMessage(resp)
                notify[result.tone](result.text)
                setKeysText('')
              }
            },
          )}>
            {t('channels.import.button')}
          </Button>
        </div>
      </div>
    </Card>
  )
}

// 新建/编辑渠道弹窗：预设模式只开放模型列表 / API 地址 / API 密钥三项，其余由预设决定；
// 名称、上游协议、优先级 / 权重 / 超时、模型映射只在「切换为自定义」后出现（T91）。
/** key 的每分钟请求上限：失焦或回车提交，0 表示不限。 */
function KeyRpmInput({ value, disabled, label, onCommit }: { value: number; disabled: boolean; label: string; onCommit: (value: number) => void }) {
  const [draft, setDraft] = useState(String(value))
  // 回车提交后失焦会立刻再触发一次 commit：同一值在极短时间内只提交一次，失败后仍可重试
  const lastSentAt = useRef(0)
  useEffect(() => setDraft(String(value)), [value])
  const commit = () => {
    const next = Number(draft)
    if (!Number.isInteger(next) || next < 0 || next === value) {
      setDraft(String(value))
      return
    }
    if (Date.now() - lastSentAt.current < 400) return
    lastSentAt.current = Date.now()
    onCommit(next)
  }
  return (
    <Input
      type="number"
      min={0}
      className="h-8 w-20 text-xs"
      value={draft}
      disabled={disabled}
      aria-label={label}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (event.key === 'Enter') {
          event.preventDefault()
          commit()
        }
      }}
    />
  )
}

function ChannelForm({ channel, onClose }: { channel: Channel | null; onClose: () => void }) {
  const save = useSaveChannel()
  const preview = usePreviewChannelModels()
  const channels = useChannels()
  const channelDetail = useChannel(channel?.id ?? null)
  const usagePrices = useUsagePrices()
  const upsertPrice = useUpsertPriceOverride()
  const addKeys = useBatchAddKeys({ silentError: true })
  const formID = 'channel-form'
  const modelsId = useId()
  const [showModelPicker, setShowModelPicker] = useState(false)
  // 渠道已创建但内联导入 key 失败时记下 id：保持弹窗可重试，避免再次保存变成重名创建。
  const [createdChannelID, setCreatedChannelID] = useState<number | null>(null)

  // schema 即校验规则与表单值类型；文本字段保留字符串，提交时再转数字 / 拆行
  const optionalInt = (min: number, max: number) =>
    z.string().trim().refine((value) => value === '' || (/^\d+$/.test(value) && Number(value) >= min && Number(value) <= max), t('channels.form.timeoutInt'))
  const schema = z
    .object({
      presetId: z.string(),
      name: z.string(),
      baseUrl: z.string().trim().refine((value) => {
        if (!/^https?:\/\//.test(value)) return false
        try { new URL(value); return true } catch { return false }
      }, t('channels.form.urlInvalid')),
      keysText: z.string(),
      supportsResponses: z.boolean(),
      supportsAnthropic: z.boolean(),
      supportsGemini: z.boolean(),
      priority: z.string().refine((value) => value === '' || (/^-?\d+$/.test(value) && Number(value) >= -1000 && Number(value) <= 1000), t('channels.form.intInvalid')),
      weight: z.string().refine((value) => value === '' || (/^\d+$/.test(value) && Number(value) >= 1 && Number(value) <= 10000), t('channels.form.intInvalid')),
      connectSec: optionalInt(1, 300),
      firstTokenSec: optionalInt(1, 3600),
      overallSec: optionalInt(1, 86400),
      models: z.string(),
      mapping: z.record(z.string(), z.string()),
    })
    // 名称只在自定义接入时必填：预设模式新建由预设默认名自动生成（重名追加 -2 / -3）
    .superRefine((values, ctx) => {
      if (findChannelPreset(values.presetId) == null && !values.name.trim()) {
        ctx.addIssue({ code: 'custom', path: ['name'], message: t('channels.form.nameRequired') })
      }
    })
  type FormValues = z.infer<typeof schema>
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      // 新建默认落在第一个供应商预设（预设自带模型与价格，是推荐路径）；自定义接入保留为显式选项
      presetId: channel?.preset_id || (channel ? CUSTOM_CHANNEL_PRESET : CHANNEL_PRESETS[0].id),
      name: channel?.name ?? '',
      baseUrl: channel?.base_url ?? '',
      keysText: '',
      supportsResponses: channel?.supports_responses ?? false,
      supportsAnthropic: channel?.supports_anthropic ?? false,
      supportsGemini: channel?.supports_gemini ?? false,
      priority: String(channel?.priority ?? 0),
      weight: String(channel?.weight ?? 1),
      connectSec: channel?.timeout_config?.connect_sec?.toString() ?? '',
      firstTokenSec: channel?.timeout_config?.first_token_sec?.toString() ?? '',
      overallSec: channel?.timeout_config?.overall_sec?.toString() ?? '',
      models: (channel?.models || []).join('\n'),
      mapping: channel?.model_mapping ?? {},
    },
  })
  const { register, handleSubmit, formState, watch, setValue, setFocus } = form
  const presetId = watch('presetId')
  const supportsResponses = watch('supportsResponses')
  const supportsAnthropic = watch('supportsAnthropic')
  const supportsGemini = watch('supportsGemini')
  const models = watch('models')
  const keysText = watch('keysText')
  const preset = findChannelPreset(presetId)
  const presetMode = preset != null
  // 有未保存更改时关闭需确认（Esc / 遮罩 / 取消按钮都走这里）
  const guard = useUnsavedGuard(formState.isDirty)
  const close = guard.guard(onClose)

  // 预设渠道的价格由内置目录兜底：编辑时用服务端算好的生效价格（覆盖 → 全局默认 → 目录），
  // 新建时还没有渠道记录，只能按目录预览当前填写的模型。
  const typedModels = splitLines(models).map((model) => watch('mapping')[model] || model)
  const catalogPrices: EffectivePrice[] = presetMode && usagePrices.data
    ? Array.from(new Set(typedModels)).flatMap((model) => {
      const entry = usagePrices.data?.catalog?.entries.find((item) => item.preset_id === presetId && item.model === model)
      return entry ? [{
        model,
        input_micros_per_million: entry.input_micros_per_million,
        output_micros_per_million: entry.output_micros_per_million,
        cache_read_micros_per_million: entry.cache_read_micros_per_million ?? 0,
        cache_write_micros_per_million: entry.cache_write_micros_per_million ?? 0,
        source: 'catalog' as const,
        source_url: entry.source_url,
      }] : []
    })
    : []
  // 编辑时服务端价格只覆盖已保存的模型；刚在文本框里新增的模型直接按目录预览
  const serverPrices = channelDetail.data?.prices ?? []
  const knownModels = new Set(serverPrices.map((item) => item.model))
  const priceRows = channel
    ? [...serverPrices, ...catalogPrices.filter((item) => !knownModels.has(item.model))]
    : catalogPrices

  const applyPreset = (id: string) => {
    setValue('presetId', id, { shouldDirty: true })
    const next = findChannelPreset(id)
    if (!next) return
    setValue('baseUrl', next.baseUrl, { shouldDirty: true })
    setValue('supportsResponses', next.supportsResponses, { shouldDirty: true })
    setValue('supportsAnthropic', next.supportsAnthropic, { shouldDirty: true })
    setValue('supportsGemini', next.supportsGemini ?? false, { shouldDirty: true })
    setValue('name', next.defaultName, { shouldDirty: true })
  }
  // 切换为自定义：保留预设已填的地址与协议作为起点，解锁名称 / 协议 / 调度字段；保存时清空 preset_id
  const switchToCustom = () => setValue('presetId', CUSTOM_CHANNEL_PRESET, { shouldDirty: true })

  // 数据层 type 只是派生值：Gemini / Anthropic 原生优先，其余归 openai-compatible；预设渠道沿用预设的 type
  const derivedType = supportsGemini ? 'gemini' : supportsAnthropic ? 'anthropic' : 'openai-compatible'
  const type = presetMode ? preset.type : derivedType

  const importKeys = (channelID: number) => {
    const keys = keysText.trim()
    if (!keys) {
      onClose()
      return
    }
    addKeys.mutate({ channelId: channelID, keys }, {
      onSuccess: (resp) => {
        const result = keyImportMessage(resp)
        notify[result.tone](result.text)
        onClose()
      },
    })
  }

  const onSubmit = handleSubmit(
    (values) => {
      const timeouts: ChannelTimeouts = {}
      if (values.connectSec.trim()) timeouts.connect_sec = Number(values.connectSec)
      if (values.firstTokenSec.trim()) timeouts.first_token_sec = Number(values.firstTokenSec)
      if (values.overallSec.trim()) timeouts.overall_sec = Number(values.overallSec)
      const currentPreset = findChannelPreset(values.presetId)
      const finalName = currentPreset && !channel
        ? uniquePresetName(currentPreset.defaultName, (channels.data?.items ?? []).map((item) => item.name))
        : values.name
      save.mutate(
        {
          id: channel?.id,
          name: finalName,
          type,
          preset_id: currentPreset ? values.presetId : '',
          base_url: values.baseUrl.trim(),
          priority: Number(values.priority || '0') || 0,
          weight: Number(values.weight || '1') || 1,
          models: splitLines(values.models),
          model_mapping: Object.keys(values.mapping).length ? values.mapping : null,
          supports_responses: values.supportsResponses,
          supports_anthropic: values.supportsAnthropic,
          supports_gemini: values.supportsGemini,
          ...(Object.keys(timeouts).length ? { timeout_config: timeouts } : {}),
        },
        {
          onSuccess: (saved) => {
            notify.success(t('toast.channel.saved'))
            if (!channel && values.keysText.trim()) {
              setCreatedChannelID(saved.id)
              importKeys(saved.id)
              return
            }
            onClose()
          },
        },
      )
    },
    // 校验失败：焦点落到第一个出错字段
    (errors) => setFocus(Object.keys(errors)[0] as keyof FormValues),
  )

  return (
    <Dialog
      open
      onClose={close}
      title={channel ? t('channels.edit.title') : t('page.channels.create')}
      wide
      footer={
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={close}>{t('common.cancel')}</Button>
          <Button type="submit" form={formID} loading={save.isPending} disabled={createdChannelID != null}>{t('common.save')}</Button>
        </div>
      }
    >
      <form id={formID} onSubmit={onSubmit} className="space-y-[var(--eg-control-gap)]" noValidate>
        {/* 提交错误放弹窗正文顶部：打开即见，不藏在「保存」按钮上方 */}
        {save.isError
          ? <Alert tone="danger">{errorMessage(save.error)}</Alert>
          : addKeys.isError
            ? (
              <Alert
                tone="danger"
                action={createdChannelID != null
                  ? <Button size="sm" variant="outline" loading={addKeys.isPending} onClick={() => importKeys(createdChannelID)}>{t('channels.form.retryImport')}</Button>
                  : undefined}
              >
                {tf('channels.form.keyImportFailed', { message: errorMessage(addKeys.error) })}
              </Alert>
            )
            : null}
        <div className="space-y-2">
          <Field label={t('channels.form.preset')} hint={t('channels.form.presetHint')}>
            <Select
              value={presetId}
              onValueChange={applyPreset}
              options={[
                { value: CUSTOM_CHANNEL_PRESET, label: t('preset.custom.label') },
                ...CHANNEL_PRESETS.map((p) => ({ value: p.id, label: t(p.labelKey) })),
              ]}
            />
          </Field>
          {preset && (
            <Alert tone={preset.warningKey ? 'warning' : 'neutral'}>
              <div>{t(preset.descKey)}</div>
              <div className="mt-1 font-mono text-[11px] opacity-80">{preset.baseUrl}</div>
              {preset.warningKey && (
                <div className="mt-1.5 font-medium">{t(preset.warningKey)}</div>
              )}
            </Alert>
          )}
        </div>
        {presetMode && (
          <Alert
            tone="neutral"
            action={<Button type="button" size="sm" variant="ghost" onClick={switchToCustom}>{t('channels.form.switchToCustom')}</Button>}
          >
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-muted">{t('channels.form.protocolReadonly')}</span>
              <Badge tone="brand">Chat Completions</Badge>
              {supportsResponses && <Badge tone="info">Responses</Badge>}
              {supportsAnthropic && <Badge tone="brand">Messages</Badge>}
              {supportsGemini && <Badge tone="success">Gemini</Badge>}
            </div>
          </Alert>
        )}
        {!presetMode && (
          <Field label={t('channels.form.name')} error={formState.errors.name?.message}><Input {...register('name')} placeholder="openai-main" /></Field>
        )}
        <Field label="Base URL" hint={tf('channels.form.baseUrlHint', { paths: [supportsAnthropic ? '/v1/messages' : '', supportsResponses ? '/responses' : '', supportsGemini ? '/v1beta/models/{model}' : '', '/chat/completions'].filter(Boolean).join(', ') })} error={formState.errors.baseUrl?.message}>
          <Input {...register('baseUrl')} placeholder="https://api.openai.com/v1" />
        </Field>
        {!channel && (
          <Field label={t('channels.form.keys')} hint={t('channels.form.keysHint')}>
            <Textarea rows={3} {...register('keysText')} placeholder={'sk-...\nsk-...'} autoComplete="off" className="font-mono text-xs" />
          </Field>
        )}
        {!presetMode && (
          <div className="space-y-2">
            <div>
              <div className="text-sm font-medium text-foreground-soft">{t('channels.form.protocol')}</div>
              <div className="text-xs text-subtle">{t('channels.form.protocolHint')}</div>
            </div>
            <SettingRow
              title={t('channels.proto.cc.title')}
              description={t('channels.proto.cc.desc')}
              checked
              disabled
              onChange={() => {}}
              label={t('channels.proto.cc.title')}
            />
            <SettingRow
              title={t('channels.proto.responses.title')}
              description={t('channels.proto.responses.desc')}
              checked={supportsResponses}
              onChange={(enabled) => setValue('supportsResponses', enabled, { shouldDirty: true })}
              label={t('channels.proto.responses.title')}
            />
            <SettingRow
              title={t('channels.proto.anthropic.title')}
              description={t('channels.proto.anthropic.desc')}
              checked={supportsAnthropic}
              onChange={(enabled) => setValue('supportsAnthropic', enabled, { shouldDirty: true })}
              label={t('channels.proto.anthropic.title')}
            />
            <SettingRow
              title={t('channels.proto.gemini.title')}
              description={t('channels.proto.gemini.desc')}
              checked={supportsGemini}
              onChange={(enabled) => setValue('supportsGemini', enabled, { shouldDirty: true })}
              label={t('channels.proto.gemini.title')}
            />
          </div>
        )}
        {!presetMode && (
          <div className="eg-grid eg-grid-auto-wide">
            <Field label={t('channels.form.priority')} hint={t('channels.form.priorityHint')} error={formState.errors.priority?.message}><Input type="number" {...register('priority')} /></Field>
            <Field label={t('channels.form.weight')} hint={t('channels.form.weightHint')} error={formState.errors.weight?.message}><Input type="number" {...register('weight')} /></Field>
          </div>
        )}
        {!presetMode && (
          <div className="eg-grid eg-grid-auto-wide">
            <Field label={t('channels.form.connectTimeout')} hint={t('channels.form.connectTimeoutHint')} error={formState.errors.connectSec?.message}>
              <Input type="number" min={1} max={300} {...register('connectSec')} placeholder={t('common.blankPlaceholder')} />
            </Field>
            <Field label={t('channels.form.firstTokenTimeout')} hint={t('channels.form.firstTokenTimeoutHint')} error={formState.errors.firstTokenSec?.message}>
              <Input type="number" min={1} max={3600} {...register('firstTokenSec')} placeholder={t('common.blankPlaceholder')} />
            </Field>
            <Field label={t('channels.form.overallTimeout')} hint={t('channels.form.overallTimeoutHint')} error={formState.errors.overallSec?.message}>
              <Input type="number" min={1} max={86400} {...register('overallSec')} placeholder={t('common.blankPlaceholder')} />
            </Field>
          </div>
        )}
        <div className="space-y-2">
          <div className="flex flex-wrap items-end justify-between gap-2">
            <label htmlFor={modelsId} className="text-sm font-medium text-foreground-soft">{t('channels.form.models')}</label>
            <Button
              type="button"
              size="sm"
              variant="outline"
              loading={preview.isPending}
              disabled={!channel || channel.key_count === 0}
              title={!channel ? t('channels.form.fetchUnavailable') : channel.key_count === 0 ? t('channels.form.fetchNeedKey') : undefined}
              onClick={() => {
                if (!channel) return
                setShowModelPicker(false)
                preview.reset()
                preview.mutate(channel.id, { onSuccess: () => setShowModelPicker(true) })
              }}
            >
              <RefreshCw size={13} />
              {t('channels.form.fetch')}
            </Button>
          </div>
          <Textarea id={modelsId} rows={4} {...register('models')} placeholder={preset?.modelsPlaceholder ?? 'gpt-4o\ngpt-4o-mini'} />
          <div className="text-xs text-subtle">
            {!channel
              ? t('channels.form.modelsHintNew')
              : channel.key_count === 0
                ? t('channels.form.modelsHintNoKey')
                : t('channels.form.modelsHintPicker')}
          </div>
          {showModelPicker && preview.data && (
            <ModelPicker
              title={tf('channels.form.pickerTitle', { name: channel?.name ?? '' })}
              options={preview.data.items.map((item) => ({
                publicModel: item.public_model,
                upstreamModel: item.upstream_model,
                price: item.price_micros,
              }))}
              initialModels={splitLines(models)}
              protocol={preview.data.protocol}
              truncated={preview.data.truncated}
              loading={preview.isPending}
              onCancel={() => {
                setShowModelPicker(false)
                preview.reset()
              }}
              onApply={(selected, mode, withPrices) => {
                const next = applyModelSelection(splitLines(models), selected, mode)
                setValue('models', next.join('\n'), { shouldDirty: true })
                // 上游目录声明了价格时勾选"同时写入价格覆盖"，就把价格固化成渠道覆盖，之后目录变动不影响计费
                if (withPrices && channel) {
                  const picked = new Set(selected)
                  const written = new Set<string>()
                  for (const item of preview.data?.items ?? []) {
                    if (!picked.has(item.public_model) || !item.price_micros || written.has(item.upstream_model)) continue
                    written.add(item.upstream_model)
                    upsertPrice.mutate({
                      channel_id: channel.id,
                      model: item.upstream_model,
                      input_micros_per_million: item.price_micros.input_micros_per_million,
                      output_micros_per_million: item.price_micros.output_micros_per_million,
                      cache_read_micros_per_million: item.price_micros.cache_read_micros_per_million ?? 0,
                      cache_write_micros_per_million: item.price_micros.cache_write_micros_per_million ?? 0,
                    })
                  }
                }
                setShowModelPicker(false)
                preview.reset()
              }}
            />
          )}
          {preview.isError && !preview.data && (
            <ErrorText>{tf('channels.form.fetchFailed', { message: errorMessage(preview.error) })}</ErrorText>
          )}
        </div>
        {presetMode && (
          <div className="space-y-2">
            <div>
              <div className="text-sm font-medium text-foreground-soft">{t('channels.form.prices')}</div>
              <div className="text-xs text-subtle">{t('channels.form.pricesHint')}</div>
            </div>
            {channel && channelDetail.isPending ? (
              <div className="flex justify-center py-3"><Spinner /></div>
            ) : priceRows.length > 0 ? (
              <PriceTable prices={priceRows} />
            ) : (
              <div className="rounded-lg border border-dashed border-line-strong px-3 py-2 text-xs text-subtle">{t('channels.form.pricesEmpty')}</div>
            )}
          </div>
        )}
        {!presetMode && <ModelMappingEditor value={watch('mapping')} onChange={(next) => setValue('mapping', next, { shouldDirty: true })} />}
      </form>
      {guard.dialog}
    </Dialog>
  )
}
function splitLines(text: string): string[] {
  return text.split(/\r?\n/).map((item) => item.trim()).filter(Boolean)
}

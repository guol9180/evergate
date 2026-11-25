import { type FormEvent, useId, useState } from 'react'
import { ChevronDown, ChevronRight, Pencil, Plug, Plus, RefreshCw, Trash2, TriangleAlert } from 'lucide-react'
import { useBatchAddKeys, useChannel, useChannels, useDeleteChannel, useDeleteKey, usePreviewChannelModels, useSaveChannel, useTestChannel, useToggleChannel, useUpdateKey } from '../api/hooks'
import { Badge, Button, Card, ConfirmDialog, Dialog, Empty, ErrorText, Field, Input, PageHeader, PageSkeleton, Select, SettingRow, SkeletonRows, Spinner, StatusBadge, Switch, Td, Textarea, Th } from '../components/ui'
import ModelPicker from '../components/ModelPicker'
import type { Channel } from '../api/types'
import { CHANNEL_PRESETS, CUSTOM_CHANNEL_PRESET, findChannelPreset } from '../lib/channelPresets'
import { channelStateTone, fmtMs, fmtTime, formatRemainingSeconds, stateLabel, statusRank } from '../lib/format'
import { applyModelSelection } from '../lib/modelSelection'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import ModelMappingEditor from '../components/ModelMappingEditor'
import TestAllPanel from '../components/TestAllPanel'

const CHANNEL_TYPES = ['openai', 'openai-compatible', 'deepseek', 'vllm', 'anthropic', 'anthropic-compatible', 'gemini', 'gemini-compat']

// 类型语义说明：8 个取值与预设部分重叠，对新手是黑盒，这里说明每个取值实际影响什么。
const CHANNEL_TYPE_HELP: Record<string, string> = {
  openai: 'OpenAI 官方端点；Chat 与 Responses 走官方路径',
  'openai-compatible': '任何 OpenAI 兼容服务（vLLM、中转、第三方）；最通用',
  deepseek: 'DeepSeek 官方；OpenAI 兼容，base_url 可省略 /v1',
  vllm: '本地 vLLM / OpenAI 兼容推理服务',
  anthropic: 'Anthropic 官方；需开启「原生支持 Anthropic Messages」',
  'anthropic-compatible': 'Anthropic 兼容中转；需开启「原生支持 Anthropic Messages」',
  gemini: 'Google Gemini 官方；需开启「原生支持 Gemini API」',
  'gemini-compat': 'Gemini 兼容中转；需开启「原生支持 Gemini API」',
}

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

  if (channels.isPending) {
    return (
      <div className="eg-page-stack">
        <PageHeader title={t('page.channels.title')} description={t('page.channels.desc')} />
        <Card><SkeletonRows rows={4} className="eg-card-fluid" /></Card>
      </div>
    )
  }
  const items = [...(channels.data?.items ?? [])].sort((a, b) => statusRank(a.status) - statusRank(b.status) || a.priority - b.priority || a.id - b.id)

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.channels.title')}
        description={t('page.channels.desc')}
        actions={(
          <>
            <Button size="sm" variant={showTestAll ? 'default' : 'outline'} onClick={() => setShowTestAll((v) => !v)} aria-pressed={showTestAll}>
              <Plug size={14} /> {t('common.testAll')}
            </Button>
            <Button onClick={() => setEditing('new')}><Plus size={15} /> {t('page.channels.create')}</Button>
          </>
        )}
      />
      {showTestAll && <TestAllPanel />}

      <Card>
        {channels.data?.items.length === 0 ? (
          <Empty icon={<Plug size={36} />} title={t('empty.channels.title')} hint={t('empty.channels.hint')} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] lg:min-w-0">
              <thead className="border-b border-line-soft bg-surface-soft">
                <tr>
                  <Th className="w-8" /><Th>名称</Th><Th>状态</Th><Th className="hidden min-[1120px]:table-cell">类型</Th>
                  <Th className="hidden min-[1180px]:table-cell">优先级</Th><Th className="hidden min-[1220px]:table-cell">权重</Th>
                  <Th>模型</Th><Th>key / 可用</Th><Th className="hidden min-[1280px]:table-cell">最近探测</Th>
                  <Th>启用</Th><Th>测试</Th><Th className="w-24"><span className="block pr-2.5 text-right">操作</span></Th>
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
      </Card>

      {expanded != null && (
        <KeysPanel channelId={expanded} onDeleteKey={(id, label) => setDeleteKeyTarget({ id, channelId: expanded, label })} />
      )}
      {editing && <ChannelForm channel={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}

      <ConfirmDialog
        open={deleteTarget != null}
        title="删除渠道"
        danger
        confirmText="删除渠道"
        loading={delChannel.isPending}
        message={<>将删除渠道「{deleteTarget?.name}」及其全部 key，正在使用该上游的请求会立即失败。此操作不可撤销。</>}
        onConfirm={() => deleteTarget && delChannel.mutate(deleteTarget.id, {
          onSuccess: () => {
            if (expanded === deleteTarget.id) setExpanded(null)
            setDeleteTarget(null)
          },
        })}
        onCancel={() => setDeleteTarget(null)}
      />
      <ConfirmDialog
        open={deleteKeyTarget != null}
        title="删除上游 key"
        danger
        confirmText="删除 key"
        loading={delKey.isPending}
        message={<>将删除{deleteKeyTarget?.label}；删除后立即退出调度，无法恢复。</>}
        onConfirm={() => deleteKeyTarget && delKey.mutate(
          { id: deleteKeyTarget.id, channelId: deleteKeyTarget.channelId },
          { onSuccess: () => setDeleteKeyTarget(null) },
        )}
        onCancel={() => setDeleteKeyTarget(null)}
      />
    </div>
  )
}

function ChannelRow({ channel: c, expanded, onToggleExpand, onEdit, onDelete }: { channel: Channel; expanded: boolean; onToggleExpand: () => void; onEdit: () => void; onDelete: () => void }) {
  const toggle = useToggleChannel()
  const test = useTestChannel()
  const result = test.data && test.variables === c.id ? test.data : null
  // 从未探测过的新渠道不能显示"正常"：状态列只反映健康度，探测前如实标注"未探测"
  const untested = !c.last_probe_at && c.status === 'active'

  return (
    <>
      <tr className="hover:bg-surface-soft">
        <Td>
          <button onClick={onToggleExpand} aria-label={expanded ? '收起 key 列表' : '展开 key 列表'} className="rounded p-1 text-subtle hover:bg-surface-strong">
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        </Td>
        <Td className="max-w-[170px] font-medium"><span className="block truncate" title={c.name}>{c.name}</span></Td>
        <Td>
          <StatusBadge tone={untested ? 'neutral' : channelStateTone(c.status)}>
            <span title={c.last_probe_error || undefined}>{untested ? '未探测' : stateLabel[c.status] || c.status}</span>
          </StatusBadge>
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
        <Td>{(c.models || []).length}</Td>
        <Td>
          <span className="font-medium">{c.available_key_count}</span>
          <span className="text-subtle"> / {c.key_count}</span>
        </Td>
        <Td className="hidden max-w-56 min-[1280px]:table-cell">
          <div className="text-xs text-muted">
            {c.last_probe_at ? `${fmtTime(c.last_probe_at)} · ${fmtMs(c.last_probe_latency_ms)}` : '未探测'}
          </div>
          {c.last_probe_error && (
            <div className="truncate text-xs text-subtle" title={c.last_probe_error}>{c.last_probe_error}</div>
          )}
        </Td>
        <Td>
          <Switch checked={c.enabled} onChange={(enabled) => toggle.mutate({ id: c.id, enabled })} label={`启用渠道 ${c.name}`} />
        </Td>
        <Td>
          {test.isPending && test.variables === c.id ? (
            <Spinner />
          ) : result ? (
            <Badge
              tone={result.result === 'success' ? 'success' : result.result === 'inconclusive' ? 'neutral' : 'danger'}
              className="cursor-help"
              title={result.error || result.error_type || undefined}
            >
              {result.result === 'success' ? `✓ ${result.latency_ms}ms` : result.result === 'inconclusive' ? '? 无法判定' : '✗ 失败'}
            </Badge>
          ) : (
            <Button size="sm" variant="outline" aria-label={`测试渠道 ${c.name}`} title="测试连通性" onClick={() => test.mutate(c.id)}>
              <Plug size={13} />
            </Button>
          )}
        </Td>
        <Td>
          <div className="flex justify-end gap-1 whitespace-nowrap">
            <Button size="sm" variant="ghost" aria-label={`编辑渠道 ${c.name}`} title="编辑" onClick={onEdit}><Pencil size={13} /></Button>
            <Button size="sm" variant="ghost" aria-label={`删除渠道 ${c.name}`} title="删除" className="text-danger-text hover:bg-danger-subtle" onClick={onDelete}>
              <Trash2 size={13} />
            </Button>
          </div>
        </Td>
      </tr>
    </>
  )
}

// 渠道详情：key 管理（gpt-load GroupCredentialsTab 模式：展开行 + 批量导入 + 状态徽标）
function KeysPanel({ channelId, onDeleteKey }: { channelId: number; onDeleteKey: (id: number, label: string) => void }) {
  const detail = useChannel(channelId)
  const addKeys = useBatchAddKeys()
  const updateKey = useUpdateKey()
  const [keysText, setKeysText] = useState('')

  if (detail.isPending) return <Card className="eg-card-fluid"><SkeletonRows rows={3} /></Card>
  if (detail.isError) return <Card className="p-4"><ErrorText>{(detail.error as Error).message}</ErrorText></Card>

  return (
    <Card>
      <h3 className="mb-3 text-sm font-semibold">上游 key（渠道：{detail.data.channel.name}）</h3>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[700px] lg:min-w-0">
          <thead className="border-b border-line-soft bg-surface-soft">
            <tr>
              <Th>#</Th><Th>状态</Th><Th>权重</Th><Th>连续失败</Th>
              <Th className="hidden min-[1180px]:table-cell">退避档位</Th><Th>冷却剩余</Th>
              <Th className="hidden min-[1220px]:table-cell">成功/失败</Th><Th className="hidden min-[1280px]:table-cell">最近错误</Th>
              <Th>启用</Th><Th className="w-16"><span className="block pr-2.5 text-right">操作</span></Th>
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
                    <StatusBadge tone={channelStateTone(shownState)}>{stateLabel[shownState] || shownState}</StatusBadge>
                  </Td>
                  <Td>{k.weight}</Td>
                  <Td className={k.consecutive_fails > 0 ? 'text-warning-text font-medium' : ''}>{k.consecutive_fails}</Td>
                  <Td className="hidden min-[1180px]:table-cell">L{k.level}</Td>
                  <Td>{cooling ? <span className="text-warning-text">{formatRemainingSeconds(k.cooldown_remaining_sec)}</span> : '—'}</Td>
                  <Td className="hidden text-xs text-muted min-[1220px]:table-cell">{k.success_count}/{k.fail_count}</Td>
                  <Td className="hidden max-w-48 truncate text-xs text-subtle min-[1280px]:table-cell" title={k.last_error}>{k.last_error || '—'}</Td>
                  <Td>
                    <Switch checked={k.enabled} onChange={(enabled) => updateKey.mutate({ id: k.id, enabled })} label={`启用 key ${i + 1}`} />
                  </Td>
                  <Td>
                    <div className="flex justify-end">
                      <Button size="sm" variant="ghost" aria-label={`删除 key ${i + 1}`} title="删除" className="text-danger-text hover:bg-danger-subtle"
                        onClick={() => onDeleteKey(k.id, `第 ${i + 1} 把 key`)}>
                        <Trash2 size={13} />
                      </Button>
                    </div>
                  </Td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <div className="mt-4 rounded-lg border border-dashed border-line-strong p-3">
        <div className="mb-2 text-xs font-medium text-muted">批量导入 key（每行一把，加密存储，不会回显明文）</div>
        <Textarea rows={3} value={keysText} onChange={(e) => setKeysText(e.target.value)} placeholder={'sk-xxxxxxx\nsk-yyyyyyy'} />
        <div className="mt-2 flex items-center gap-3">
          <Button size="sm" disabled={!keysText.trim() || addKeys.isPending} onClick={() => addKeys.mutate(
            { channelId, keys: keysText },
            { onSuccess: () => setKeysText('') },
          )}>
            {addKeys.isPending ? '导入中…' : '导入'}
          </Button>
          {addKeys.data && <span className="text-xs text-success-text">已导入 {addKeys.data.added} 把</span>}
        </div>
      </div>
    </Card>
  )
}

// 新建/编辑渠道弹窗
function ChannelForm({ channel, onClose }: { channel: Channel | null; onClose: () => void }) {
  const save = useSaveChannel()
  const preview = usePreviewChannelModels()
  const formID = 'channel-form'
  const modelsId = useId()
  const [name, setName] = useState(channel?.name ?? '')
  const [nameTouched, setNameTouched] = useState(Boolean(channel))
  const [presetId, setPresetId] = useState(CUSTOM_CHANNEL_PRESET)
  const [type, setType] = useState(channel?.type ?? 'openai-compatible')
  const [baseUrl, setBaseUrl] = useState(channel?.base_url ?? '')
  const [priority, setPriority] = useState(String(channel?.priority ?? 0))
  const [weight, setWeight] = useState(String(channel?.weight ?? 1))
  const [supportsResponses, setSupportsResponses] = useState(channel?.supports_responses ?? false)
  const [supportsAnthropic, setSupportsAnthropic] = useState(channel?.supports_anthropic ?? false)
  const [supportsGemini, setSupportsGemini] = useState(channel?.supports_gemini ?? false)
  const [models, setModels] = useState((channel?.models || []).join('\n'))
  const [mapping, setMapping] = useState<Record<string, string>>(channel?.model_mapping ?? {})
  const [showModelPicker, setShowModelPicker] = useState(false)
  const [formError, setFormError] = useState('')
  // 新建时可内联导入 key：省去"保存 → 回列表 → 展开 → 粘贴 key → 再编辑"的四次往返（T77）。
  const [keysText, setKeysText] = useState('')
  const addKeys = useBatchAddKeys()
  const [connectSec, setConnectSec] = useState(channel?.timeout_config?.connect_sec?.toString() ?? '')
  const [firstTokenSec, setFirstTokenSec] = useState(channel?.timeout_config?.first_token_sec?.toString() ?? '')
  const [overallSec, setOverallSec] = useState(channel?.timeout_config?.overall_sec?.toString() ?? '')
  const preset = findChannelPreset(presetId)
  const typeOptions = presetId === CUSTOM_CHANNEL_PRESET ? CHANNEL_TYPES : [preset!.type]

  const applyPreset = (id: string) => {
    setPresetId(id)
    const next = findChannelPreset(id)
    if (!next) return
    setType(next.type)
    setBaseUrl(next.baseUrl)
    setSupportsResponses(next.supportsResponses)
    setSupportsAnthropic(next.supportsAnthropic)
    setSupportsGemini(next.supportsGemini ?? false)
    if (!nameTouched) setName(next.defaultName)
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const timeouts: Record<string, number> = {}
    if (connectSec.trim()) timeouts.connect_sec = Number(connectSec)
    if (firstTokenSec.trim()) timeouts.first_token_sec = Number(firstTokenSec)
    if (overallSec.trim()) timeouts.overall_sec = Number(overallSec)
    if (Object.values(timeouts).some((v) => !Number.isFinite(v) || v <= 0)) {
      setFormError('超时必须为正整数秒')
      return
    }
    setFormError('')
    save.mutate(
      {
        id: channel?.id,
        name,
        type,
        base_url: baseUrl,
        priority: Number(priority) || 0,
        weight: Number(weight) || 1,
        models: models.split('\n').map((s) => s.trim()).filter(Boolean),
        model_mapping: Object.keys(mapping).length ? mapping : null,
        supports_responses: supportsResponses,
        supports_anthropic: supportsAnthropic,
        supports_gemini: supportsGemini,
        ...(Object.keys(timeouts).length ? { timeout_config: timeouts } : {}),
      },
      {
        onSuccess: (saved) => {
          const keys = keysText.trim()
          if (!channel && keys) {
            addKeys.mutate({ channelId: saved.id, keys }, { onSuccess: onClose, onError: () => onClose() })
            return
          }
          onClose()
        },
      },
    )
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={channel ? '编辑渠道' : '新建渠道'}
      wide
      footer={
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>取消</Button>
          <Button type="submit" form={formID} disabled={save.isPending}>{save.isPending ? '保存中…' : '保存'}</Button>
        </div>
      }
    >
      <form id={formID} onSubmit={onSubmit} className="space-y-[var(--eg-control-gap)]">
        {!channel && (
          <div className="space-y-2">
            <Field label="供应商预设" hint="选择后自动填充 Base URL 与推荐配置，仍可手动调整">
              <Select value={presetId} onChange={(e) => applyPreset(e.target.value)}>
                <option value={CUSTOM_CHANNEL_PRESET}>自定义</option>
                {CHANNEL_PRESETS.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
              </Select>
            </Field>
            {preset && (
              <div className={`rounded-lg border px-3 py-2.5 ${preset.warning ? 'border-warning-border bg-warning-subtle' : 'border-line bg-surface-soft'}`}>
                <div className={`text-xs ${preset.warning ? 'text-warning-text' : 'text-muted'}`}>{preset.description}</div>
                <div className="mt-1 font-mono text-[11px] text-muted">{preset.baseUrl}</div>
                {preset.warning && (
                  <div className="mt-1.5 flex gap-1.5 text-xs font-medium text-warning-text">
                    <TriangleAlert size={13} className="mt-0.5 shrink-0" />
                    <span>{preset.warning}</span>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
        <div className="eg-grid eg-grid-auto-wide">
          <Field label="名称"><Input value={name} onChange={(e) => { setName(e.target.value); setNameTouched(true) }} required placeholder="openai-main" /></Field>
          <Field label="类型" hint={CHANNEL_TYPE_HELP[type]}>
            <Select value={type} onChange={(e) => setType(e.target.value)}>
              {typeOptions.map((t) => <option key={t} value={t}>{t}</option>)}
            </Select>
          </Field>
        </div>
        <Field label="Base URL" hint={`上游 API 根地址；网关按能力拼接 ${supportsAnthropic ? '/v1/messages、' : ''}${supportsResponses ? '/responses、' : ''}${supportsGemini ? '/v1beta/models/{model}、' : ''}/chat/completions`}>
          <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} required placeholder="https://api.openai.com/v1" />
        </Field>
        {!channel && (
          <Field label="上游 API key（可选，每行一把）" hint="填写后创建时直接导入，保存后即可「从上游拉取」模型目录；留空可稍后在渠道行展开导入">
            <Textarea rows={3} value={keysText} onChange={(e) => setKeysText(e.target.value)} placeholder={'sk-...\nsk-...'} autoComplete="off" className="font-mono text-xs" />
          </Field>
        )}
        <div className="space-y-2">
          <SettingRow
            title="原生支持 Responses API"
            description="开启后 /v1/responses 优先透传；关闭时转换为 Chat Completions"
            checked={supportsResponses}
            onChange={setSupportsResponses}
            label="原生支持 Responses API"
          />
          <SettingRow
            title="原生支持 Anthropic Messages"
            description="开启后 /v1/messages 透传；关闭时转换为 Chat Completions"
            checked={supportsAnthropic}
            onChange={setSupportsAnthropic}
            label="原生支持 Anthropic Messages"
          />
          <SettingRow
            title="原生支持 Gemini API"
            description="开启后 /v1beta/models/* 透传；关闭时转换为 Chat Completions"
            checked={supportsGemini}
            onChange={setSupportsGemini}
            label="原生支持 Gemini API"
          />
        </div>
        <div className="eg-grid eg-grid-auto-wide">
          <Field label="优先级" hint="越小越优先"><Input type="number" value={priority} onChange={(e) => setPriority(e.target.value)} /></Field>
          <Field label="权重" hint="同优先级内加权随机"><Input type="number" value={weight} onChange={(e) => setWeight(e.target.value)} /></Field>
        </div>
        <div className="eg-grid eg-grid-auto-wide">
          <Field label="连接超时（秒）" hint="留空用全局设置；1–300"><Input type="number" min={1} max={300} value={connectSec} onChange={(e) => setConnectSec(e.target.value)} placeholder="留空" /></Field>
          <Field label="首 token 超时（秒）" hint="留空用全局设置；1–3600"><Input type="number" min={1} max={3600} value={firstTokenSec} onChange={(e) => setFirstTokenSec(e.target.value)} placeholder="留空" /></Field>
          <Field label="整体超时（秒）" hint="留空用全局设置；不小于前两项"><Input type="number" min={1} max={86400} value={overallSec} onChange={(e) => setOverallSec(e.target.value)} placeholder="留空" /></Field>
        </div>
        <div className="space-y-2">
          <div className="flex flex-wrap items-end justify-between gap-2">
            <label htmlFor={modelsId} className="text-sm font-medium text-foreground-soft">支持的模型（每行一个）</label>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={!channel || channel.key_count === 0 || preview.isPending}
              title={!channel ? '保存渠道后可用' : channel.key_count === 0 ? '请先导入至少一把 key' : undefined}
              onClick={() => {
                if (!channel) return
                setShowModelPicker(false)
                preview.reset()
                preview.mutate(channel.id, { onSuccess: () => setShowModelPicker(true) })
              }}
            >
              <RefreshCw size={13} className={preview.isPending ? 'animate-spin' : ''} />
              {preview.isPending ? '拉取中…' : '从上游拉取'}
            </Button>
          </div>
          <Textarea id={modelsId} rows={4} value={models} onChange={(e) => setModels(e.target.value)} placeholder={preset?.modelsPlaceholder ?? 'gpt-4o\ngpt-4o-mini'} />
          <div className="text-xs text-subtle">
            {!channel
              ? '可留空：保存并导入 key 后，再「从上游拉取」模型目录；也可直接手填。'
              : channel.key_count === 0
                ? '请先在渠道行展开并导入至少一把启用 key。'
                : '候选映射按当前已保存的模型映射生成；确认后会写入本表单，仍需点击保存。'}
          </div>
          {showModelPicker && preview.data && (
            <ModelPicker
              title={`${channel?.name ?? ''} 上游模型`}
              options={preview.data.items.map((item) => ({
                publicModel: item.public_model,
                upstreamModel: item.upstream_model,
              }))}
              initialModels={splitLines(models)}
              protocol={preview.data.protocol}
              truncated={preview.data.truncated}
              loading={preview.isPending}
              onCancel={() => {
                setShowModelPicker(false)
                preview.reset()
              }}
              onApply={(selected, mode) => {
                const next = applyModelSelection(splitLines(models), selected, mode)
                setModels(next.join('\n'))
                setShowModelPicker(false)
                preview.reset()
              }}
            />
          )}
          {preview.isError && !preview.data && (
            <ErrorText>拉取失败：{(preview.error as Error).message}</ErrorText>
          )}
        </div>
        <ModelMappingEditor value={mapping} onChange={setMapping} />
        <ErrorText>{formError}</ErrorText>
        <ErrorText>{save.isError ? (save.error as Error).message : addKeys.isError ? `渠道已创建，但 key 导入失败：${(addKeys.error as Error).message}。可在渠道行展开重新导入。` : null}</ErrorText>
      </form>
    </Dialog>
  )
}

function splitLines(text: string): string[] {
  return text.split(/\r?\n/).map((item) => item.trim()).filter(Boolean)
}

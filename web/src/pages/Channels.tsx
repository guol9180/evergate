import { type FormEvent, useState } from 'react'
import { ChevronDown, ChevronRight, Pencil, Plug, Plus, Trash2, TriangleAlert } from 'lucide-react'
import { useBatchAddKeys, useChannel, useChannels, useDeleteChannel, useDeleteKey, useSaveChannel, useTestChannel, useToggleChannel, useUpdateKey } from '../api/hooks'
import { Badge, Button, Card, Dialog, Empty, ErrorText, Field, Input, PageHeader, Select, Spinner, Switch, Td, Textarea, Th } from '../components/ui'
import type { Channel } from '../api/types'
import { CHANNEL_PRESETS, CUSTOM_CHANNEL_PRESET, findChannelPreset } from '../lib/channelPresets'
import { cooldownRemaining, fmtMs, fmtTime, stateLabel, statusRank, stateTone } from '../lib/format'

export default function Channels() {
  const channels = useChannels()
  const [editing, setEditing] = useState<Channel | 'new' | null>(null)
  const [expanded, setExpanded] = useState<number | null>(null)

  if (channels.isPending) return <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>
  const items = [...(channels.data?.items ?? [])].sort((a, b) => statusRank(a.status) - statusRank(b.status) || a.priority - b.priority || a.id - b.id)

  return (
    <div className="space-y-4">
      <PageHeader
        title="渠道管理"
        description="上游渠道、优先级权重与 key 池状态；展开行可管理凭据。"
        actions={<Button onClick={() => setEditing('new')}><Plus size={15} /> 新建渠道</Button>}
      />

      <Card>
        {channels.data?.items.length === 0 ? (
          <Empty icon={<Plug size={36} />} title="还没有渠道" hint="新建一个上游渠道，填入 base_url 与支持的模型" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[960px]">
              <thead className="border-b border-slate-100 bg-slate-50/50">
                <tr>
                  <Th className="w-8" /><Th>名称</Th><Th>状态</Th><Th>类型</Th><Th>优先级</Th><Th>权重</Th>
                  <Th>模型</Th><Th>key / 可用</Th><Th>最近探测</Th><Th>启用</Th><Th>测试</Th><Th className="w-28"><span className="block pr-2.5 text-right">操作</span></Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-50">
                {items.map((c) => (
                  <ChannelRow
                    key={c.id}
                    channel={c}
                    expanded={expanded === c.id}
                    onToggleExpand={() => setExpanded(expanded === c.id ? null : c.id)}
                    onEdit={() => setEditing(c)}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {expanded != null && <KeysPanel channelId={expanded} />}
      {editing && <ChannelForm channel={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
    </div>
  )
}

function ChannelRow({ channel: c, expanded, onToggleExpand, onEdit }: { channel: Channel; expanded: boolean; onToggleExpand: () => void; onEdit: () => void }) {
  const toggle = useToggleChannel()
  const test = useTestChannel()
  const del = useDeleteChannel()
  const result = test.data && test.variables === c.id ? test.data : null

  return (
    <>
      <tr className="hover:bg-slate-50/50">
        <Td>
          <button onClick={onToggleExpand} aria-label={expanded ? '收起 key 列表' : '展开 key 列表'} className="rounded p-1 text-slate-400 hover:bg-slate-100">
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        </Td>
        <Td className="font-medium">{c.name}</Td>
        <Td>
          <Badge tone={stateTone[c.status] || ''}>
            <span title={c.last_probe_error || undefined}>{stateLabel[c.status] || c.status}</span>
          </Badge>
        </Td>
        <Td>
          <div className="flex flex-wrap items-center gap-1">
            <span className="text-slate-500">{c.type}</span>
            {c.supports_responses && <Badge tone="bg-sky-100 text-sky-700">Responses</Badge>}
          </div>
        </Td>
        <Td>{c.priority}</Td>
        <Td>{c.weight}</Td>
        <Td>{(c.models || []).length}</Td>
        <Td>
          <span className="font-medium">{c.available_key_count}</span>
          <span className="text-slate-400"> / {c.key_count}</span>
        </Td>
        <Td className="max-w-56">
          <div className="text-xs text-slate-500">
            {c.last_probe_at ? `${fmtTime(c.last_probe_at)} · ${fmtMs(c.last_probe_latency_ms)}` : '未探测'}
          </div>
          {c.last_probe_error && (
            <div className="truncate text-xs text-slate-400" title={c.last_probe_error}>{c.last_probe_error}</div>
          )}
        </Td>
        <Td>
          <Switch checked={c.enabled} onChange={(enabled) => toggle.mutate({ id: c.id, enabled })} label={`启用渠道 ${c.name}`} />
        </Td>
        <Td>
          {test.isPending && test.variables === c.id ? (
            <Spinner />
          ) : result ? (
            <Badge tone={result.result === 'success' ? 'bg-emerald-100 text-emerald-700' : result.result === 'inconclusive' ? 'bg-slate-200 text-slate-700' : 'bg-red-100 text-red-700'}>
              {result.result === 'success' ? `✓ ${result.latency_ms}ms` : result.result === 'inconclusive' ? `? ${result.status_code ?? 0}` : `✗ ${result.status_code ?? 'err'}`}
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
            <Button size="sm" variant="ghost" aria-label={`删除渠道 ${c.name}`} title="删除" className="text-red-500 hover:bg-red-50"
              onClick={() => confirm(`删除渠道「${c.name}」及其全部 key？`) && del.mutate(c.id)}>
              <Trash2 size={13} />
            </Button>
          </div>
        </Td>
      </tr>
    </>
  )
}

// 渠道详情：key 管理（gpt-load GroupCredentialsTab 模式：展开行 + 批量导入 + 状态徽标）
function KeysPanel({ channelId }: { channelId: number }) {
  const detail = useChannel(channelId)
  const addKeys = useBatchAddKeys()
  const updateKey = useUpdateKey()
  const delKey = useDeleteKey()
  const [keysText, setKeysText] = useState('')

  if (detail.isPending) return <Card className="p-6 text-center text-sm text-slate-400"><Spinner className="mr-2" />加载 key…</Card>
  if (detail.isError) return <Card className="p-4"><ErrorText>{(detail.error as Error).message}</ErrorText></Card>

  return (
    <Card className="p-4">
      <h3 className="mb-3 text-sm font-semibold">上游 key（渠道：{detail.data.channel.name}）</h3>
      <div className="overflow-x-auto">
        <table className="w-full min-w-[860px]">
          <thead className="border-b border-slate-100 bg-slate-50/50">
            <tr><Th>#</Th><Th>状态</Th><Th>权重</Th><Th>连续失败</Th><Th>退避档位</Th><Th>冷却剩余</Th><Th>成功/失败</Th><Th>最近错误</Th><Th>启用</Th><Th className="w-16"><span className="block pr-2.5 text-right">操作</span></Th></tr>
          </thead>
          <tbody className="divide-y divide-slate-50">
            {detail.data.keys.map((k, i) => {
              const remain = cooldownRemaining(k.cooldown_until)
              return (
                <tr key={k.id}>
                  <Td className="text-slate-400">{i + 1}</Td>
                  <Td>
                    <Badge tone={stateTone[k.state] || ''}>{stateLabel[k.state] || k.state}</Badge>
                  </Td>
                  <Td>{k.weight}</Td>
                  <Td className={k.consecutive_fails > 0 ? 'text-amber-600 font-medium' : ''}>{k.consecutive_fails}</Td>
                  <Td>L{k.level}</Td>
                  <Td>{remain ? <span className="text-amber-600">{remain}</span> : '—'}</Td>
                  <Td className="text-xs text-slate-500">{k.success_count}/{k.fail_count}</Td>
                  <Td className="max-w-48 truncate text-xs text-slate-400" title={k.last_error}>{k.last_error || '—'}</Td>
                  <Td>
                    <Switch checked={k.enabled} onChange={(enabled) => updateKey.mutate({ id: k.id, enabled })} label={`启用 key ${i + 1}`} />
                  </Td>
                  <Td>
                    <div className="flex justify-end">
                      <Button size="sm" variant="ghost" aria-label={`删除 key ${i + 1}`} title="删除" className="text-red-500 hover:bg-red-50"
                        onClick={() => confirm('删除这把 key？') && delKey.mutate({ id: k.id, channelId })}>
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
      <div className="mt-4 rounded-lg border border-dashed border-slate-300 p-3">
        <div className="mb-2 text-xs font-medium text-slate-600">批量导入 key（每行一把，加密存储，不会回显明文）</div>
        <Textarea rows={3} value={keysText} onChange={(e) => setKeysText(e.target.value)} placeholder={'sk-xxxxxxx\nsk-yyyyyyy'} />
        <div className="mt-2 flex items-center gap-3">
          <Button size="sm" disabled={!keysText.trim() || addKeys.isPending} onClick={() => addKeys.mutate(
            { channelId, keys: keysText },
            { onSuccess: () => setKeysText('') },
          )}>
            {addKeys.isPending ? '导入中…' : '导入'}
          </Button>
          {addKeys.data && <span className="text-xs text-emerald-600">已导入 {addKeys.data.added} 把</span>}
        </div>
      </div>
    </Card>
  )
}

// 新建/编辑渠道弹窗
function ChannelForm({ channel, onClose }: { channel: Channel | null; onClose: () => void }) {
  const save = useSaveChannel()
  const formID = 'channel-form'
  const [name, setName] = useState(channel?.name ?? '')
  const [nameTouched, setNameTouched] = useState(Boolean(channel))
  const [presetId, setPresetId] = useState(CUSTOM_CHANNEL_PRESET)
  const [type, setType] = useState(channel?.type ?? 'openai-compatible')
  const [baseUrl, setBaseUrl] = useState(channel?.base_url ?? '')
  const [priority, setPriority] = useState(String(channel?.priority ?? 0))
  const [weight, setWeight] = useState(String(channel?.weight ?? 1))
  const [supportsResponses, setSupportsResponses] = useState(channel?.supports_responses ?? false)
  const [models, setModels] = useState((channel?.models || []).join('\n'))
  const [mapping, setMapping] = useState(channel?.model_mapping ? JSON.stringify(channel.model_mapping, null, 2) : '')
  const preset = findChannelPreset(presetId)

  const applyPreset = (id: string) => {
    setPresetId(id)
    const next = findChannelPreset(id)
    if (!next) return
    setType(next.type)
    setBaseUrl(next.baseUrl)
    setSupportsResponses(next.supportsResponses)
    if (!nameTouched) setName(next.defaultName)
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    let modelMapping: Record<string, string> | null = null
    if (mapping.trim()) {
      try {
        modelMapping = JSON.parse(mapping)
      } catch {
        alert('模型映射不是合法 JSON')
        return
      }
    }
    save.mutate(
      {
        id: channel?.id,
        name,
        type,
        base_url: baseUrl,
        priority: Number(priority) || 0,
        weight: Number(weight) || 1,
        models: models.split('\n').map((s) => s.trim()).filter(Boolean),
        model_mapping: modelMapping,
        supports_responses: supportsResponses,
      },
      { onSuccess: onClose },
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
      <form id={formID} onSubmit={onSubmit} className="space-y-4">
        {!channel && (
          <div className="space-y-2">
            <Field label="供应商预设" hint="选择后自动填充 Base URL 与推荐配置，仍可手动调整">
              <Select value={presetId} onChange={(e) => applyPreset(e.target.value)}>
                <option value={CUSTOM_CHANNEL_PRESET}>自定义</option>
                {CHANNEL_PRESETS.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
              </Select>
            </Field>
            {preset && (
              <div className={`rounded-lg border px-3 py-2.5 ${preset.warning ? 'border-amber-200 bg-amber-50' : 'border-slate-200 bg-slate-50/70'}`}>
                <div className={`text-xs ${preset.warning ? 'text-amber-800' : 'text-slate-600'}`}>{preset.description}</div>
                <div className="mt-1 font-mono text-[11px] text-slate-500">{preset.baseUrl}</div>
                {preset.warning && (
                  <div className="mt-1.5 flex gap-1.5 text-xs font-medium text-amber-700">
                    <TriangleAlert size={13} className="mt-0.5 shrink-0" />
                    <span>{preset.warning}</span>
                  </div>
                )}
              </div>
            )}
          </div>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="名称"><Input value={name} onChange={(e) => { setName(e.target.value); setNameTouched(true) }} required placeholder="openai-main" /></Field>
          <Field label="类型">
            <Select value={type} onChange={(e) => setType(e.target.value)}>
              {['openai', 'openai-compatible', 'deepseek', 'vllm', 'anthropic-compat', 'gemini-compat'].map((t) => <option key={t}>{t}</option>)}
            </Select>
          </Field>
        </div>
        <Field label="Base URL" hint={`上游 API 根地址（一般以 /v1 结尾），网关会拼接 ${supportsResponses ? '/responses（Responses 请求）与 /chat/completions（Chat 请求）' : '/chat/completions'}`}>
          <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} required placeholder="https://api.openai.com/v1" />
        </Field>
        <div className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 bg-slate-50/50 px-3 py-2.5">
          <div className="min-w-0">
            <div className="text-sm text-slate-700">原生支持 Responses API</div>
            <div className="text-xs text-slate-400">开启后 /v1/responses 优先透传；关闭时转换为 Chat Completions</div>
          </div>
          <Switch checked={supportsResponses} onChange={setSupportsResponses} label="原生支持 Responses API" />
        </div>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="优先级" hint="越小越优先"><Input type="number" value={priority} onChange={(e) => setPriority(e.target.value)} /></Field>
          <Field label="权重" hint="同优先级内加权随机"><Input type="number" value={weight} onChange={(e) => setWeight(e.target.value)} /></Field>
        </div>
        <Field label="支持的模型（每行一个）">
          <Textarea rows={4} value={models} onChange={(e) => setModels(e.target.value)} placeholder={preset?.modelsPlaceholder ?? 'gpt-4o\ngpt-4o-mini'} required />
        </Field>
        <Field label="模型映射（可选 JSON）" hint='{"请求模型": "上游模型"}，如 {"gpt-4": "gpt-4o-2024"}'>
          <Textarea rows={3} value={mapping} onChange={(e) => setMapping(e.target.value)} placeholder='{}' />
        </Field>
        <ErrorText>{save.isError ? (save.error as Error).message : null}</ErrorText>
      </form>
    </Dialog>
  )
}

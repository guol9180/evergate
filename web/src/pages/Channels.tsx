import { type FormEvent, useState } from 'react'
import { ChevronDown, ChevronRight, Plug, Plus, Trash2 } from 'lucide-react'
import { useBatchAddKeys, useChannel, useChannels, useDeleteChannel, useDeleteKey, useSaveChannel, useTestChannel, useToggleChannel, useUpdateKey } from '../api/hooks'
import { Badge, Button, Card, Dialog, Drawer, Empty, ErrorText, Field, Input, Spinner, Td, Textarea, Th } from '../components/ui'
import type { Channel } from '../api/types'
import { cooldownRemaining, stateLabel, stateTone } from '../lib/format'

export default function Channels() {
  const channels = useChannels()
  const [editing, setEditing] = useState<Channel | 'new' | null>(null)
  const [expanded, setExpanded] = useState<number | null>(null)

  if (channels.isPending) return <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">渠道管理</h1>
        <Button onClick={() => setEditing('new')}><Plus size={15} /> 新建渠道</Button>
      </div>

      <Card>
        {channels.data?.items.length === 0 ? (
          <Empty icon={<Plug size={36} />} title="还没有渠道" hint="新建一个上游渠道，填入 base_url 与支持的模型" />
        ) : (
          <table className="w-full">
            <thead className="border-b border-slate-100 bg-slate-50/50">
              <tr>
                <Th className="w-8" /><Th>名称</Th><Th>状态</Th><Th>类型</Th><Th>优先级</Th><Th>权重</Th>
                <Th>模型</Th><Th>key</Th><Th>启用</Th><Th>测试</Th><Th className="w-20">操作</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-50">
              {channels.data?.items.map((c) => (
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
          <button onClick={onToggleExpand} className="rounded p-1 text-slate-400 hover:bg-slate-100">
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>
        </Td>
        <Td className="font-medium">{c.name}</Td>
        <Td><Badge tone={stateTone[c.status] || ''}>{stateLabel[c.status] || c.status}</Badge></Td>
        <Td>
          <div className="flex flex-wrap items-center gap-1">
            <span className="text-slate-500">{c.type}</span>
            {c.supports_responses && <Badge tone="bg-sky-100 text-sky-700">Responses</Badge>}
          </div>
        </Td>
        <Td>{c.priority}</Td>
        <Td>{c.weight}</Td>
        <Td>{(c.models || []).length}</Td>
        <Td>{c.key_count}</Td>
        <Td>
          <button
            onClick={() => toggle.mutate({ id: c.id, enabled: !c.enabled })}
            className={`h-5 w-9 rounded-full transition-colors ${c.enabled ? 'bg-emerald-500' : 'bg-slate-300'}`}
          >
            <span className={`block h-4 w-4 translate-x-0.5 rounded-full bg-white transition-transform ${c.enabled ? 'translate-x-4' : ''}`} />
          </button>
        </Td>
        <Td>
          {test.isPending && test.variables === c.id ? (
            <Spinner />
          ) : result ? (
            <Badge tone={result.ok ? 'bg-emerald-100 text-emerald-700' : 'bg-red-100 text-red-700'}>
              {result.ok ? `✓ ${result.latency_ms}ms` : `✗ ${result.status_code ?? 'err'}`}
            </Badge>
          ) : (
            <Button size="sm" variant="outline" onClick={() => test.mutate(c.id)}>测试</Button>
          )}
        </Td>
        <Td>
          <div className="flex gap-1">
            <Button size="sm" variant="ghost" onClick={onEdit}>编辑</Button>
            <Button size="sm" variant="ghost" className="text-red-500 hover:bg-red-50"
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
      <table className="w-full">
        <thead className="border-b border-slate-100 bg-slate-50/50">
          <tr><Th>#</Th><Th>状态</Th><Th>权重</Th><Th>连续失败</Th><Th>退避档位</Th><Th>冷却剩余</Th><Th>成功/失败</Th><Th>最近错误</Th><Th>启用</Th><Th className="w-16">操作</Th></tr>
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
                  <button
                    onClick={() => updateKey.mutate({ id: k.id, enabled: !k.enabled })}
                    className={`h-5 w-9 rounded-full transition-colors ${k.enabled ? 'bg-emerald-500' : 'bg-slate-300'}`}
                  >
                    <span className={`block h-4 w-4 translate-x-0.5 rounded-full bg-white transition-transform ${k.enabled ? 'translate-x-4' : ''}`} />
                  </button>
                </Td>
                <Td>
                  <Button size="sm" variant="ghost" className="text-red-500 hover:bg-red-50"
                    onClick={() => confirm('删除这把 key？') && delKey.mutate({ id: k.id, channelId })}>
                    <Trash2 size={13} />
                  </Button>
                </Td>
              </tr>
            )
          })}
        </tbody>
      </table>
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
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState(channel?.type ?? 'openai-compatible')
  const [baseUrl, setBaseUrl] = useState(channel?.base_url ?? '')
  const [priority, setPriority] = useState(String(channel?.priority ?? 0))
  const [weight, setWeight] = useState(String(channel?.weight ?? 1))
  const [supportsResponses, setSupportsResponses] = useState(channel?.supports_responses ?? false)
  const [models, setModels] = useState((channel?.models || []).join('\n'))
  const [mapping, setMapping] = useState(channel?.model_mapping ? JSON.stringify(channel.model_mapping, null, 2) : '')

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
    <Dialog open onClose={onClose} title={channel ? '编辑渠道' : '新建渠道'} wide>
      <form onSubmit={onSubmit} className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <Field label="名称"><Input value={name} onChange={(e) => setName(e.target.value)} required placeholder="openai-main" /></Field>
          <Field label="类型">
            <select value={type} onChange={(e) => setType(e.target.value)}
              className="h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm">
              {['openai', 'openai-compatible', 'deepseek', 'vllm', 'anthropic-compat', 'gemini-compat'].map((t) => <option key={t}>{t}</option>)}
            </select>
          </Field>
        </div>
        <Field label="Base URL" hint={`上游 API 根地址（一般以 /v1 结尾），网关会拼接 ${supportsResponses ? '/responses（Responses 请求）与 /chat/completions（Chat 请求）' : '/chat/completions'}`}>
          <Input value={baseUrl} onChange={(e) => setBaseUrl(e.target.value)} required placeholder="https://api.openai.com/v1" />
        </Field>
        <label className="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50/50 px-3 py-2 text-sm text-slate-700">
          <input type="checkbox" checked={supportsResponses} onChange={(e) => setSupportsResponses(e.target.checked)}
            className="h-4 w-4 rounded border-slate-300 text-slate-900" />
          <span>
            原生支持 Responses API
            <span className="block text-xs text-slate-400">开启后 /v1/responses 优先透传；关闭时转换为 Chat Completions</span>
          </span>
        </label>
        <div className="grid grid-cols-2 gap-4">
          <Field label="优先级" hint="越小越优先"><Input type="number" value={priority} onChange={(e) => setPriority(e.target.value)} /></Field>
          <Field label="权重" hint="同优先级内加权随机"><Input type="number" value={weight} onChange={(e) => setWeight(e.target.value)} /></Field>
        </div>
        <Field label="支持的模型（每行一个）">
          <Textarea rows={4} value={models} onChange={(e) => setModels(e.target.value)} placeholder={'gpt-4o\ngpt-4o-mini'} required />
        </Field>
        <Field label="模型映射（可选 JSON）" hint='{"请求模型": "上游模型"}，如 {"gpt-4": "gpt-4o-2024"}'>
          <Textarea rows={3} value={mapping} onChange={(e) => setMapping(e.target.value)} placeholder='{}' />
        </Field>
        <ErrorText>{save.isError ? (save.error as Error).message : null}</ErrorText>
        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="outline" onClick={onClose}>取消</Button>
          <Button type="submit" disabled={save.isPending}>{save.isPending ? '保存中…' : '保存'}</Button>
        </div>
      </form>
    </Dialog>
  )
}

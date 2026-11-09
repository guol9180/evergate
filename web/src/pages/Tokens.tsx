import { useState } from 'react'
import { Check, Copy, KeyRound, Plus, Trash2 } from 'lucide-react'
import { useCreateToken, useDeleteToken, useTokens, useUpdateToken } from '../api/hooks'
import { Badge, Button, Card, Dialog, Empty, Field, Input, Spinner, Td, Th } from '../components/ui'
import { fmtTime } from '../lib/format'

export default function Tokens() {
  const tokens = useTokens()
  const del = useDeleteToken()
  const update = useUpdateToken()
  const [creating, setCreating] = useState(false)

  if (tokens.isPending) return <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">令牌管理</h1>
        <Button onClick={() => setCreating(true)}><Plus size={15} /> 新建令牌</Button>
      </div>
      <Card>
        {tokens.data?.items.length === 0 ? (
          <Empty icon={<KeyRound size={36} />} title="还没有下游令牌" hint="创建令牌后发给客户端，作为调用网关的 API key" />
        ) : (
          <table className="w-full">
            <thead className="border-b border-slate-100 bg-slate-50/50">
              <tr><Th>名称</Th><Th>状态</Th><Th>额度</Th><Th>允许模型</Th><Th>过期时间</Th><Th>创建时间</Th><Th>启用</Th><Th className="w-16">操作</Th></tr>
            </thead>
            <tbody className="divide-y divide-slate-50">
              {tokens.data?.items.map((t) => (
                <tr key={t.id} className="hover:bg-slate-50/50">
                  <Td className="font-medium">{t.name}</Td>
                  <Td><Badge tone={t.enabled ? 'bg-emerald-100 text-emerald-700' : 'bg-slate-100 text-slate-500'}>{t.enabled ? '启用' : '停用'}</Badge></Td>
                  <Td>{t.quota > 0 ? t.used_quota + ' / ' + t.quota : '不限'}</Td>
                  <Td className="text-xs text-slate-500">{(t.allowed_models || []).join(', ') || '全部'}</Td>
                  <Td className="text-xs">{fmtTime(t.expired_at)}</Td>
                  <Td className="text-xs text-slate-400">{fmtTime(t.created_at)}</Td>
                  <Td>
                    <button onClick={() => update.mutate({ id: t.id, enabled: !t.enabled })}
                      className={`h-5 w-9 rounded-full transition-colors ${t.enabled ? 'bg-emerald-500' : 'bg-slate-300'}`}>
                      <span className={`block h-4 w-4 translate-x-0.5 rounded-full bg-white transition-transform ${t.enabled ? 'translate-x-4' : ''}`} />
                    </button>
                  </Td>
                  <Td>
                    <Button size="sm" variant="ghost" className="text-red-500 hover:bg-red-50"
                      onClick={() => confirm(`删除令牌「${t.name}」？使用它的客户端将立即失效。`) && del.mutate(t.id)}>
                      <Trash2 size={13} />
                    </Button>
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>
      {creating && <CreateTokenDialog onClose={() => setCreating(false)} />}
    </div>
  )
}

// 创建成功 → 明文仅此一次 + 一键复制（new-api 模式）
function CreateTokenDialog({ onClose }: { onClose: () => void }) {
  const create = useCreateToken()
  const [name, setName] = useState('')
  const [copied, setCopied] = useState(false)

  if (create.data) {
    return (
      <Dialog open onClose={onClose} title="令牌已创建">
        <div className="space-y-4">
          <div className="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-700">
            ⚠️ 请立即复制保存——明文 key 仅此一次显示，关闭后无法再次查看。
          </div>
          <div className="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 p-3">
            <code className="flex-1 break-all font-mono text-sm">{create.data.key}</code>
            <Button size="sm" variant="outline" onClick={() => {
              navigator.clipboard.writeText(create.data.key)
              setCopied(true)
              setTimeout(() => setCopied(false), 1500)
            }}>
              {copied ? <Check size={14} className="text-emerald-600" /> : <Copy size={14} />}
              {copied ? '已复制' : '复制'}
            </Button>
          </div>
          <Button className="w-full" onClick={onClose}>我已保存，关闭</Button>
        </div>
      </Dialog>
    )
  }

  return (
    <Dialog open onClose={onClose} title="新建令牌">
      <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); name && create.mutate({ name }) }}>
        <Field label="名称" hint="便于识别用途，如 cli-token、chatbox">
          <Input value={name} onChange={(e) => setName(e.target.value)} required placeholder="my-token" />
        </Field>
        <Button type="submit" className="w-full" disabled={create.isPending || !name}>
          {create.isPending ? '创建中…' : '创建'}
        </Button>
      </form>
    </Dialog>
  )
}

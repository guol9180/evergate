import { type FormEvent, useState } from 'react'
import { Check, Copy, KeyRound, Pencil, Plus, Trash2 } from 'lucide-react'
import { useCreateToken, useDeleteToken, useTokens, useUpdateToken } from '../api/hooks'
import type { Token } from '../api/types'
import { Badge, Button, Card, Dialog, Empty, ErrorText, Field, Input, Spinner, Td, Textarea, Th } from '../components/ui'
import { fmtInt, fmtTime } from '../lib/format'

const tokenStatusTone: Record<string, string> = {
  active: 'bg-emerald-100 text-emerald-700',
  disabled: 'bg-slate-100 text-slate-500',
  expired: 'bg-amber-100 text-amber-700',
  exhausted: 'bg-red-100 text-red-700',
}

const tokenStatusLabel: Record<string, string> = {
  active: '可用',
  disabled: '停用',
  expired: '过期',
  exhausted: '额度用尽',
}

export default function Tokens() {
  const tokens = useTokens()
  const del = useDeleteToken()
  const update = useUpdateToken()
  const [editing, setEditing] = useState<Token | 'new' | null>(null)

  if (tokens.isPending) return <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-bold">令牌管理</h1>
        <Button onClick={() => setEditing('new')}><Plus size={15} /> 新建令牌</Button>
      </div>
      <Card>
        {tokens.data?.items.length === 0 ? (
          <Empty icon={<KeyRound size={36} />} title="还没有下游令牌" hint="创建令牌后发给客户端，作为调用网关的 API key" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="border-b border-slate-100 bg-slate-50/50">
                <tr>
                  <Th>名称</Th><Th>状态</Th><Th>额度</Th><Th>允许模型</Th><Th>IP 白名单</Th>
                  <Th>过期时间</Th><Th>创建时间</Th><Th>启用</Th><Th className="w-24">操作</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-50">
                {tokens.data?.items.map((t) => (
                  <tr key={t.id} className="hover:bg-slate-50/50">
                    <Td className="font-medium">{t.name}</Td>
                    <Td><Badge tone={tokenStatusTone[t.status] || ''}>{tokenStatusLabel[t.status] || t.status}</Badge></Td>
                    <Td>
                      {t.quota > 0 ? (
                        <div>
                          <div>{fmtInt(t.used_quota)} / {fmtInt(t.quota)}</div>
                          <div className={`text-xs ${t.remaining_quota === 0 ? 'text-red-500' : 'text-slate-400'}`}>
                            剩余 {fmtInt(t.remaining_quota)}
                          </div>
                        </div>
                      ) : (
                        <span className="text-slate-400">不限</span>
                      )}
                    </Td>
                    <Td className="max-w-40 text-xs text-slate-500">
                      <div className="truncate" title={(t.allowed_models || []).join(', ')}>
                        {(t.allowed_models || []).join(', ') || '全部模型'}
                      </div>
                    </Td>
                    <Td className="max-w-40 text-xs text-slate-500">
                      <div className="truncate" title={(t.allowed_ips || []).join(', ')}>
                        {(t.allowed_ips || []).join(', ') || '全部 IP'}
                      </div>
                    </Td>
                    <Td className="text-xs">{fmtTime(t.expired_at)}</Td>
                    <Td className="text-xs text-slate-400">{fmtTime(t.created_at)}</Td>
                    <Td>
                      <button
                        onClick={() => update.mutate({ id: t.id, enabled: !t.enabled })}
                        className={`h-5 w-9 rounded-full transition-colors ${t.enabled ? 'bg-emerald-500' : 'bg-slate-300'}`}
                      >
                        <span className={`block h-4 w-4 translate-x-0.5 rounded-full bg-white transition-transform ${t.enabled ? 'translate-x-4' : ''}`} />
                      </button>
                    </Td>
                    <Td>
                      <div className="flex gap-1">
                        <Button size="sm" variant="ghost" onClick={() => setEditing(t)}><Pencil size={13} /></Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-red-500 hover:bg-red-50"
                          onClick={() => confirm(`删除令牌「${t.name}」？使用它的客户端将立即失效。`) && del.mutate(t.id)}
                        >
                          <Trash2 size={13} />
                        </Button>
                      </div>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      {editing && <TokenDialog token={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
    </div>
  )
}

function TokenDialog({ token, onClose }: { token: Token | null; onClose: () => void }) {
  const create = useCreateToken()
  const update = useUpdateToken()
  const [name, setName] = useState(token?.name ?? '')
  const [quota, setQuota] = useState(String(token?.quota ?? 0))
  const [expiredAt, setExpiredAt] = useState(toLocalInput(token?.expired_at))
  const [modelsText, setModelsText] = useState((token?.allowed_models || []).join('\n'))
  const [ipsText, setIpsText] = useState((token?.allowed_ips || []).join('\n'))
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

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const payload = {
      name,
      quota: Number(quota) || 0,
      expired_at: expiredAt ? new Date(expiredAt).toISOString() : null,
      allowed_models: splitLines(modelsText),
      allowed_ips: splitLines(ipsText),
    }
    if (token) {
      update.mutate({ id: token.id, ...payload, clear_expired_at: !expiredAt }, { onSuccess: onClose })
    } else {
      create.mutate(payload, { onSuccess: () => undefined })
    }
  }

  return (
    <Dialog open onClose={onClose} title={token ? '编辑令牌治理' : '新建令牌'} wide>
      <form onSubmit={onSubmit} className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <Field label="名称" hint="便于识别用途，如 cli-token、chatbox">
            <Input value={name} onChange={(e) => setName(e.target.value)} required placeholder="my-token" />
          </Field>
          <Field label="Token 额度" hint="0 表示不限；成功响应按输入 + 输出 token 扣减">
            <Input type="number" min={0} value={quota} onChange={(e) => setQuota(e.target.value)} />
          </Field>
        </div>
        <Field label="过期时间" hint="留空表示不限；编辑时留空会清除过期时间">
          <Input type="datetime-local" value={expiredAt} onChange={(e) => setExpiredAt(e.target.value)} />
        </Field>
        <div className="grid grid-cols-2 gap-4">
          <Field label="允许模型（每行一个）" hint="留空表示允许全部模型">
            <Textarea rows={4} value={modelsText} onChange={(e) => setModelsText(e.target.value)} placeholder={'gpt-4o\nglm-4.7'} />
          </Field>
          <Field label="允许 IP（每行一个）" hint="支持精确 IP 与 CIDR；留空允许全部 IP">
            <Textarea rows={4} value={ipsText} onChange={(e) => setIpsText(e.target.value)} placeholder={'127.0.0.1\n192.168.0.0/16'} />
          </Field>
        </div>
        <ErrorText>{create.isError ? (create.error as Error).message : update.isError ? (update.error as Error).message : null}</ErrorText>
        <div className="flex justify-end gap-2 pt-2">
          <Button type="button" variant="outline" onClick={onClose}>取消</Button>
          <Button type="submit" disabled={create.isPending || update.isPending}>
            {create.isPending || update.isPending ? '保存中…' : '保存'}
          </Button>
        </div>
      </form>
    </Dialog>
  )
}

function splitLines(text: string): string[] {
  return text.split(/\r?\n/).map((item) => item.trim()).filter(Boolean)
}

function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 19)
}

import { type FormEvent, useId, useState } from 'react'
import { Check, Copy, KeyRound, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useCreateToken, useDeleteToken, useModelCatalog, useTokens, useUpdateToken } from '../api/hooks'
import type { Token } from '../api/types'
import { Badge, Button, Card, ConfirmDialog, Dialog, Empty, ErrorText, Field, Input, PageHeader, SettingRow, SkeletonRows, StatusBadge, Switch, Td, Textarea, Th } from '../components/ui'
import ModelPicker from '../components/ModelPicker'
import { fmtInt, fmtTime, tokenStatusLabel, tokenStatusTone } from '../lib/format'
import { applyModelSelection } from '../lib/modelSelection'
import { clientEnvSnippet, copyText } from '../lib/clipboard'
import { currentBasePath } from '../lib/runtime'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'

/** 创建成功：明文 key 仅此一次；同时给出可直接粘贴到客户端的环境变量片段。 */
function TokenCreatedDialog({ apiKey, onClose }: { apiKey: string; onClose: () => void }) {
  const baseURL = `${window.location.origin}${currentBasePath() || ''}/v1`
  const snippet = clientEnvSnippet(apiKey, baseURL)
  const [copiedKey, setCopiedKey] = useState<'' | 'ok' | 'fail'>('')
  const [copiedSnippet, setCopiedSnippet] = useState<'' | 'ok' | 'fail'>('')

  const copy = async (text: string, setter: (v: '' | 'ok' | 'fail') => void) => {
    const ok = await copyText(text)
    setter(ok ? 'ok' : 'fail')
    if (ok) setTimeout(() => setter(''), 1500)
  }

  return (
    <Dialog open onClose={onClose} title="令牌已创建">
      <div className="space-y-[var(--eg-control-gap)]">
        <div className="rounded-md bg-warning-subtle px-3 py-2 text-xs text-warning-text" role="alert">
          请立即复制保存——明文 key 仅此一次显示，关闭后无法再次查看。
        </div>
        <div>
          <div className="mb-1.5 text-xs font-medium text-foreground-soft">下游令牌</div>
          <div className="flex items-center gap-2 rounded-lg border border-line bg-surface-soft p-3">
            <code className="flex-1 break-all font-mono text-sm">{apiKey}</code>
            <CopyButton state={copiedKey} onCopy={() => void copy(apiKey, setCopiedKey)} />
          </div>
        </div>
        <div>
          <div className="mb-1.5 flex items-center justify-between text-xs font-medium text-foreground-soft">
            <span>客户端配置片段</span>
            <span className="font-normal text-subtle">OpenAI 兼容 / Anthropic 客户端通用</span>
          </div>
          <div className="relative rounded-lg border border-line bg-surface-soft p-3 pr-12">
            <pre className="max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-[11px] leading-5 text-foreground-soft">{snippet}</pre>
            <div className="absolute right-2 top-2">
              <CopyButton state={copiedSnippet} onCopy={() => void copy(snippet, setCopiedSnippet)} />
            </div>
          </div>
        </div>
        <p className="text-xs text-muted">把 base_url 指向上面的地址并用该令牌作为 API key；Claude Code 使用 ANTHROPIC_*，Codex/Cline/Cursor 使用 OPENAI_* 或自定义 provider。</p>
        <Button className="w-full" onClick={onClose}>我已保存，关闭</Button>
      </div>
    </Dialog>
  )
}

function CopyButton({ state, onCopy }: { state: '' | 'ok' | 'fail'; onCopy: () => void }) {
  return (
    <Button size="sm" variant="outline" onClick={onCopy} aria-live="polite">
      {state === 'ok' ? <Check size={14} className="text-success-text" /> : <Copy size={14} />}
      {state === 'ok' ? '已复制' : state === 'fail' ? '复制失败，请手动选择' : '复制'}
    </Button>
  )
}

export default function Tokens() {
  usePageTitle(t('page.tokens.title'))
  const tokens = useTokens()
  const del = useDeleteToken()
  const [deleteTarget, setDeleteTarget] = useState<Token | null>(null)
  const update = useUpdateToken()
  const [editing, setEditing] = useState<Token | 'new' | null>(null)

  if (tokens.isPending) {
    return (
      <div className="eg-page-stack">
        <PageHeader title={t('page.tokens.title')} description={t('page.tokens.desc')} />
        <Card><SkeletonRows rows={4} /></Card>
      </div>
    )
  }

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.tokens.title')}
        description={t('page.tokens.desc')}
        actions={<Button onClick={() => setEditing('new')}><Plus size={15} /> {t('page.tokens.create')}</Button>}
      />
      <Card>
        {tokens.data?.items.length === 0 ? (
          <Empty icon={<KeyRound size={36} />} title={t('empty.tokens.title')} hint={t('empty.tokens.hint')} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[700px] lg:min-w-0">
              <thead className="border-b border-line-soft bg-surface-soft">
                <tr>
                  <Th>名称</Th><Th>状态</Th><Th>额度</Th><Th>允许模型</Th><Th className="hidden min-[1180px]:table-cell">IP 白名单</Th>
                  <Th>过期时间</Th><Th className="hidden min-[1240px]:table-cell">创建时间</Th><Th>启用</Th>
                  <Th className="w-24"><span className="block pr-2.5 text-right">操作</span></Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {tokens.data?.items.map((t) => (
                  <tr key={t.id} className="hover:bg-surface-soft">
                    <Td className="font-medium">{t.name}</Td>
                    <Td><StatusBadge tone={tokenStatusTone(t.status)}>{tokenStatusLabel[t.status] || t.status}</StatusBadge></Td>
                    <Td>
                      {t.quota > 0 ? (
                        <div>
                          <div>{fmtInt(t.used_quota)} / {fmtInt(t.quota)}</div>
                          <div className={`text-xs ${t.remaining_quota === 0 ? 'text-danger-text' : 'text-subtle'}`}>
                            剩余 {fmtInt(t.remaining_quota)}
                          </div>
                        </div>
                      ) : (
                        <span className="text-subtle">不限</span>
                      )}
                    </Td>
                    <Td className="max-w-40 text-xs text-muted">
                      <div className="truncate" title={(t.allowed_models || []).join(', ')}>
                        {(t.allowed_models || []).join(', ') || '全部模型'}
                      </div>
                      {t.disable_fallback && <Badge tone="warning">禁止降级</Badge>}
                    </Td>
                    <Td className="hidden max-w-40 text-xs text-muted min-[1180px]:table-cell">
                      <div className="truncate" title={(t.allowed_ips || []).join(', ')}>
                        {(t.allowed_ips || []).join(', ') || '全部 IP'}
                      </div>
                    </Td>
                    <Td className="text-xs">{fmtTime(t.expired_at)}</Td>
                    <Td className="hidden text-xs text-subtle min-[1240px]:table-cell">{fmtTime(t.created_at)}</Td>
                    <Td>
                      <Switch checked={t.enabled} onChange={(enabled) => update.mutate({ id: t.id, enabled })} label={`启用令牌 ${t.name}`} />
                    </Td>
                    <Td>
                      <div className="flex justify-end gap-1">
                        <Button size="sm" variant="ghost" aria-label={`编辑令牌 ${t.name}`} title="编辑" onClick={() => setEditing(t)}><Pencil size={13} /></Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          aria-label={`删除令牌 ${t.name}`}
                          title="删除"
                          className="text-danger-text hover:bg-danger-subtle"
                          onClick={() => setDeleteTarget(t)}
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
      <ConfirmDialog
        open={deleteTarget != null}
        title="删除令牌"
        danger
        confirmText="删除令牌"
        loading={del.isPending}
        message={<>将删除令牌「{deleteTarget?.name}」，使用它的所有客户端会立即收到 401。此操作不可撤销；如需继续服务请先创建新令牌并轮换。</>}
        onConfirm={() => deleteTarget && del.mutate(deleteTarget.id, { onSuccess: () => setDeleteTarget(null) })}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function TokenDialog({ token, onClose }: { token: Token | null; onClose: () => void }) {
  const create = useCreateToken()
  const update = useUpdateToken()
  const catalog = useModelCatalog()
  const formID = 'token-form'
  const modelsId = useId()
  const [name, setName] = useState(token?.name ?? '')
  const [quota, setQuota] = useState(String(token?.quota ?? 0))
  const [expiredAt, setExpiredAt] = useState(toLocalInput(token?.expired_at))
  const [modelsText, setModelsText] = useState((token?.allowed_models || []).join('\n'))
  const [ipsText, setIpsText] = useState((token?.allowed_ips || []).join('\n'))
  const [disableFallback, setDisableFallback] = useState(token?.disable_fallback ?? false)
  const [showModelPicker, setShowModelPicker] = useState(false)

  if (create.data) {
    return <TokenCreatedDialog apiKey={create.data.key} onClose={onClose} />
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const payload = {
      name,
      quota: Number(quota) || 0,
      expired_at: expiredAt ? new Date(expiredAt).toISOString() : null,
      allowed_models: splitLines(modelsText),
      allowed_ips: splitLines(ipsText),
      disable_fallback: disableFallback,
    }
    if (token) {
      update.mutate({ id: token.id, ...payload, clear_expired_at: !expiredAt }, { onSuccess: onClose })
    } else {
      create.mutate(payload, { onSuccess: () => undefined })
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={token ? '编辑令牌治理' : '新建令牌'}
      wide
      footer={
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>取消</Button>
          <Button type="submit" form={formID} disabled={create.isPending || update.isPending}>
            {create.isPending || update.isPending ? '保存中…' : '保存'}
          </Button>
        </div>
      }
    >
      <form id={formID} onSubmit={onSubmit} className="space-y-[var(--eg-control-gap)]">
        <div className="eg-grid eg-grid-auto-wide">
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
        <SettingRow
          title="允许模型降级"
          description="关闭后即使配置了降级链，也保持原模型失败语义"
          checked={!disableFallback}
          onChange={(enabled) => setDisableFallback(!enabled)}
          label="允许模型降级"
        />
        <div className="eg-grid eg-grid-auto-wide">
          <div className="space-y-2">
            <div className="flex items-end justify-between gap-2">
              <label htmlFor={modelsId} className="text-sm font-medium text-foreground-soft">允许模型（每行一个）</label>
              <Button type="button" size="sm" variant="outline" onClick={() => setShowModelPicker((current) => !current)}>
                <RefreshCw size={13} /> {showModelPicker ? '收起目录' : '从目录选择'}
              </Button>
            </div>
            <Textarea id={modelsId} rows={4} value={modelsText} onChange={(e) => setModelsText(e.target.value)} placeholder={'gpt-4o\nglm-4.7'} />
            <div className="text-xs text-subtle">留空表示允许全部模型；手动输入时须为对外模型名。</div>
            {showModelPicker && (
              <ModelPicker
                title="对外模型目录"
                options={(catalog.data?.items ?? []).map((item) => ({
                  publicModel: item.model,
                  upstreamModel: item.channels[0]?.upstream_model ?? item.model,
                  context: item.channels.length > 0
                    ? item.channels.map((channel) => `${channel.channel_name}: ${channel.upstream_model}`).join('；')
                    : undefined,
                }))}
                initialModels={splitLines(modelsText)}
                loading={catalog.isPending}
                error={catalog.isError ? (catalog.error as Error).message : undefined}
                onCancel={() => setShowModelPicker(false)}
                onApply={(selected, mode) => {
                  const next = applyModelSelection(splitLines(modelsText), selected, mode)
                  setModelsText(next.join('\n'))
                  setShowModelPicker(false)
                }}
              />
            )}
          </div>
          <Field label="允许 IP（每行一个）" hint="支持精确 IP 与 CIDR；留空允许全部 IP">
            <Textarea rows={4} value={ipsText} onChange={(e) => setIpsText(e.target.value)} placeholder={'127.0.0.1\n192.168.0.0/16'} />
          </Field>
        </div>
        <ErrorText>{create.isError ? (create.error as Error).message : update.isError ? (update.error as Error).message : null}</ErrorText>
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

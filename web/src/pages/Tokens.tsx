import { type FormEvent, useId, useState } from 'react'
import { KeyRound, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useCreateToken, useDeleteToken, useModelCatalog, useTokens, useUpdateToken } from '../api/hooks'
import type { Token } from '../api/types'
import { Alert, Badge, Button, Card, ConfirmDialog, ContentSwap, CopyButton, Dialog, Empty, Field, IconButton, Input, PageHeader, SettingRow, SkeletonRows, StatusBadge, Switch, Td, Textarea, Th } from '../components/ui'
import { notify } from '../components/Toaster'
import ModelPicker from '../components/ModelPicker'
import { fmtInt, fmtTime, tokenStatusLabel, tokenStatusTone } from '../lib/format'
import { applyModelSelection } from '../lib/modelSelection'
import { clientEnvSnippet, modelListSnippets } from '../lib/clipboard'
import { currentBasePath } from '../lib/runtime'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'

/** 创建成功：明文 key 仅此一次；同时给出可直接粘贴到客户端的环境变量片段。 */
function TokenCreatedDialog({ apiKey, onClose }: { apiKey: string; onClose: () => void }) {
  const baseURL = `${window.location.origin}${currentBasePath() || ''}/v1`
  const snippet = clientEnvSnippet(apiKey, baseURL)
  const modelsSnippet = modelListSnippets(baseURL)

  return (
    <Dialog open onClose={onClose} title={t('tokens.created.title')}>
      <div className="space-y-[var(--eg-control-gap)]">
        <Alert tone="warning">{t('tokens.created.once')}</Alert>
        <div>
          <div className="mb-1.5 text-xs font-medium text-foreground-soft">{t('tokens.created.label')}</div>
          <div className="flex items-center gap-2 rounded-lg border border-line bg-surface-soft p-3">
            <code className="flex-1 break-all font-mono text-sm">{apiKey}</code>
            <CopyButton value={apiKey} />
          </div>
        </div>
        <div>
          <div className="mb-1.5 flex items-center justify-between text-xs font-medium text-foreground-soft">
            <span>{t('tokens.created.snippet')}</span>
            <span className="font-normal text-subtle">{t('tokens.created.snippetHint')}</span>
          </div>
          <div className="relative rounded-lg border border-line bg-surface-soft p-3 pr-12">
            <pre className="whitespace-pre-wrap break-all font-mono text-[11px] leading-5 text-foreground-soft">{snippet}</pre>
            <div className="absolute right-2 top-2">
              <CopyButton value={snippet} />
            </div>
          </div>
        </div>
        <div>
          <div className="mb-1.5 flex items-center justify-between text-xs font-medium text-foreground-soft">
            <span>{t('tokens.created.models')}</span>
            <span className="font-normal text-subtle">{t('tokens.created.modelsHint')}</span>
          </div>
          <div className="relative rounded-lg border border-line bg-surface-soft p-3 pr-12">
            <pre className="whitespace-pre-wrap break-all font-mono text-[11px] leading-5 text-foreground-soft">{modelsSnippet}</pre>
            <div className="absolute right-2 top-2">
              <CopyButton value={modelsSnippet} />
            </div>
          </div>
        </div>
        <p className="text-xs text-muted">{t('tokens.created.hint')}</p>
        <Button className="w-full" onClick={onClose}>{t('tokens.created.close')}</Button>
      </div>
    </Dialog>
  )
}

export default function Tokens() {
  usePageTitle(t('page.tokens.title'))
  const tokens = useTokens()
  const del = useDeleteToken()
  const [deleteTarget, setDeleteTarget] = useState<Token | null>(null)
  const update = useUpdateToken()
  const [editing, setEditing] = useState<Token | 'new' | null>(null)

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.tokens.title')}
        description={t('page.tokens.desc')}
        actions={<Button onClick={() => setEditing('new')}><Plus size={15} /> {t('page.tokens.create')}</Button>}
      />
      <Card>
        <ContentSwap loading={tokens.isPending} skeleton={<SkeletonRows rows={4} />}>
          {tokens.data?.items.length === 0 ? (
            <Empty
              icon={<KeyRound size={36} />}
              title={t('empty.tokens.title')}
              hint={t('empty.tokens.hint')}
              action={<Button size="sm" onClick={() => setEditing('new')}><Plus size={14} /> {t('page.tokens.create')}</Button>}
            />
          ) : (
            <div className="overflow-x-auto">
            <table className="w-full min-w-[700px] lg:min-w-0">
              <thead className="border-b border-line-soft bg-surface-soft">
                <tr>
                  <Th>{t('table.name')}</Th><Th>{t('table.status')}</Th><Th>{t('table.quota')}</Th><Th>{t('table.allowedModels')}</Th><Th className="hidden min-[1180px]:table-cell">{t('table.allowedIPs')}</Th>
                  <Th>{t('table.expiresAt')}</Th><Th className="hidden min-[1240px]:table-cell">{t('table.createdAt')}</Th><Th>{t('table.enabled')}</Th>
                  <Th className="w-24"><span className="block pr-2.5 text-right">{t('table.actions')}</span></Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {tokens.data?.items.map((tok) => (
                  <tr key={tok.id} className="hover:bg-surface-soft">
                    <Td className="font-medium">{tok.name}</Td>
        <Td><StatusBadge tone={tokenStatusTone(tok.status)}>{tokenStatusLabel(tok.status)}</StatusBadge></Td>
                    <Td>
                      {tok.quota > 0 ? (
                        <div>
                          <div>{fmtInt(tok.used_quota)} / {fmtInt(tok.quota)}</div>
                          <div className={`text-xs ${tok.remaining_quota === 0 ? 'text-danger-text' : 'text-subtle'}`}>
                            {tf('tokens.remaining', { quota: fmtInt(tok.remaining_quota) })}
                          </div>
                        </div>
                      ) : (
                        <span className="text-subtle">{t('tokens.unlimited')}</span>
                      )}
                    </Td>
                    <Td className="max-w-40 text-xs text-muted">
                      <div className="truncate" title={(tok.allowed_models || []).join(', ')}>
                        {(tok.allowed_models || []).join(', ') || t('tokens.allModels')}
                      </div>
                      {tok.disable_fallback && <Badge tone="warning">{t('tokens.fallbackDisabled')}</Badge>}
                    </Td>
                    <Td className="hidden max-w-40 text-xs text-muted min-[1180px]:table-cell">
                      <div className="truncate" title={(tok.allowed_ips || []).join(', ')}>
                        {(tok.allowed_ips || []).join(', ') || t('tokens.allIPs')}
                      </div>
                    </Td>
                    <Td className="text-xs">{fmtTime(tok.expired_at)}</Td>
                    <Td className="hidden text-xs text-subtle min-[1240px]:table-cell">{fmtTime(tok.created_at)}</Td>
                    <Td>
            <Switch checked={tok.enabled} disabled={update.isPending && update.variables?.id === tok.id} onChange={(enabled) => update.mutate({ id: tok.id, enabled }, { onError: (err) => notify.error(err) })} label={tf('tokens.enableAria', { name: tok.name })} />
                    </Td>
                    <Td>
                      <div className="flex justify-end gap-1">
                        <IconButton label={tf('tokens.editAria', { name: tok.name })} onClick={() => setEditing(tok)}><Pencil size={13} /></IconButton>
                        <IconButton label={tf('tokens.deleteAria', { name: tok.name })} danger onClick={() => setDeleteTarget(tok)}><Trash2 size={13} /></IconButton>
                      </div>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
            </div>
          )}
        </ContentSwap>
      </Card>
      {editing && <TokenDialog token={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog
        open={deleteTarget != null}
        title={t('tokens.delete.title')}
        danger
        confirmText={t('tokens.delete.title')}
        loading={del.isPending}
        message={tf('tokens.delete.message', { name: deleteTarget?.name ?? '' })}
        onConfirm={() => deleteTarget && del.mutate(deleteTarget.id, {
          onSuccess: () => {
            notify.success(tf('toast.token.deleted', { name: deleteTarget.name }))
            setDeleteTarget(null)
          },
          onError: (err) => notify.error(err),
        })}
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
      update.mutate({ id: token.id, ...payload, clear_expired_at: !expiredAt }, {
        onSuccess: () => {
          notify.success(t('toast.token.updated'))
          onClose()
        },
      })
    } else {
      create.mutate(payload, { onSuccess: () => undefined })
    }
  }

  return (
    <Dialog
      open
      onClose={onClose}
      title={token ? t('tokens.edit.title') : t('page.tokens.create')}
      wide
      footer={
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose}>{t('common.cancel')}</Button>
          <Button type="submit" form={formID} loading={create.isPending || update.isPending}>
            {t('common.save')}
          </Button>
        </div>
      }
    >
      <form id={formID} onSubmit={onSubmit} className="space-y-[var(--eg-control-gap)]">
        {(create.isError || update.isError) && (
          <Alert tone="danger">{((create.isError ? create.error : update.error) as Error).message}</Alert>
        )}
        <div className="eg-grid eg-grid-auto-wide">
          <Field label={t('tokens.form.name')} hint={t('tokens.form.nameHint')}>
            <Input value={name} onChange={(e) => setName(e.target.value)} required placeholder="my-token" />
          </Field>
          <Field label={t('tokens.form.quota')} hint={t('tokens.form.quotaHint')}>
            <Input type="number" min={0} value={quota} onChange={(e) => setQuota(e.target.value)} />
          </Field>
        </div>
        <Field label={t('tokens.form.expiresAt')} hint={t('tokens.form.expiresHint')}>
          <Input type="datetime-local" value={expiredAt} onChange={(e) => setExpiredAt(e.target.value)} />
        </Field>
        <SettingRow
          title={t('tokens.form.fallback')}
          description={t('tokens.form.fallbackDesc')}
          checked={!disableFallback}
          onChange={(enabled) => setDisableFallback(!enabled)}
          label={t('tokens.form.fallback')}
        />
        <div className="eg-grid eg-grid-auto-wide">
          <div className="space-y-2">
            <div className="flex items-end justify-between gap-2">
              <label htmlFor={modelsId} className="text-sm font-medium text-foreground-soft">{t('tokens.form.models')}</label>
              <Button type="button" size="sm" variant="outline" onClick={() => setShowModelPicker((current) => !current)}>
                <RefreshCw size={13} /> {showModelPicker ? t('tokens.form.catalogClose') : t('tokens.form.catalogOpen')}
              </Button>
            </div>
            <Textarea id={modelsId} rows={4} value={modelsText} onChange={(e) => setModelsText(e.target.value)} placeholder={'gpt-4o\nglm-4.7'} />
            <div className="text-xs text-subtle">{t('tokens.form.modelsHint')}</div>
          </div>
          <Field label={t('tokens.form.ips')} hint={t('tokens.form.ipsHint')}>
            <Textarea rows={4} value={ipsText} onChange={(e) => setIpsText(e.target.value)} placeholder={'127.0.0.1\n192.168.0.0/16'} />
          </Field>
        </div>
        {/* 目录选择器横跨弹窗全宽：挤在左半栏时每行长说明折成三行、还要在弹窗里再套一层内滚动 */}
        {showModelPicker && (
          <ModelPicker
            title={t('tokens.form.catalogTitle')}
            options={(catalog.data?.items ?? []).map((item) => ({
              publicModel: item.model,
              upstreamModel: item.channels[0]?.upstream_model ?? item.model,
              context: item.channels.length > 0
                ? item.channels.map((channel) => `${channel.channel_name}: ${channel.upstream_model}`).join('; ')
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

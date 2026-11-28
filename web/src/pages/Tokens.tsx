import { useId, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { KeyRound, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { useCreateToken, useDeleteToken, useModelCatalog, useTokens, useUpdateToken, type TokenGovernanceInput } from '../api/hooks'
import type { Token } from '../api/types'
import { Alert, Badge, Button, Card, ConfirmDialog, ContentSwap, CopyButton, DataTable, type DataTableColumn, Dialog, Empty, Field, IconButton, Input, LoadError, PageHeader, SettingRow, SkeletonRows, StatusBadge, Switch, Textarea } from '../components/ui'
import { notify } from '../components/Toaster'
import ModelPicker from '../components/ModelPicker'
import { fmtInt, fmtTime, tokenStatusLabel, tokenStatusTone } from '../lib/format'
import { applyModelSelection } from '../lib/modelSelection'
import { clientEnvSnippet, modelListSnippets } from '../lib/clipboard'
import { currentBasePath } from '../lib/runtime'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'
import { errorMessage } from '../lib/errors'
import { useUnsavedGuard } from '../lib/useUnsavedGuard'

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
        <p className="text-xs text-muted-foreground">{t('tokens.created.hint')}</p>
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
  const columns: DataTableColumn<Token>[] = [
      { id: 'name', header: t('table.name'), cell: (tok) => <span className="font-medium">{tok.name}</span>, sortKey: (tok) => tok.name },
      { id: 'status', header: t('table.status'), cell: (tok) => <StatusBadge tone={tokenStatusTone(tok.status)}>{tokenStatusLabel(tok.status)}</StatusBadge>, sortKey: (tok) => tok.status },
      {
        id: 'quota',
        header: t('table.quota'),
        sortKey: (tok) => tok.quota,
        cell: (tok) =>
          tok.quota > 0 ? (
            <div>
              <div>{fmtInt(tok.used_quota)} / {fmtInt(tok.quota)}</div>
              <div className={`text-xs ${tok.remaining_quota === 0 ? 'text-danger-text' : 'text-subtle'}`}>
                {tf('tokens.remaining', { quota: fmtInt(tok.remaining_quota) })}
              </div>
            </div>
          ) : (
            <span className="text-subtle">{t('tokens.unlimited')}</span>
          ),
      },
      {
        id: 'rateLimit',
        header: t('table.rateLimit'),
        className: 'hidden text-xs text-muted-foreground min-[1180px]:table-cell',
        hideable: true,
        cell: (tok) =>
          tok.rpm_limit > 0 || tok.concurrency_limit > 0
            ? tf('tokens.rateLimitSummary', {
              rpm: tok.rpm_limit > 0 ? fmtInt(tok.rpm_limit) : '∞',
              concurrency: tok.concurrency_limit > 0 ? fmtInt(tok.concurrency_limit) : '∞',
            })
            : <span className="text-subtle">{t('tokens.unlimited')}</span>,
      },
      {
        id: 'models',
        header: t('table.allowedModels'),
        className: 'max-w-40 text-xs text-muted-foreground',
        cell: (tok) => (
          <>
            <div className="truncate" title={(tok.allowed_models || []).join(', ')}>
              {(tok.allowed_models || []).join(', ') || t('tokens.allModels')}
            </div>
            {tok.disable_fallback && <Badge tone="warning">{t('tokens.fallbackDisabled')}</Badge>}
          </>
        ),
      },
      {
        id: 'ips',
        header: t('table.allowedIPs'),
        className: 'hidden max-w-40 text-xs text-muted-foreground min-[1320px]:table-cell',
        hideable: true,
        cell: (tok) => <div className="truncate" title={(tok.allowed_ips || []).join(', ')}>{(tok.allowed_ips || []).join(', ') || t('tokens.allIPs')}</div>,
      },
      { id: 'expires', header: t('table.expiresAt'), className: 'text-xs', cell: (tok) => fmtTime(tok.expired_at), sortKey: (tok) => tok.expired_at ?? '' },
      { id: 'created', header: t('table.createdAt'), className: 'hidden text-xs text-subtle min-[1400px]:table-cell', cell: (tok) => fmtTime(tok.created_at), sortKey: (tok) => tok.created_at },
      {
        id: 'enabled',
        header: t('table.enabled'),
        cell: (tok) => (
          <Switch checked={tok.enabled} disabled={update.isPending && update.variables?.id === tok.id} onChange={(enabled) => update.mutate({ id: tok.id, enabled })} label={tf('tokens.enableAria', { name: tok.name })} />
        ),
      },
      {
        id: 'actions',
        header: <span className="block pr-2.5 text-right">{t('table.actions')}</span>,
        headerClassName: 'w-24',
        cell: (tok) => (
          <div className="flex justify-end gap-1">
            <IconButton label={tf('tokens.editAria', { name: tok.name })} onClick={() => setEditing(tok)}><Pencil size={13} /></IconButton>
            <IconButton label={tf('tokens.deleteAria', { name: tok.name })} danger onClick={() => setDeleteTarget(tok)}><Trash2 size={13} /></IconButton>
          </div>
        ),
      },
  ]

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={t('page.tokens.title')}
        description={t('page.tokens.desc')}
        actions={<Button onClick={() => setEditing('new')}><Plus size={15} /> {t('page.tokens.create')}</Button>}
      />
      <Card>
        <ContentSwap loading={tokens.isPending} skeleton={<SkeletonRows rows={4} />}>
          {tokens.isError ? (
            <LoadError error={tokens.error} onRetry={() => void tokens.refetch()} />
          ) : tokens.data?.items.length === 0 ? (
            <Empty
              icon={<KeyRound size={36} />}
              title={t('empty.tokens.title')}
              hint={t('empty.tokens.hint')}
              action={<Button size="sm" onClick={() => setEditing('new')}><Plus size={14} /> {t('page.tokens.create')}</Button>}
            />
          ) : (
                        <DataTable columns={columns} rows={tokens.data?.items ?? []} rowKey={(tok) => tok.id} minWidth={700} className="lg:min-w-0" />
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
          }
        })}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}

function TokenDialog({ token, onClose }: { token: Token | null; onClose: () => void }) {
  const create = useCreateToken()
  const update = useUpdateToken({ silentError: true })
  const catalog = useModelCatalog()
  const formID = 'token-form'
  const modelsId = useId()
  // schema 即校验规则与入参类型；文本域字段保留原始字符串，提交时再拆行
  const schema = z.object({
    name: z.string().trim().min(1, t('tokens.form.nameRequired')),
    quota: z.string().refine((value) => value === '' || (!Number.isNaN(Number(value)) && Number(value) >= 0), t('tokens.form.quotaInvalid')),
    rpm_limit: z.string().refine((value) => value === '' || (/^\d+$/.test(value) && Number(value) >= 0), t('tokens.form.rpmInvalid')),
    concurrency_limit: z.string().refine((value) => value === '' || (/^\d+$/.test(value) && Number(value) >= 0), t('tokens.form.concurrencyInvalid')),
    expired_at: z.string(),
    modelsText: z.string(),
    ipsText: z.string(),
    disable_fallback: z.boolean(),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: token?.name ?? '',
      quota: String(token?.quota ?? 0),
      rpm_limit: String(token?.rpm_limit ?? 0),
      concurrency_limit: String(token?.concurrency_limit ?? 0),
      expired_at: toLocalInput(token?.expired_at),
      modelsText: (token?.allowed_models || []).join('\n'),
      ipsText: (token?.allowed_ips || []).join('\n'),
      disable_fallback: token?.disable_fallback ?? false,
    },
  })
  const { register, handleSubmit, formState, setValue, watch } = form
  const [showModelPicker, setShowModelPicker] = useState(false)
  // 有未保存更改时关闭需确认（Esc / 遮罩 / 取消按钮都走这里）
  const guard = useUnsavedGuard(formState.isDirty)
  const close = guard.guard(onClose)

  if (create.data) {
    return <TokenCreatedDialog apiKey={create.data.key} onClose={onClose} />
  }

  const onSubmit = handleSubmit((values) => {
    const payload: TokenGovernanceInput = {
      name: values.name.trim(),
      quota: Number(values.quota) || 0,
      rpm_limit: Number(values.rpm_limit) || 0,
      concurrency_limit: Number(values.concurrency_limit) || 0,
      expired_at: values.expired_at ? new Date(values.expired_at).toISOString() : null,
      allowed_models: splitLines(values.modelsText),
      allowed_ips: splitLines(values.ipsText),
      disable_fallback: values.disable_fallback,
    }
    if (token) {
      update.mutate({ id: token.id, ...payload, clear_expired_at: !values.expired_at }, {
        onSuccess: () => {
          notify.success(t('toast.token.updated'))
          onClose()
        },
      })
    } else {
      create.mutate(payload, { onSuccess: () => undefined })
    }
  })

  return (
    <Dialog
      open
      onClose={close}
      title={token ? t('tokens.edit.title') : t('page.tokens.create')}
      wide
      footer={
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={close}>{t('common.cancel')}</Button>
          <Button type="submit" form={formID} loading={create.isPending || update.isPending}>
            {t('common.save')}
          </Button>
        </div>
      }
    >
      <form id={formID} onSubmit={onSubmit} className="space-y-[var(--eg-control-gap)]">
        {(create.isError || update.isError) && (
          <Alert tone="danger">{errorMessage(create.isError ? create.error : update.error)}</Alert>
        )}
        <div className="eg-grid eg-grid-auto-wide">
          <Field label={t('tokens.form.name')} hint={t('tokens.form.nameHint')} error={formState.errors.name?.message}>
            <Input {...register('name')} placeholder="my-token" />
          </Field>
          <Field label={t('tokens.form.quota')} hint={t('tokens.form.quotaHint')} error={formState.errors.quota?.message}>
            <Input type="number" min={0} {...register('quota')} />
          </Field>
        </div>
        <div className="eg-grid eg-grid-auto-wide">
          <Field label={t('tokens.form.rpm')} hint={t('tokens.form.rpmHint')} error={formState.errors.rpm_limit?.message}>
            <Input type="number" min={0} {...register('rpm_limit')} />
          </Field>
          <Field label={t('tokens.form.concurrency')} hint={t('tokens.form.concurrencyHint')} error={formState.errors.concurrency_limit?.message}>
            <Input type="number" min={0} {...register('concurrency_limit')} />
          </Field>
        </div>
        <Field label={t('tokens.form.expiresAt')} hint={t('tokens.form.expiresHint')}>
          <Input type="datetime-local" {...register('expired_at')} />
        </Field>
        <SettingRow
          title={t('tokens.form.fallback')}
          description={t('tokens.form.fallbackDesc')}
          checked={!watch('disable_fallback')}
          onChange={(enabled) => setValue('disable_fallback', !enabled, { shouldDirty: true })}
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
            <Textarea id={modelsId} rows={4} {...register('modelsText')} placeholder={'gpt-4o\nglm-4.7'} />
            <div className="text-xs text-subtle">{t('tokens.form.modelsHint')}</div>
          </div>
          <Field label={t('tokens.form.ips')} hint={t('tokens.form.ipsHint')}>
            <Textarea rows={4} {...register('ipsText')} placeholder={'127.0.0.1\n192.168.0.0/16'} />
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
            initialModels={splitLines(watch('modelsText'))}
            loading={catalog.isPending}
            error={catalog.isError ? errorMessage(catalog.error) : undefined}
            onCancel={() => setShowModelPicker(false)}
            onApply={(selected, mode) => {
              const next = applyModelSelection(splitLines(watch('modelsText')), selected, mode)
              setValue('modelsText', next.join('\n'), { shouldDirty: true })
              setShowModelPicker(false)
            }}
          />
        )}
      </form>
      {guard.dialog}
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

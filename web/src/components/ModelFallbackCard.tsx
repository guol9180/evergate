import { useEffect, useState } from 'react'
import { GitBranch, Plus, Save, Trash2 } from 'lucide-react'
import { useModelFallbacks, useSaveModelFallbacks } from '../api/hooks'
import type { ModelFallback } from '../api/types'
import { Button, Card, ErrorText, Field, IconButton, Input, Spinner, Textarea } from './ui'
import { notify } from './Toaster'
import { t, tf } from '../lib/i18n'

type FallbackForm = {
  model: string
  budgetThreshold: string
  fallbacks: string
}

export default function ModelFallbackCard() {
  const fallbacks = useModelFallbacks()
  const save = useSaveModelFallbacks()
  const [rows, setRows] = useState<FallbackForm[] | null>(null)

  useEffect(() => {
    if (fallbacks.data?.items && rows === null) {
      setRows(fallbacks.data.items.map(toForm))
    }
  }, [fallbacks.data, rows])

  if (fallbacks.isPending) {
    return <Card className="flex justify-center"><Spinner className="h-5 w-5" /></Card>
  }
  if (fallbacks.isError || rows === null) {
    return (
      <Card>
        <h2 className="mb-2 text-sm font-semibold">{t('fallback.title')}</h2>
        <ErrorText>{(fallbacks.error as Error)?.message || t('fallback.unavailable')}</ErrorText>
      </Card>
    )
  }

  const update = (index: number, patch: Partial<FallbackForm>) => {
    setRows(rows.map((row, i) => i === index ? { ...row, ...patch } : row))
  }

  const submit = () => save.mutate(
    rows.map(toPayload).filter(item => item.model.trim() || item.fallbacks.length),
    { onSuccess: () => notify.success(t('fallback.saved')), onError: (err) => notify.error(err) },
  )

  return (
    <Card>
      <div className="eg-control-row mb-4 flex flex-wrap items-start justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold"><GitBranch size={15} className="text-brand-text" /> {t('fallback.title')}</h2>
          <p className="mt-1 text-xs text-muted">{t('fallback.desc')}</p>
        </div>
        <Button type="button" size="sm" variant="outline" onClick={() => setRows([...rows, { model: '', budgetThreshold: '0', fallbacks: '' }])}>
          <Plus size={14} /> {t('fallback.add')}
        </Button>
      </div>

      {rows.length === 0 ? (
        <div className="rounded-lg border border-dashed border-line-strong px-3 py-6 text-center text-xs text-muted">
          {t('fallback.empty')}
        </div>
      ) : (
        <div className="space-y-[var(--eg-control-gap)]">
          {rows.map((row, index) => (
            <div key={index} className="eg-grid rounded-lg border border-line p-3 min-[1100px]:grid-cols-[minmax(0,1fr)_150px_minmax(0,1.5fr)_40px] min-[1100px]:items-start">
              <Field label={t('fallback.sourceModel')}>
                <Input value={row.model} onChange={(e) => update(index, { model: e.target.value })} placeholder="gpt-4o" />
              </Field>
              <Field label={t('fallback.budget')} hint={t('fallback.budgetHint')}>
                <Input type="number" min={0} value={row.budgetThreshold} onChange={(e) => update(index, { budgetThreshold: e.target.value })} />
              </Field>
              <Field label={t('fallback.candidates')}>
                <Textarea rows={2} value={row.fallbacks} onChange={(e) => update(index, { fallbacks: e.target.value })} placeholder={'gpt-4o-mini\ngpt-4o-nano'} />
              </Field>
              <div className="flex items-center justify-end md:pt-7">
                <IconButton
                  label={tf('fallback.deleteAria', { name: row.model || index + 1 })}
                  danger
                  onClick={() => setRows(rows.filter((_, i) => i !== index))}
                >
                  <Trash2 size={14} />
                </IconButton>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="mt-4 border-t border-line-soft pt-4">
        <div className="flex justify-end">
          <Button type="button" onClick={submit} disabled={save.isPending}>
            <Save size={15} /> {save.isPending ? t('common.saving') : t('fallback.save')}
          </Button>
        </div>
      </div>
    </Card>
  )
}

function toForm(item: ModelFallback): FallbackForm {
  return {
    model: item.model,
    budgetThreshold: String(item.budget_threshold),
    fallbacks: (item.fallbacks || []).join('\n'),
  }
}

function toPayload(item: FallbackForm): ModelFallback {
  return {
    model: item.model.trim(),
    budget_threshold: Math.max(0, Math.round(Number(item.budgetThreshold) || 0)),
    fallbacks: item.fallbacks.split(/\r?\n/).map(value => value.trim()).filter(Boolean),
  }
}

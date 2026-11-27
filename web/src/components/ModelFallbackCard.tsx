import { zodResolver } from '@hookform/resolvers/zod'
import { useFieldArray, useForm } from 'react-hook-form'
import { z } from 'zod'
import { GitBranch, Plus, Save, Trash2 } from 'lucide-react'
import { useModelFallbacks, useSaveModelFallbacks } from '../api/hooks'
import type { ModelFallback } from '../api/types'
import { Button, Card, ErrorText, Field, IconButton, Input, Spinner, Textarea } from './ui'
import { notify } from './Toaster'
import { errorMessage } from '../lib/errors'
import { t, tf } from '../lib/i18n'

// 降级链是行数组编辑：useFieldArray 负责增删与 key，schema 校验每行预算阈值为非负整数
const schema = z.object({
  rows: z.array(z.object({
    model: z.string(),
    budgetThreshold: z.string().refine((value) => value === '' || (/^\d+$/.test(value) && Number.isSafeInteger(Number(value))), t('fallback.budgetInvalid')),
    fallbacks: z.string(),
  })),
})
type FormValues = z.infer<typeof schema>

export default function ModelFallbackCard() {
  const fallbacks = useModelFallbacks()
  const save = useSaveModelFallbacks()
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    values: fallbacks.data?.items ? { rows: fallbacks.data.items.map(toForm) } : undefined,
  })
  const { register, handleSubmit, formState, setFocus } = form
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'rows' })

  if (fallbacks.isPending) {
    return <Card className="flex justify-center"><Spinner className="h-5 w-5" /></Card>
  }
  if (fallbacks.isError) {
    return (
      <Card>
        <h2 className="mb-2 text-sm font-semibold">{t('fallback.title')}</h2>
        <ErrorText>{errorMessage(fallbacks.error) || t('fallback.unavailable')}</ErrorText>
      </Card>
    )
  }

  // 空行（源模型与候选都没填）直接丢弃，不发给服务端
  const submit = handleSubmit(
    (values) => save.mutate(
      values.rows.map(toPayload).filter((item) => item.model || item.fallbacks.length),
      { onSuccess: () => notify.success(t('fallback.saved')) },
    ),
    (errors) => {
      const index = Array.isArray(errors.rows) ? errors.rows.findIndex((row) => row != null && 'budgetThreshold' in row) : -1
      if (index >= 0) setFocus(`rows.${index}.budgetThreshold`)
    },
  )

  return (
    <Card>
      <div className="eg-control-row mb-4 flex flex-wrap items-start justify-between">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold"><GitBranch size={15} className="text-brand-text" /> {t('fallback.title')}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{t('fallback.desc')}</p>
        </div>
        <Button type="button" size="sm" variant="outline" onClick={() => append({ model: '', budgetThreshold: '0', fallbacks: '' })}>
          <Plus size={14} /> {t('fallback.add')}
        </Button>
      </div>

      {fields.length === 0 ? (
        <div className="rounded-lg border border-dashed border-line-strong px-3 py-6 text-center text-xs text-muted-foreground">
          {t('fallback.empty')}
        </div>
      ) : (
        <div className="space-y-[var(--eg-control-gap)]">
          {fields.map((field, index) => (
            <div key={field.id} className="eg-grid rounded-lg border border-line p-3 min-[1100px]:grid-cols-[minmax(0,1fr)_150px_minmax(0,1.5fr)_40px] min-[1100px]:items-start">
              <Field label={t('fallback.sourceModel')}>
                <Input {...register(`rows.${index}.model`)} placeholder="gpt-4o" />
              </Field>
              <Field label={t('fallback.budget')} hint={t('fallback.budgetHint')} error={formState.errors.rows?.[index]?.budgetThreshold?.message}>
                <Input type="number" min={0} {...register(`rows.${index}.budgetThreshold`)} />
              </Field>
              <Field label={t('fallback.candidates')}>
                <Textarea rows={2} {...register(`rows.${index}.fallbacks`)} placeholder={'gpt-4o-mini\ngpt-4o-nano'} />
              </Field>
              <div className="flex items-center justify-end md:pt-7">
                <IconButton label={tf('fallback.deleteAria', { name: field.model || index + 1 })} danger onClick={() => remove(index)}>
                  <Trash2 size={14} />
                </IconButton>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="mt-4 border-t border-line-soft pt-4">
        <div className="flex justify-end">
          <Button type="button" onClick={submit} loading={save.isPending}>
            <Save size={15} /> {t('fallback.save')}
          </Button>
        </div>
      </div>
    </Card>
  )
}

function toForm(item: ModelFallback): FormValues['rows'][number] {
  return {
    model: item.model,
    budgetThreshold: String(item.budget_threshold),
    fallbacks: (item.fallbacks || []).join('\n'),
  }
}

function toPayload(item: FormValues['rows'][number]): ModelFallback {
  return {
    model: item.model.trim(),
    budget_threshold: Math.max(0, Math.round(Number(item.budgetThreshold) || 0)),
    fallbacks: item.fallbacks.split(/\r?\n/).map(value => value.trim()).filter(Boolean),
  }
}
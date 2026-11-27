import { useMemo, useState } from 'react'
import { Search } from 'lucide-react'
import { Badge, Button, Checkbox, ErrorText, Input, SegmentedControl, Spinner } from './ui'
import { t, tf } from '../lib/i18n'
import { priceHint } from './PriceTable'

export interface ModelPickerOption {
  publicModel: string
  upstreamModel: string
  context?: string
  /** 上游目录声明的价格（USD / 百万 token，输入 / 输出）；有则可一并写入价格覆盖 */
  price?: { input_micros_per_million: number; output_micros_per_million: number }
}

interface ModelPickerProps {
  title: string
  options: ModelPickerOption[]
  initialModels: string[]
  loading?: boolean
  error?: string
  truncated?: boolean
  protocol?: string
  onApply: (models: string[], mode: 'append' | 'replace', withPrices: boolean) => void
  onCancel: () => void
}

export default function ModelPicker({
  title,
  options,
  initialModels,
  loading = false,
  error,
  truncated = false,
  protocol,
  onApply,
  onCancel,
}: ModelPickerProps) {
  const [query, setQuery] = useState('')
  const [mode, setMode] = useState<'append' | 'replace'>('append')
  const [withPrices, setWithPrices] = useState(true)
  const hasPrices = options.some((option) => option.price != null)
  const [selected, setSelected] = useState(() => {
    const available = new Set(options.map((option) => option.publicModel))
    return new Set(initialModels.filter((model) => available.has(model)))
  })

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return options
    return options.filter((option) =>
      `${option.publicModel} ${option.upstreamModel} ${option.context ?? ''}`.toLowerCase().includes(needle),
    )
  }, [options, query])

  function toggle(model: string) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(model)) next.delete(model)
      else next.add(model)
      return next
    })
  }

  return (
    <div className="rounded-lg border border-brand-border bg-brand-subtle p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <div className="text-sm font-medium text-foreground-soft">{title}</div>
          <div className="mt-0.5 text-xs text-muted-foreground">{t('picker.hint')}</div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {protocol && <Badge tone="brand">{protocol}</Badge>}
          <Badge tone="neutral">{tf('picker.count', { n: options.length })}</Badge>
          <Badge tone="success">{tf('picker.selected', { n: selected.size })}</Badge>
        </div>
      </div>

      {options.length > 0 && (
        <div className="relative mt-3">
          <Search size={14} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-subtle" />
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="pl-8"
            placeholder={t('picker.search')}
            aria-label={tf('picker.searchAria', { title })}
            disabled={loading}
          />
        </div>
      )}

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground"><Spinner /> {t('picker.loading')}</div>
      ) : error ? (
        <div className="mt-3"><ErrorText>{error}</ErrorText></div>
      ) : visible.length === 0 ? (
        <div className="py-8 text-center text-sm text-subtle">
          {options.length === 0 ? t('picker.emptyUpstream') : t('picker.noMatch')}
        </div>
      ) : (
        <div className="mt-3 max-h-64 overflow-y-auto rounded-md border border-line bg-surface">
          {visible.map((option, index) => {
            const checked = selected.has(option.publicModel)
            const mapped = option.publicModel !== option.upstreamModel
            return (
              <label
                key={`${option.publicModel}|${option.upstreamModel}|${option.context ?? ''}|${index}`}
                className="flex cursor-pointer items-start gap-2.5 border-b border-line-soft px-3 py-2.5 last:border-b-0 hover:bg-surface-soft"
              >
                <Checkbox
                  checked={checked}
                  onCheckedChange={() => toggle(option.publicModel)}
                  aria-label={option.publicModel}
                  className="mt-0.5"
                />
                <span className="min-w-0 flex-1">
                  <span className="block break-all text-sm font-medium text-foreground-soft">{option.publicModel}</span>
                  <span className={`mt-0.5 block break-all text-xs ${mapped ? 'text-brand-text' : 'text-subtle'}`}>
                    {mapped ? `${option.publicModel} → ${option.upstreamModel}` : tf('picker.upstreamSame', { model: option.upstreamModel })}
                    {option.context && <span className="text-subtle"> · {option.context}</span>}
                    {option.price && <span className="text-success-text"> · {priceHint(option.price.input_micros_per_million, option.price.output_micros_per_million)}</span>}
                  </span>
                </span>
              </label>
            )
          })}
        </div>
      )}

      {truncated && (
        <div className="mt-2 text-xs text-warning-text">{t('picker.truncated')}</div>
      )}

      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>{t('picker.writeMode')}</span>
          <SegmentedControl
            size="sm"
            className="w-52 grid-cols-2"
            label={t('picker.writeMode')}
            value={mode}
            onChange={setMode}
            options={[
              { value: 'append', label: t('picker.append') },
              { value: 'replace', label: t('picker.replace') },
            ]}
          />
          {hasPrices && (
            <label className="inline-flex cursor-pointer items-center gap-1.5 text-success-text">
              <Checkbox checked={withPrices} onCheckedChange={(next) => setWithPrices(next === true)} aria-label={t('picker.writePrices')} />
              {t('picker.writePrices')}
            </label>
          )}
        </div>
        <div className="flex gap-2">
          {/* 内联选择器嵌在表单里，叫"取消"会和弹窗自己的取消按钮同名；它只是收起，不丢弃已填内容 */}
          <Button type="button" variant="outline" onClick={onCancel}>{t('picker.collapse')}</Button>
          <Button type="button" disabled={selected.size === 0 || loading || Boolean(error)} onClick={() => onApply([...selected].sort(), mode, hasPrices && withPrices)}>
            {t('picker.confirm')}
          </Button>
        </div>
      </div>
    </div>
  )
}

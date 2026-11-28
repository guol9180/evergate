import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { defaultFilter, useCommandState } from 'cmdk'
import { cn } from 'cn'
import { Badge, Button, Checkbox, Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList, ErrorText, Popover, PopoverAnchor, PopoverContent, SegmentedControl, Spinner } from './ui'
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
  /** 弹层锚点：触发按钮（弹层对齐到它下方） */
  trigger: ReactNode
  open: boolean
  onOpenChange: (open: boolean) => void
  options: ModelPickerOption[]
  /** 预选：当前表单里已有的模型；目录晚于打开到达时也会按它重算 */
  initialModels: string[]
  loading?: boolean
  error?: string
  truncated?: boolean
  protocol?: string
  onApply: (models: string[], mode: 'append' | 'replace', withPrices: boolean) => void
}

/**
 * 模型多选（Popover + cmdk Command）：搜索、↑/↓/Home/End 高亮、Enter 切换由 cmdk 负责；
 * 选中态 = 预选 ∩ 目录，再叠加本次打开后的手动切换，所以目录晚到也不需要补丁式回填。
 */
export default function ModelPicker({
  title,
  trigger,
  open,
  onOpenChange,
  options,
  initialModels,
  loading = false,
  error,
  truncated = false,
  protocol,
  onApply,
}: ModelPickerProps) {
  const [mode, setMode] = useState<'append' | 'replace'>('append')
  const [withPrices, setWithPrices] = useState(true)
  const [overrides, setOverrides] = useState<Map<string, boolean>>(() => new Map())
  const hasPrices = options.some((option) => option.price != null)
  // 选中态以 publicModel 为键：同名重复行只会表达同一个状态，这里直接去重
  const items = useMemo(() => {
    const seen = new Set<string>()
    return options.filter((option) => {
      if (seen.has(option.publicModel)) return false
      seen.add(option.publicModel)
      return true
    })
  }, [options])
  // 调用点每次渲染都传新数组（splitLines 的结果），按内容键缓存，避免 useMemo 失效
  const initialKey = initialModels.join('\n')
  const initial = useMemo(() => new Set(initialKey.split('\n').filter(Boolean)), [initialKey])
  const selected = useMemo(() => {
    const next = new Set<string>()
    for (const item of items) {
      if (overrides.get(item.publicModel) ?? initial.has(item.publicModel)) next.add(item.publicModel)
    }
    return next
  }, [items, overrides, initial])
  // 关闭即丢弃本次的手动切换：下次打开按当前表单重新预选
  useEffect(() => {
    if (!open) setOverrides(new Map())
  }, [open])

  const toggle = (model: string) => {
    const next = !selected.has(model)
    setOverrides((current) => new Map(current).set(model, next))
  }
  const toggleMany = (models: string[], force: boolean) => {
    setOverrides((current) => {
      const next = new Map(current)
      for (const model of models) next.set(model, force)
      return next
    })
  }

  const list =
    loading ? (
      <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground"><Spinner /> {t('picker.loading')}</div>
    ) : error ? (
      <div className="px-3 py-2"><ErrorText>{error}</ErrorText></div>
    ) : items.length === 0 ? (
      <div className="py-8 text-center text-sm text-subtle">{t('picker.emptyUpstream')}</div>
    ) : (
      <Command label={title} loop className="mt-2 bg-transparent">
        <CommandInput placeholder={t('picker.search')} aria-label={tf('picker.searchAria', { title })} />
        <SelectBar items={items} selected={selected} onToggleMany={toggleMany} />
        <CommandList className="max-h-64">
          <CommandEmpty className="py-8 text-subtle">{t('picker.noMatch')}</CommandEmpty>
          <CommandGroup>
            {items.map((item) => {
              const checked = selected.has(item.publicModel)
              const mapped = item.publicModel !== item.upstreamModel
              return (
                <CommandItem
                  key={item.publicModel}
                  value={item.publicModel}
                  keywords={[item.upstreamModel, item.context ?? '']}
                  onSelect={() => toggle(item.publicModel)}
                  aria-checked={checked}
                  className="items-start gap-2.5 py-2"
                >
                  {/* 纯视觉指示：点击落到整行（由 cmdk 处理），状态由 option 的 aria-checked 表达 */}
                  <Checkbox checked={checked} tabIndex={-1} aria-hidden className="pointer-events-none mt-0.5" />
                  <span className="min-w-0 flex-1">
                    <span className="block break-all font-medium text-foreground-soft">{item.publicModel}</span>
                    <span className={cn('mt-0.5 block break-all text-xs', mapped ? 'text-brand-text' : 'text-subtle')}>
                      {mapped ? `${item.publicModel} → ${item.upstreamModel}` : tf('picker.upstreamSame', { model: item.upstreamModel })}
                      {item.context && <span className="text-subtle"> · {item.context}</span>}
                      {item.price && <span className="text-success-text"> · {priceHint(item.price.input_micros_per_million, item.price.output_micros_per_million)}</span>}
                    </span>
                  </span>
                </CommandItem>
              )
            })}
          </CommandGroup>
        </CommandList>
      </Command>
    )

  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverAnchor asChild>{trigger}</PopoverAnchor>
      <PopoverContent align="end" sideOffset={6} collisionPadding={8} className="z-[60] w-[min(560px,var(--radix-popover-content-available-width))] max-h-[var(--radix-popover-content-available-height)] gap-0 overflow-y-auto p-0">
        <div className="flex flex-wrap items-start justify-between gap-2 px-3 pt-3">
          <div>
            <div className="text-sm font-medium text-foreground-soft">{title}</div>
            <div className="mt-0.5 text-xs text-muted-foreground">{t('picker.hint')}</div>
          </div>
          <div className="flex flex-wrap items-center gap-1.5">
            {protocol && <Badge tone="brand">{protocol}</Badge>}
            <Badge tone="neutral">{tf('picker.count', { n: items.length })}</Badge>
            <Badge tone="success">{tf('picker.selected', { n: selected.size })}</Badge>
          </div>
        </div>
        {list}
        {truncated && <div className="px-3 pt-2 text-xs text-warning-text">{t('picker.truncated')}</div>}
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 border-t border-line-soft px-3 py-2.5">
          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span>{t('picker.writeMode')}</span>
            <SegmentedControl
              size="sm"
              className="w-48"
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
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>{t('picker.collapse')}</Button>
            <Button type="button" disabled={selected.size === 0 || loading || Boolean(error)} onClick={() => onApply([...selected].sort(), mode, hasPrices && withPrices)}>
              {t('picker.confirm')}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  )
}

/** 列表上方的「全选 / 反选」：作用于当前搜索结果（cmdk 过滤后的可见项） */
function SelectBar({ items, selected, onToggleMany }: { items: ModelPickerOption[]; selected: Set<string>; onToggleMany: (models: string[], force: boolean) => void }) {
  // cmdk 的 filtered.items 是原地修改的 Map（引用不变，订阅不到），这里按同一套打分函数复算可见项
  const search = useCommandState((state) => state.search)
  const visible = items
    .filter((item) => !search || defaultFilter(item.publicModel, search, [item.upstreamModel, item.context ?? '']) > 0)
    .map((item) => item.publicModel)
  const all = visible.length > 0 && visible.every((model) => selected.has(model))
  const some = visible.some((model) => selected.has(model))
  const invert = () => {
    onToggleMany(visible.filter((model) => selected.has(model)), false)
    onToggleMany(visible.filter((model) => !selected.has(model)), true)
  }
  return (
    <div className="flex items-center justify-between gap-2 px-2 py-1">
      <label className="inline-flex cursor-pointer items-center gap-2 text-xs text-muted-foreground">
        <Checkbox
          checked={all ? true : some ? 'indeterminate' : false}
          onCheckedChange={() => onToggleMany(visible, !all)}
          disabled={visible.length === 0}
          aria-label={t('picker.selectAll')}
        />
        {t('picker.selectAll')}
      </label>
      <Button type="button" size="xs" variant="ghost" disabled={visible.length === 0} onClick={invert}>
        {t('picker.invert')}
      </Button>
    </div>
  )
}

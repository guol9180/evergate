// 全站 UI 原语唯一入口（barrel）：shadcn/ui（Radix 变体）的实现放在 components/ui/，这里保留项目特有的组合组件。
// 颜色一律走 index.css 语义 token（T76），禁止在此出现调色板类名。
import {
  forwardRef,
  type ReactNode,
  cloneElement,
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
} from 'react'
import { Checkbox as CheckboxPrimitive, Dialog as DialogPrimitive, Select as SelectPrimitive, Switch as SwitchPrimitive } from 'radix-ui'
import { Check, ChevronDown, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, ChevronUp, CircleAlert, CircleCheck, Copy, Inbox, Info, TriangleAlert, X, type LucideIcon } from 'lucide-react'
import type { Tone } from '../lib/tone'
import { t, tf } from '../lib/i18n'
import { copyText } from '../lib/clipboard'
import { errorMessage } from '../lib/errors'
import { cn } from '../lib/utils'
import { Button, type ButtonProps } from './ui/button'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from './ui/accordion'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from './ui/alert-dialog'
import { Badge as BadgePrimitive } from './ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from './ui/collapsible'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuShortcut, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from './ui/dropdown-menu'
import { Field as FieldPrimitive, FieldDescription, FieldError as FieldErrorPrimitive, FieldLabel } from './ui/field'
import { Kbd, KbdGroup } from './ui/kbd'
import { Popover, PopoverAnchor, PopoverContent, PopoverTrigger } from './ui/popover'
import { Progress as ProgressPrimitive } from './ui/progress'
import { Separator } from './ui/separator'
import { Skeleton as SkeletonPrimitive } from './ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from './ui/tabs'
import { TooltipProvider as TooltipRootProvider, TooltipWrap } from './ui/tooltip'
import { Th, Td } from './ui/table-cells'
import { DataTable, DataTableColumnToggle, type DataTableColumn, type DataTableProps } from './ui/data-table'
import { ToggleGroup, ToggleGroupItem } from './ui/toggle-group'

export type { Tone }

export const toneClass: Record<Tone, string> = {
  success: 'bg-success-subtle text-success-text',
  info: 'bg-info-subtle text-info-text',
  warning: 'bg-warning-subtle text-warning-text',
  danger: 'bg-danger-subtle text-danger-text',
  neutral: 'bg-neutral-subtle text-neutral-text',
  brand: 'bg-brand-subtle text-brand-text',
}

// shadcn/ui（radix-nova）生成的实现放在 components/ui/，这里导入后再导出：页面仍然只从 components/ui 取原语。
export { Button, buttonVariants } from './ui/button'
export type { ButtonProps } from './ui/button'
export { Accordion, AccordionContent, AccordionItem, AccordionTrigger }
export { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle }
export { Collapsible, CollapsibleContent, CollapsibleTrigger }
export { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuShortcut, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger }
export { Kbd, KbdGroup }
export { Popover, PopoverAnchor, PopoverContent, PopoverTrigger }
export { Separator }
export { Tabs, TabsContent, TabsList, TabsTrigger }
export { Th, Td }
export { DataTable, DataTableColumnToggle }
export type { DataTableColumn, DataTableProps }

// 控件带 aria-invalid 时由 Field 置为出错态：红边 + 红色聚焦环，错误文字紧贴其下。
const invalidRing = 'aria-invalid:border-danger-solid aria-invalid:focus:border-danger-solid aria-invalid:focus:ring-danger-subtle'

export function Input({ className = '', ...props }: React.ComponentProps<'input'>) {
  return (
    <input
      className={`h-9 w-full rounded-md border border-line-strong bg-surface px-3 text-sm text-foreground outline-none transition-colors placeholder:text-subtle focus:border-brand focus:ring-2 focus:ring-brand-subtle disabled:bg-surface-strong disabled:text-subtle ${invalidRing} ${className}`}
      {...props}
    />
  )
}

export function Textarea({ className = '', ...props }: React.ComponentProps<'textarea'>) {
  return (
    <textarea
      className={`w-full rounded-md border border-line-strong bg-surface px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-subtle focus:border-brand focus:ring-2 focus:ring-brand-subtle disabled:bg-surface-strong disabled:text-subtle ${invalidRing} ${className}`}
      {...props}
    />
  )
}

export interface SelectOption { value: string; label: ReactNode; disabled?: boolean }

// Radix SelectItem 不接受空串；「全部 / 不限」在业务里是 value=""，内部用哨兵承载。
const ALL_VALUE = '__eg_all__'
const toInternal = (value: string) => (value === '' ? ALL_VALUE : value)
const fromInternal = (value: string) => (value === ALL_VALUE ? '' : value)

/**
 * 下拉选择（Radix Select）：options 数组 API，调用点不再手写 <option>；
 * 弹层材质与 Dialog 同源，选中项带对勾，支持键盘上下 / Enter / typeahead。
 */
export function Select({ value, onValueChange, options, size = 'md', className = '', disabled, id, placeholder, 'aria-label': ariaLabel, 'aria-describedby': ariaDescribedBy, 'aria-invalid': ariaInvalid }: {
  value: string
  onValueChange: (value: string) => void
  options: readonly SelectOption[]
  size?: 'sm' | 'md'
  className?: string
  disabled?: boolean
  id?: string
  placeholder?: string
  'aria-label'?: string
  'aria-describedby'?: string
  'aria-invalid'?: boolean
}) {
  const compact = size === 'sm'
  return (
    <SelectPrimitive.Root value={toInternal(value)} onValueChange={(next) => onValueChange(fromInternal(next))} disabled={disabled}>
      <SelectPrimitive.Trigger
        id={id}
        disabled={disabled}
        aria-label={ariaLabel}
        aria-describedby={ariaDescribedBy}
        aria-invalid={ariaInvalid || undefined}
        className={`group inline-flex items-center justify-between gap-2 rounded-md border border-line-strong bg-surface text-foreground outline-none transition-colors focus-visible:border-brand focus-visible:ring-2 focus-visible:ring-brand-subtle data-[state=open]:border-brand data-[state=open]:ring-2 data-[state=open]:ring-brand-subtle disabled:cursor-not-allowed disabled:bg-surface-strong disabled:text-subtle aria-invalid:border-danger-solid aria-invalid:focus-visible:border-danger-solid aria-invalid:focus-visible:ring-danger-subtle aria-invalid:data-[state=open]:border-danger-solid aria-invalid:data-[state=open]:ring-danger-subtle ${
          compact ? 'h-7 w-auto px-2 text-xs' : 'h-9 w-full px-3 text-sm'
        } ${className}`}
      >
        <span className="min-w-0 truncate">
          <SelectPrimitive.Value placeholder={placeholder} />
        </span>
        <SelectPrimitive.Icon asChild>
          <ChevronDown size={compact ? 14 : 16} className="shrink-0 text-muted-foreground transition-transform duration-200 group-data-[state=open]:rotate-180" aria-hidden="true" />
        </SelectPrimitive.Icon>
      </SelectPrimitive.Trigger>
      <SelectPrimitive.Portal>
        <SelectPrimitive.Content position="popper" sideOffset={6} collisionPadding={8} className="eg-select-content eg-select-pop-in">
          <SelectPrimitive.ScrollUpButton className="eg-select-scroll">
            <ChevronUp size={14} aria-hidden="true" />
          </SelectPrimitive.ScrollUpButton>
          <SelectPrimitive.Viewport className="p-1">
            {options.map((option) => (
              <SelectPrimitive.Item
                key={option.value}
                value={toInternal(option.value)}
                disabled={option.disabled}
                className="relative flex h-8 cursor-pointer select-none items-center rounded-md pl-2.5 pr-8 text-sm text-foreground-soft outline-none data-[highlighted]:bg-surface-strong data-[highlighted]:text-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 data-[state=checked]:font-medium data-[state=checked]:text-foreground"
              >
                <SelectPrimitive.ItemText>{option.label}</SelectPrimitive.ItemText>
                <SelectPrimitive.ItemIndicator className="absolute right-2 inline-flex text-brand-text">
                  <Check size={14} aria-hidden="true" />
                </SelectPrimitive.ItemIndicator>
              </SelectPrimitive.Item>
            ))}
          </SelectPrimitive.Viewport>
          <SelectPrimitive.ScrollDownButton className="eg-select-scroll">
            <ChevronDown size={14} aria-hidden="true" />
          </SelectPrimitive.ScrollDownButton>
        </SelectPrimitive.Content>
      </SelectPrimitive.Portal>
    </SelectPrimitive.Root>
  )
}

/** 复选框（Radix Checkbox）：勾选态走品牌色，浅深色一致。 */
export function Checkbox({ className = '', ...props }: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      className={`flex h-4 w-4 shrink-0 items-center justify-center rounded border border-line-strong bg-surface transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:opacity-50 data-[state=checked]:border-brand data-[state=checked]:bg-brand data-[state=checked]:text-white ${className}`}
      {...props}
    >
      <CheckboxPrimitive.Indicator className="flex items-center justify-center text-current">
        <Check size={12} strokeWidth={3} aria-hidden="true" />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  )
}

/** 应用根部的 Tooltip Provider（main.tsx 挂一次），统一延迟与跳过重叠交互的设置。 */
const Tooltip = TooltipWrap
const TooltipProvider = TooltipRootProvider
export { Tooltip, TooltipProvider }

/** 图标按钮：Button + Tooltip + aria-label 只在这里组合；表格行操作 / 分页箭头 / 侧栏共用。 */
export function IconButton({ label, danger = false, side, variant = 'ghost', loading = false, size = 'icon-sm', className = '', children, ...props }: React.ComponentProps<'button'> & { label: string; danger?: boolean; side?: 'top' | 'right' | 'bottom' | 'left'; variant?: ButtonProps['variant']; loading?: boolean; size?: 'icon' | 'icon-sm' | 'icon-xs' }) {
  return (
    <Tooltip content={label} side={side}>
      <Button size={size} variant={variant} aria-label={label} loading={loading} className={`${danger ? 'text-danger-text hover:bg-danger-subtle' : ''} ${className}`} {...props}>
        {children}
      </Button>
    </Tooltip>
  )
}

/**
 * 复制按钮（全站唯一出处）：compact 为字段旁的图标形态，默认为带文字的小按钮。
 * 结果直接在按钮上短暂反馈（aria-live），不弹 Toast：复制是轻量动作，不值得一条通知。
 */
export function CopyButton({ value, label, compact = false, className = '', side = 'top' }: {
  value: string
  label?: string
  compact?: boolean
  className?: string
  side?: 'top' | 'right' | 'bottom' | 'left'
}) {
  const [state, setState] = useState<'' | 'ok' | 'fail'>('')
  useEffect(() => {
    if (!state) return
    const timer = window.setTimeout(() => setState(''), 1500)
    return () => window.clearTimeout(timer)
  }, [state])
  const copy = async () => setState(await copyText(value) ? 'ok' : 'fail')
  const text = state === 'ok' ? t('copy.copied') : state === 'fail' ? t('copy.failed') : label ?? t('copy.button')
  if (compact) {
    return (
      <IconButton label={text} side={side} size="icon-xs" className={className} onClick={() => void copy()} aria-live="polite">
        {state === 'ok' ? <Check size={13} className="text-success-text" /> : <Copy size={13} />}
      </IconButton>
    )
  }
  return (
    <Button size="sm" variant="outline" className={className} onClick={() => void copy()} aria-live="polite">
      {state === 'ok' ? <Check size={14} className="text-success-text" /> : <Copy size={14} />}
      {text}
    </Button>
  )
}

/** 开关（Radix Switch）：role / aria-checked / 键盘空格切换由原语负责，调用方只给 checked / onChange。 */
export function Switch({ checked, onChange, disabled, label, id }: { checked: boolean; onChange: (value: boolean) => void; disabled?: boolean; label: string; id?: string }) {
  return (
    <SwitchPrimitive.Root
      id={id}
      checked={checked}
      onCheckedChange={onChange}
      disabled={disabled}
      aria-label={label}
      className="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:opacity-50 data-[state=checked]:bg-brand data-[state=unchecked]:bg-line-strong"
    >
      <SwitchPrimitive.Thumb className="eg-switch-thumb pointer-events-none block h-4 w-4 rounded-full bg-surface shadow transition-transform data-[state=checked]:translate-x-4 data-[state=unchecked]:translate-x-0.5" />
    </SwitchPrimitive.Root>
  )
}

/**
 * Field（shadcn Field 组合）：label 通过 useId 自动关联第一个控件，hint / error 作为 aria-describedby；
 * error 存在时控件置 aria-invalid 并以 FieldError 取代 hint。不用 <label> 包裹控件，避免内含按钮时误触发。
 */
export function Field({ label, children, hint, error, className }: { label: ReactNode; children: ReactNode; hint?: ReactNode; error?: ReactNode; className?: string }) {
  const id = useId()
  const hintId = `${id}-hint`
  const errorId = `${id}-error`
  const describedBy = [hint && !error ? hintId : '', error ? errorId : ''].filter(Boolean).join(' ') || undefined
  let control: ReactNode = children
  if (isValidElement(children)) {
    const element = children as React.ReactElement<Record<string, unknown>>
    const props = element.props ?? {}
    control = cloneElement(element, {
      id: props.id ?? id,
      'aria-describedby': describedBy ?? props['aria-describedby'],
      ...(error ? { 'aria-invalid': true } : {}),
    })
  }
  return (
    <FieldPrimitive data-invalid={error ? true : undefined} className={cn('gap-1.5', className)}>
      <FieldLabel htmlFor={id} className="block w-fit text-sm font-medium text-foreground-soft">{label}</FieldLabel>
      {control}
      {error ? <FieldError id={errorId}>{error}</FieldError> : hint ? <FieldDescription id={hintId} className="text-xs text-subtle">{hint}</FieldDescription> : null}
    </FieldPrimitive>
  )
}

/** 字段级内联错误：紧贴出错控件。不用 live region（覆盖 shadcn 默认的 role=alert），由 aria-invalid + aria-describedby 在聚焦时播报。 */
export function FieldError({ id, children, className }: { id?: string; children: ReactNode; className?: string }) {
  return (
    <FieldErrorPrimitive id={id} role={undefined} data-field-error="" className={cn('flex items-start gap-1.5 text-xs leading-5 text-danger-text', className)}>
      <CircleAlert size={13} className="mt-0.5 shrink-0" aria-hidden="true" />
      <span>{children}</span>
    </FieldErrorPrimitive>
  )
}

/** 提交失败后把焦点移到第一个出错字段，读屏用户不必从头 Tab。需在状态提交后调用（内部用 rAF 等待渲染）。 */
export function focusFirstInvalid(root: ParentNode | null) {
  requestAnimationFrame(() => {
    root?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus()
  })
}

export type BadgeProps = React.ComponentProps<'span'> & { tone?: Tone; compact?: boolean }

/** 徽标（shadcn Badge）：tone 只换底色 / 文字色；透传 ref 与 rest props，可直接当 Radix Trigger（asChild）用。 */
export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(function Badge({ tone = 'neutral', compact = false, className, children, ...props }, ref) {
  return (
    <BadgePrimitive
      ref={ref}
      variant="secondary"
      className={cn('h-auto gap-0 rounded-full border-transparent py-0.5 transition-colors', compact ? 'px-1.5 text-[10px]' : 'px-2 text-xs', toneClass[tone], className)}
      {...props}
    >
      {children}
    </BadgePrimitive>
  )
})

/** 状态徽标：与 Badge 同源，语义化命名用于渠道/令牌/HTTP 状态展示。 */
const dotClass: Record<Tone, string> = {
  success: 'bg-success-solid',
  info: 'bg-info-solid',
  warning: 'bg-warning-solid',
  danger: 'bg-danger-solid',
  neutral: 'bg-neutral-text',
  brand: 'bg-brand',
}

export type StatusBadgeProps = React.ComponentProps<'span'> & { tone: Tone }

export const StatusBadge = forwardRef<HTMLSpanElement, StatusBadgeProps>(function StatusBadge({ tone, className, children, ...props }, ref) {
  return (
    <Badge ref={ref} tone={tone} className={className} {...props}>
      <span aria-hidden="true" className={`mr-1.5 inline-block h-1.5 w-1.5 rounded-full ${dotClass[tone]}`} />
      {children}
    </Badge>
  )
})

export function Card({ className = '', children, flush = false }: { className?: string; children: ReactNode; flush?: boolean }) {
  return <div className={`${flush ? '' : 'eg-card-fluid'} rounded-xl border border-line bg-surface shadow-sm ${className}`}>{children}</div>
}

export function PageHeader({ title, description, actions }: { title: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="eg-page-header flex flex-wrap items-start justify-between">
      <div className="min-w-0">
        <h1 className="text-lg font-bold tracking-tight text-foreground">{title}</h1>
        {description && <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>}
      </div>
      {actions && <div className="eg-control-row flex w-full flex-wrap items-center sm:w-auto sm:shrink-0">{actions}</div>}
    </header>
  )
}

export { Spinner } from './ui/spinner'

type AlertTone = 'info' | 'success' | 'warning' | 'danger' | 'neutral'

const alertIcons: Record<AlertTone, LucideIcon> = {
  info: Info,
  success: CircleCheck,
  warning: TriangleAlert,
  danger: CircleAlert,
  neutral: Info,
}

const alertStyles: Record<AlertTone, string> = {
  info: 'border-brand-border bg-brand-subtle text-brand-text',
  success: 'border-success-border bg-success-subtle text-success-text',
  warning: 'border-warning-border bg-warning-subtle text-warning-text',
  danger: 'border-danger-border bg-danger-subtle text-danger-text',
  neutral: 'border-line bg-surface-soft text-foreground-soft',
}

/**
 * 行内提示框，只用于需要持续阅读的内容：页头下方的页面级横幅、表单 / 弹窗顶部的提交错误、详情里的上游错误。
 * 瞬时操作结果走 Toaster；可归因到字段的校验走 Field error。危险与警告用 role=alert，其余用 status。
 */
export function Alert({
  tone = 'info',
  title,
  action,
  children,
  className = '',
  ...props
}: React.HTMLAttributes<HTMLDivElement> & {
  tone?: AlertTone
  title?: ReactNode
  action?: ReactNode
}) {
  const Icon = alertIcons[tone]
  return (
    <div
      role={tone === 'danger' || tone === 'warning' ? 'alert' : 'status'}
      className={`eg-alert-in flex items-start gap-2.5 rounded-lg border px-3 py-2.5 text-xs leading-5 ${alertStyles[tone]} ${className}`}
      {...props}
    >
      <Icon size={15} className="mt-0.5 shrink-0" aria-hidden="true" />
      <div className="min-w-0 flex-1">
        {title && <div className="font-semibold">{title}</div>}
        {children && <div className={title ? 'mt-0.5' : ''}>{children}</div>}
      </div>
      {action && <div className="shrink-0 self-center">{action}</div>}
    </div>
  )
}

/** 骨架块（shadcn Skeleton）：首次加载时替代 Spinner，保持布局稳定；外观由 eg-skeleton 令牌类负责。 */
export function Skeleton({ className, ...props }: React.ComponentProps<'div'>) {
  return <SkeletonPrimitive aria-hidden="true" className={cn('eg-skeleton', className)} {...props} />
}

/**
 * 加载失败态（列表 / 概览 / 卡片）：危险 Alert + 重试。
 * 与「有数据但为空」「筛选无结果」严格区分，接口失败时绝不渲染空表或引导态。
 */
export function LoadError({ error, onRetry, title, className = '' }: { error: unknown; onRetry?: () => void; title?: string; className?: string }) {
  return (
    <Alert
      tone="danger"
      title={title ?? t('loadFailed.list')}
      className={className}
      action={onRetry ? <Button size="sm" variant="outline" onClick={onRetry}>{t('common.retry')}</Button> : undefined}
    >
      {errorMessage(error)}
    </Alert>
  )
}

/** 表格骨架：header 行 + n 数据行。 */
export function SkeletonRows({ rows = 5, className = '' }: { rows?: number; className?: string }) {
  return (
    <div className={`space-y-3 ${className}`} role="status" aria-label={t('common.loading')}>
      <Skeleton className="h-8 w-full" />
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-7 w-full" style={{ opacity: 1 - i * 0.08 }} />
      ))}
    </div>
  )
}

export function PageSkeleton() {
  return (
    <div className="eg-page-stack" role="status" aria-label={t('common.loading')}>
      <Skeleton className="h-10 w-64" />
      <div className="eg-grid eg-grid-auto">
        {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-20" />)}
      </div>
      <Skeleton className="h-64" />
    </div>
  )
}

const dialogBody = 'eg-dialog-body'

/**
 * 记录打开对话框前的焦点元素：Radix 默认把焦点还给 DialogTrigger，而这里的触发器是页面里的任意按钮
 * （没有 Trigger 组件），所以要自己捕获。必须在 open 翻转为 true 的那次渲染里取，
 * 等 effect 阶段焦点已经被移进对话框了。
 */
function useRestoreFocus(open: boolean) {
  const wasOpen = useRef(false)
  const target = useRef<HTMLElement | null>(null)
  if (open && !wasOpen.current) target.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
  wasOpen.current = open
  return (event: Event) => {
    event.preventDefault()
    target.current?.focus()
  }
}

/**
 * 模态对话框（Radix Dialog）：Portal、焦点陷阱、Esc、点击遮罩关闭、焦点归还、滚动锁由 Radix 负责；
 * 这里只负责版式与令牌。面板用 inset-0 + margin:auto 居中，避免 transform 居中与入场动画互相覆盖。
 */
export function Dialog({ open, onClose, title, description, children, footer, wide, role = 'dialog' }: { open: boolean; onClose: () => void; title: ReactNode; description?: ReactNode; children: ReactNode; footer?: ReactNode; wide?: boolean; role?: 'dialog' | 'alertdialog' }) {
  const descId = useId()
  const restoreFocus = useRestoreFocus(open)
  return (
    <DialogPrimitive.Root open={open} onOpenChange={(next) => { if (!next) onClose() }}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="eg-overlay fixed inset-0 z-50 bg-overlay" />
        <DialogPrimitive.Content
          role={role}
          aria-modal="true"
          aria-describedby={description ? descId : undefined}
          onCloseAutoFocus={restoreFocus}
          className={`eg-dialog-panel fixed inset-0 z-50 m-auto flex h-fit max-h-[90vh] w-[calc(100%-2*var(--eg-page-x))] ${wide ? 'max-w-2xl' : 'max-w-md'} flex-col overflow-hidden rounded-xl bg-surface shadow-xl outline-none`}
        >
          <div className={`${dialogBody} flex shrink-0 items-center justify-between border-b border-line-soft`}>
            <DialogPrimitive.Title asChild>
              <h3 className="text-sm font-semibold text-foreground">{title}</h3>
            </DialogPrimitive.Title>
            <CloseButton />
          </div>
          {description && <p id={descId} className={`${dialogBody} border-b border-line-soft pb-3 text-xs leading-5 text-muted-foreground`}>{description}</p>}
          <div className={`${dialogBody} flex-1 overflow-auto`}>{children}</div>
          {footer && <div className="eg-dialog-footer shrink-0 border-t border-line-soft bg-surface">{footer}</div>}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

/** 右侧抽屉：与 Dialog 同一 Radix 底座，只换面板位置与入场动画。 */
export function Drawer({ open, onClose, title, description, children, wide }: { open: boolean; onClose: () => void; title: ReactNode; description?: ReactNode; children: ReactNode; wide?: boolean }) {
  const descId = useId()
  const restoreFocus = useRestoreFocus(open)
  return (
    <DialogPrimitive.Root open={open} onOpenChange={(next) => { if (!next) onClose() }}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="eg-overlay fixed inset-0 z-50 bg-overlay" />
        <DialogPrimitive.Content
          aria-modal="true"
          aria-describedby={description ? descId : undefined}
          onCloseAutoFocus={restoreFocus}
          className={`eg-drawer-panel fixed inset-y-0 right-0 z-50 flex h-full w-full ${wide ? 'sm:w-[min(640px,94vw)]' : 'sm:w-[min(420px,92vw)]'} flex-col bg-surface shadow-2xl outline-none`}
        >
          <div className={`${dialogBody} flex items-center justify-between border-b border-line-soft`}>
            <DialogPrimitive.Title asChild>
              <h3 className="text-sm font-semibold text-foreground">{title}</h3>
            </DialogPrimitive.Title>
            <CloseButton />
          </div>
          {description && <p id={descId} className={`${dialogBody} border-b border-line-soft pb-3 text-xs leading-5 text-muted-foreground`}>{description}</p>}
          <div className={`${dialogBody} flex-1 overflow-auto`}>{children}</div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

function CloseButton() {
  // 不包 Tooltip：Radix Tooltip 在获得焦点时也会弹出，而对话框打开时焦点正好落在关闭按钮上，
  // 会每次打开都先冒出一个"关闭"气泡；X 图标 + aria-label 已足够
  return (
    <DialogPrimitive.Close asChild>
      <button type="button" aria-label={t('common.close')} className="rounded p-1 text-subtle transition-colors hover:bg-surface-strong hover:text-foreground-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand">
        <X size={16} aria-hidden="true" />
      </button>
    </DialogPrimitive.Close>
  )
}

/** 危险/确认操作对话框：替代浏览器原生 confirm()，文案必须说明后果。 */
export function ConfirmDialog({ open, title, message, confirmText, cancelText, danger = false, loading = false, onConfirm, onCancel }: {
  open: boolean
  title: ReactNode
  message: ReactNode
  confirmText?: string
  cancelText?: string
  danger?: boolean
  loading?: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const restoreFocus = useRestoreFocus(open)
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onCancel()
      }}
    >
      <AlertDialogContent onCloseAutoFocus={restoreFocus}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
        </AlertDialogHeader>
        <AlertDialogDescription asChild>
          <div className="space-y-3 text-sm text-foreground-soft">{typeof message === 'string' ? <p>{message}</p> : message}</div>
        </AlertDialogDescription>
        <AlertDialogFooter>
          {/* 按钮不自动关闭：确认后是否关闭由调用方按请求结果决定，取消只走 onCancel 一个出口（Esc 同样落到 onCancel） */}
          <AlertDialogCancel variant="outline" disabled={loading} onClick={onCancel}>
            {cancelText ?? t('common.cancel')}
          </AlertDialogCancel>
          <AlertDialogAction variant={danger ? 'destructive' : 'default'} loading={loading} autoFocus onClick={onConfirm}>
            {confirmText ?? t('common.confirm')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

/** 设置行：标题 + 说明 + 右侧开关，整块为 label 可点击切换；替代各页面重复的开关卡片块。 */
export function SettingRow({ title, description, checked, onChange, label, disabled }: {
  title: ReactNode
  description?: ReactNode
  checked: boolean
  onChange: (value: boolean) => void
  label: string
  disabled?: boolean
}) {
  const switchId = useId()
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border border-line bg-surface-soft px-3 py-2.5">
      <label htmlFor={switchId} className="min-w-0 cursor-pointer">
        <div className="text-sm text-foreground-soft">{title}</div>
        {description && <div className="text-xs text-subtle">{description}</div>}
      </label>
      <Switch id={switchId} checked={checked} onChange={onChange} label={label} disabled={disabled} />
    </div>
  )
}

/**
 * 分段单选（shadcn ToggleGroup，type=single，单项即 radio）：单条轨道内凸起选中段；roving focus 与方向键由 Radix 负责。
 * Radix 的方向键只移动焦点，这里让「焦点到哪就选到哪」（radio 语义，也是键盘用户少按一次的来源），
 * 不再需要在 keydown 里手写方向补丁。
 */
export function SegmentedControl<T extends string>({ value, onChange, options, label, disabled, size = 'md', className }: {
  value: T
  onChange: (value: T) => void
  options: readonly { value: T; label: ReactNode }[]
  label: string
  disabled?: boolean
  size?: 'sm' | 'md'
  className?: string
}) {
  return (
    <ToggleGroup
      type="single"
      role="radiogroup"
      value={value}
      onValueChange={(next) => {
        // type=single 允许点选当前段取消；分段控件必须始终有一段选中，空值与重复值都不向外抛
        if (next && next !== value) onChange(next as T)
      }}
      disabled={disabled}
      aria-label={label}
      spacing={1}
      className={cn('rounded-lg bg-surface-strong p-1', className)}
    >
      {options.map((option) => (
        <ToggleGroupItem
          key={option.value}
          value={option.value}
          onFocus={() => {
            if (option.value !== value) onChange(option.value)
          }}
          className={cn(
            'min-w-0 flex-1 shrink items-center justify-center rounded-md font-medium text-muted-foreground hover:bg-transparent hover:text-foreground focus-visible:ring-2 focus-visible:ring-brand data-[state=on]:bg-surface data-[state=on]:text-foreground data-[state=on]:shadow-sm disabled:opacity-50',
            size === 'sm' ? 'h-6 px-2 text-xs' : 'h-7 px-3 text-sm'
          )}
        >
          <span className="truncate">{option.label}</span>
        </ToggleGroupItem>
      ))}
    </ToggleGroup>
  )
}

/** 进度条：更新流程等需要展示百分比的位置共用；颜色只走语义色。 */
export function Progress({ value, tone = 'brand', className }: { value: number; tone?: 'brand' | 'danger'; className?: string }) {
  const clamped = Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0))
  return (
    <ProgressPrimitive
      value={clamped}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(clamped)}
      className={cn('h-2 bg-surface-strong', className)}
      indicatorClassName={tone === 'danger' ? 'bg-danger-solid' : 'bg-brand'}
    />
  )
}

/**
 * 空状态唯一出处：图标容器 + 标题 + 说明 + 主行动。
 * filtered=true 表示"有数据但被筛选条件排空"，用中性色容器，调用方应提供清除筛选的主行动；
 * 初始为空用品牌色容器，主行动指向创建/接入。
 */
export function Empty({ icon, title, hint, action, filtered = false, className = '' }: { icon?: ReactNode; title: string; hint?: ReactNode; action?: ReactNode; filtered?: boolean; className?: string }) {
  return (
    <div className={`flex flex-col items-center justify-center gap-2 px-4 py-14 text-center ${className}`}>
      <div className={`mb-1 flex h-12 w-12 items-center justify-center rounded-full ${filtered ? 'bg-surface-strong text-muted-foreground' : 'bg-brand-subtle text-brand-text'} [&>svg]:h-5 [&>svg]:w-5`}>
        {icon ?? <Inbox aria-hidden="true" />}
      </div>
      <p className="text-sm font-medium text-foreground-soft">{title}</p>
      {hint && <p className="max-w-sm text-xs text-muted-foreground">{hint}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  )
}

/**
 * 折叠分组（shadcn Collapsible）：展开状态、按钮语义与 aria-expanded 由 Radix 负责；
 * 这里只统一版式与箭头旋转。defaultOpen 只决定初始状态，之后由用户掌控。
 */
export function Disclosure({ title, badge, defaultOpen = true, children, className }: {
  title: ReactNode
  badge?: ReactNode
  defaultOpen?: boolean
  children: ReactNode
  className?: string
}) {
  return (
    <Collapsible defaultOpen={defaultOpen} className={cn('group rounded-lg border border-line', className)}>
      <CollapsibleTrigger className="flex w-full cursor-pointer items-center gap-2 rounded-t-lg px-3 py-2 text-left text-xs font-semibold text-foreground-soft outline-none select-none focus-visible:ring-2 focus-visible:ring-brand">
        <ChevronRight size={14} className="shrink-0 text-subtle transition-transform group-data-[state=open]:rotate-90" aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate">{title}</span>
        {badge && <span className="shrink-0">{badge}</span>}
      </CollapsibleTrigger>
      <CollapsibleContent className="border-t border-line-soft px-3 py-3">{children}</CollapsibleContent>
    </Collapsible>
  )
}

/** 骨架到内容的交叉淡入：骨架在加载期占位，内容到位后作为绝对层淡出，不参与布局。 */
export function ContentSwap({ loading, skeleton, children, className = '' }: { loading: boolean; skeleton: ReactNode; children: ReactNode; className?: string }) {
  const [fading, setFading] = useState(false)
  const wasLoading = useRef(loading)
  useEffect(() => {
    if (loading) {
      wasLoading.current = true
      setFading(false)
      return
    }
    if (!wasLoading.current) return
    wasLoading.current = false
    setFading(true)
    const timer = window.setTimeout(() => setFading(false), 200)
    return () => window.clearTimeout(timer)
  }, [loading])
  if (loading) return <div className={className}>{skeleton}</div>
  return (
    <div className={`relative ${className}`}>
      <div className="eg-content-in">{children}</div>
      {fading && (
        <div aria-hidden="true" className="eg-skeleton-overlay pointer-events-none absolute inset-0 z-10">
          {skeleton}
        </div>
      )}
    </div>
  )
}

/** 加载 / 详情类错误（需要持续阅读）的简写；提交错误请用 Alert 置于表单顶部或 Field error。 */
export function ErrorText({ children }: { children?: ReactNode }) {
  if (!children) return null
  return <Alert tone="danger">{children}</Alert>
}

/** 表格容器：窄屏卡片内横向滚动，禁止页面级横向溢出（AGENTS 管理台约束）。 */
export function TableWrap({ minWidth = 720, children, className = '' }: { minWidth?: number; children: ReactNode; className?: string }) {
  return (
    <div className={`overflow-x-auto ${className}`} style={{ ['--eg-table-min' as string]: `${minWidth}px` }}>
      {children}
    </div>
  )
}

/** 分页器：页码 + 每页条数；替代仅"上一页/下一页"的箭头分页。 */
export function Pagination({ page, totalPages, total, pageSize, sizes = [20, 50, 100], onPageChange, onPageSizeChange, className = '' }: {
  page: number
  totalPages: number
  total: number
  pageSize: number
  sizes?: number[]
  onPageChange: (page: number) => void
  onPageSizeChange?: (size: number) => void
  className?: string
}) {
  const pages = pageWindow(page, totalPages, 2)
  const goto = (target: number) => {
    if (target >= 1 && target <= totalPages && target !== page) onPageChange(target)
  }
  return (
    <nav aria-label={t('pagination.label')} className={`flex flex-wrap items-center justify-between gap-2 border-t border-line-soft text-xs text-muted-foreground ${className}`} style={{ padding: 'var(--eg-cell-y) var(--eg-card-pad)' }}>
      <span className="tabular-nums">{tf('pagination.summary', { total: total.toLocaleString('zh-CN'), page, pages: totalPages })}</span>
      <div className="flex flex-wrap items-center gap-2">
        {onPageSizeChange && (
          <Select
            size="sm"
            aria-label={t('pagination.pageSize')}
            value={String(pageSize)}
            onValueChange={(next) => onPageSizeChange(Number(next))}
            options={sizes.map((s) => ({ value: String(s), label: tf('pagination.pageSizeOption', { n: s }) }))}
          />
        )}
        <div className="flex items-center gap-1">
          <IconButton variant="outline" label={t('pagination.first')} disabled={page <= 1} onClick={() => goto(1)}><ChevronsLeft size={14} /></IconButton>
          <IconButton variant="outline" label={t('pagination.prev')} disabled={page <= 1} onClick={() => goto(page - 1)}><ChevronLeft size={14} /></IconButton>
          {pages.map((p, i) => (
            p === null
              ? <span key={`gap-${i}`} className="px-1 text-subtle" aria-hidden="true">…</span>
              : (
                <Button
                  key={p}
                  size="icon-sm"
                  variant={p === page ? 'default' : 'outline'}
                  aria-label={tf('pagination.pageAria', { p })}
                  aria-current={p === page ? 'page' : undefined}
                  onClick={() => goto(p)}
                >
                  {p}
                </Button>
              )
          ))}
          <IconButton variant="outline" label={t('pagination.next')} disabled={page >= totalPages} onClick={() => goto(page + 1)}><ChevronRight size={14} /></IconButton>
          <IconButton variant="outline" label={t('pagination.last')} disabled={page >= totalPages} onClick={() => goto(totalPages)}><ChevronsRight size={14} /></IconButton>
        </div>
      </div>
    </nav>
  )
}

/** 页码窗口：首页/末页 + 当前 ±radius，间隔用 null 表示省略号。 */
export function pageWindow(page: number, totalPages: number, radius = 2): (number | null)[] {
  if (totalPages <= 0) return [1]
  const want = new Set<number>([1, totalPages])
  for (let p = page - radius; p <= page + radius; p++) if (p >= 1 && p <= totalPages) want.add(p)
  const sorted = [...want].sort((a, b) => a - b)
  const out: (number | null)[] = []
  sorted.forEach((p, i) => {
    if (i > 0 && p - sorted[i - 1] > 1) out.push(null)
    out.push(p)
  })
  return out
}

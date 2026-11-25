// shadcn 风格手写最小组件集（任务书 3.2 精神：可控、无重型依赖）
// 颜色一律走 index.css 语义 token（T76），禁止在此出现调色板类名。
import {
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type RefObject,
  type SelectHTMLAttributes,
  cloneElement,
  isValidElement,
  useEffect,
  useId,
  useRef,
} from 'react'
import { ChevronDown, X } from 'lucide-react'
import type { Tone } from '../lib/tone'

export type { Tone }

export const toneClass: Record<Tone, string> = {
  success: 'bg-success-subtle text-success-text',
  info: 'bg-info-subtle text-info-text',
  warning: 'bg-warning-subtle text-warning-text',
  danger: 'bg-danger-subtle text-danger-text',
  neutral: 'bg-neutral-subtle text-neutral-text',
  brand: 'bg-brand-subtle text-brand-text',
}

export function Button({ variant = 'default', size = 'md', className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'default' | 'outline' | 'ghost' | 'danger'; size?: 'sm' | 'md' | 'icon' }) {
  const base = 'inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:opacity-50 disabled:pointer-events-none'
  const variants = {
    default: 'bg-brand text-white hover:bg-brand-strong',
    outline: 'border border-line-strong bg-surface text-foreground-soft hover:bg-surface-soft',
    ghost: 'text-muted hover:bg-surface-strong',
    danger: 'bg-danger-solid text-white hover:opacity-90',
  }
  const sizes = { sm: 'h-7 px-2.5 text-xs', md: 'h-9 px-4 text-sm', icon: 'h-7 w-7 px-0' }
  return <button className={`${base} ${variants[variant]} ${sizes[size]} ${className}`} {...props} />
}

export function Input({ className = '', ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`h-9 w-full rounded-md border border-line-strong bg-surface px-3 text-sm text-foreground outline-none transition-colors placeholder:text-subtle focus:border-brand focus:ring-2 focus:ring-brand-subtle disabled:bg-surface-strong disabled:text-subtle ${className}`}
      {...props}
    />
  )
}

export function Textarea({ className = '', ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={`w-full rounded-md border border-line-strong bg-surface px-3 py-2 text-sm text-foreground outline-none transition-colors placeholder:text-subtle focus:border-brand focus:ring-2 focus:ring-brand-subtle disabled:bg-surface-strong disabled:text-subtle ${className}`}
      {...props}
    />
  )
}

export function Select({ className = '', children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <div className="relative">
      <select
        className={`h-9 w-full appearance-none rounded-md border border-line-strong bg-surface px-3 pr-8 text-sm text-foreground outline-none transition-colors focus:border-brand focus:ring-2 focus:ring-brand-subtle disabled:bg-surface-strong disabled:text-subtle ${className}`}
        {...props}
      >
        {children}
      </select>
      <ChevronDown size={14} className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-subtle" />
    </div>
  )
}

export function Switch({ checked, onChange, disabled, label, id }: { checked: boolean; onChange: (value: boolean) => void; disabled?: boolean; label: string; id?: string }) {
  return (
    <button
      type="button"
      id={id}
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:opacity-50 ${checked ? 'bg-brand' : 'bg-line-strong'}`}
    >
      <span className={`inline-block h-4 w-4 transform rounded-full bg-surface shadow transition-transform ${checked ? 'translate-x-4' : 'translate-x-0.5'}`} />
    </button>
  )
}

/**
 * Field：label 通过 useId 自动关联第一个控件（Input/Textarea/Select 接受 id），
 * hint 作为 aria-describedby；不再用 <label> 包裹控件，避免内含按钮时误触发。
 */
export function Field({ label, children, hint, className = '' }: { label: ReactNode; children: ReactNode; hint?: ReactNode; className?: string }) {
  const id = useId()
  const hintId = `${id}-hint`
  let control: ReactNode = children
  if (isValidElement(children)) {
    const element = children as React.ReactElement<Record<string, unknown>>
    const props = element.props ?? {}
    control = cloneElement(element, {
      id: props.id ?? id,
      'aria-describedby': hint ? hintId : props['aria-describedby'],
    })
  }
  return (
    <div className={`space-y-1.5 ${className}`}>
      <label className="block text-sm font-medium text-foreground-soft" htmlFor={id}>{label}</label>
      {control}
      {hint && <span id={hintId} className="block text-xs text-subtle">{hint}</span>}
    </div>
  )
}

export function Badge({ tone = 'neutral', compact = false, className = '', children }: { tone?: Tone; compact?: boolean; className?: string; children: ReactNode }) {
  const size = compact ? 'px-1.5 py-0 text-[10px]' : 'px-2 py-0.5 text-xs'
  return <span className={`inline-flex items-center whitespace-nowrap rounded-full font-medium ${size} ${toneClass[tone]} ${className}`}>{children}</span>
}

/** 状态徽标：与 Badge 同源，语义化命名用于渠道/令牌/HTTP 状态展示。 */
const dotClass: Record<Tone, string> = {
  success: 'bg-success-solid',
  info: 'bg-info-solid',
  warning: 'bg-warning-solid',
  danger: 'bg-danger-solid',
  neutral: 'bg-neutral-text',
  brand: 'bg-brand',
}

export function StatusBadge({ tone, className = '', children }: { tone: Tone; className?: string; children: ReactNode }) {
  return (
    <Badge tone={tone} className={className}>
      <span aria-hidden="true" className={`mr-1.5 inline-block h-1.5 w-1.5 rounded-full ${dotClass[tone]}`} />
      {children}
    </Badge>
  )
}

export function Card({ className = '', children, flush = false }: { className?: string; children: ReactNode; flush?: boolean }) {
  return <div className={`${flush ? '' : 'eg-card-fluid'} rounded-xl border border-line bg-surface shadow-sm ${className}`}>{children}</div>
}

export function PageHeader({ title, description, actions }: { title: ReactNode; description?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="eg-page-header flex flex-wrap items-start justify-between">
      <div className="min-w-0">
        <h1 className="text-lg font-bold tracking-tight text-foreground">{title}</h1>
        {description && <p className="mt-1 text-xs leading-5 text-muted">{description}</p>}
      </div>
      {actions && <div className="eg-control-row flex w-full flex-wrap items-center sm:w-auto sm:shrink-0">{actions}</div>}
    </header>
  )
}

export function Spinner({ className = '' }: { className?: string }) {
  return <span aria-hidden="true" className={`inline-block h-4 w-4 animate-spin rounded-full border-2 border-line-strong border-t-brand ${className}`} />
}

/** 骨架块：首次加载时替代 Spinner，保持布局稳定。 */
export function Skeleton({ className = '', style }: { className?: string; style?: React.CSSProperties }) {
  return <div aria-hidden="true" className={`eg-skeleton ${className}`} style={style} />
}

/** 表格骨架：header 行 + n 数据行。 */
export function SkeletonRows({ rows = 5, className = '' }: { rows?: number; className?: string }) {
  return (
    <div className={`space-y-3 ${className}`} role="status" aria-label="加载中">
      <Skeleton className="h-8 w-full" />
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-7 w-full" style={{ opacity: 1 - i * 0.08 }} />
      ))}
    </div>
  )
}

export function PageSkeleton() {
  return (
    <div className="eg-page-stack" role="status" aria-label="加载中">
      <Skeleton className="h-10 w-64" />
      <div className="eg-grid eg-grid-auto">
        {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-20" />)}
      </div>
      <Skeleton className="h-64" />
    </div>
  )
}

export function Dialog({ open, onClose, title, children, footer, wide, role = 'dialog' }: { open: boolean; onClose: () => void; title: ReactNode; children: ReactNode; footer?: ReactNode; wide?: boolean; role?: 'dialog' | 'alertdialog' }) {
  const ref = useRef<HTMLDivElement>(null)
  const titleId = useId()
  const descId = useId()
  useDialogFocus(open, onClose, ref)
  if (!open) return null
  return (
    <div className="eg-overlay fixed inset-0 z-50 flex items-center justify-center bg-overlay p-[var(--eg-page-x)]" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={ref} tabIndex={-1} role={role} aria-modal="true" aria-labelledby={titleId} aria-describedby={role === 'alertdialog' ? descId : undefined} className={`eg-dialog-panel flex max-h-[90vh] w-full ${wide ? 'max-w-2xl' : 'max-w-md'} flex-col overflow-hidden rounded-xl bg-surface shadow-xl outline-none`}>
        <div className="eg-dialog-body flex shrink-0 items-center justify-between border-b border-line-soft">
          <h3 id={titleId} className="text-sm font-semibold text-foreground">{title}</h3>
          <CloseButton onClick={onClose} />
        </div>
        <div id={descId} className="eg-dialog-body flex-1 overflow-auto">{children}</div>
        {footer && <div className="eg-dialog-footer shrink-0 border-t border-line-soft bg-surface">{footer}</div>}
      </div>
    </div>
  )
}

export function Drawer({ open, onClose, title, children, wide }: { open: boolean; onClose: () => void; title: ReactNode; children: ReactNode; wide?: boolean }) {
  const ref = useRef<HTMLDivElement>(null)
  const titleId = useId()
  useDialogFocus(open, onClose, ref)
  if (!open) return null
  return (
    <div className="eg-overlay fixed inset-0 z-50 flex justify-end bg-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={ref} tabIndex={-1} role="dialog" aria-modal="true" aria-labelledby={titleId} className={`eg-drawer-panel flex h-full w-full ${wide ? 'sm:w-[min(560px,92vw)]' : 'sm:w-[min(420px,92vw)]'} flex-col bg-surface shadow-2xl outline-none`}>
        <div className="eg-dialog-body flex items-center justify-between border-b border-line-soft">
          <h3 id={titleId} className="text-sm font-semibold text-foreground">{title}</h3>
          <CloseButton onClick={onClose} />
        </div>
        <div className="eg-dialog-body flex-1 overflow-auto">{children}</div>
      </div>
    </div>
  )
}

function CloseButton({ onClick }: { onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} aria-label="关闭" title="关闭" className="rounded p-1 text-subtle hover:bg-surface-strong hover:text-foreground-soft">
      <X size={16} aria-hidden="true" />
    </button>
  )
}

const FOCUSABLE = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

/**
 * 对话框焦点管理：打开时焦点移入、Esc 关闭、Tab/Shift+Tab 在对话框内循环、关闭后焦点归还触发元素。
 * Dialog 与 Drawer 共用，保证两者键盘行为一致。
 */
export function useDialogFocus(open: boolean, onClose: () => void, ref: RefObject<HTMLElement | null>) {
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose
  useEffect(() => {
    if (!open) return
    const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const node = ref.current
    // 不用 offsetParent 判可见：对话框位于 position:fixed 覆盖层内，浏览器对其后代返回 null，会导致焦点陷阱失效。
    const focusables = () => node ? Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => !el.hasAttribute('disabled')) : []
    const frame = requestAnimationFrame(() => {
      if (!node) return
      const first = focusables()[0]
      ;(first ?? node).focus()
    })
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onCloseRef.current()
        return
      }
      if (e.key !== 'Tab' || !node) return
      const items = focusables()
      if (items.length === 0) {
        e.preventDefault()
        node.focus()
        return
      }
      const first = items[0]
      const last = items[items.length - 1]
      const active = document.activeElement
      if (e.shiftKey && (active === first || !node.contains(active))) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (active === last || !node.contains(active))) {
        e.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('keydown', onKey)
      previouslyFocused?.focus()
    }
  }, [open, ref])
}

/** 危险/确认操作对话框：替代浏览器原生 confirm()，文案必须说明后果。 */
export function ConfirmDialog({ open, title, message, confirmText = '确认', cancelText = '取消', danger = false, loading = false, onConfirm, onCancel }: {
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
  return (
    <Dialog
      open={open}
      onClose={onCancel}
      title={title}
      role="alertdialog"
      footer={(
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onCancel} disabled={loading}>{cancelText}</Button>
          <Button type="button" variant={danger ? 'danger' : 'default'} onClick={onConfirm} disabled={loading} autoFocus>
            {loading ? '处理中…' : confirmText}
          </Button>
        </div>
      )}
    >
      <div className="space-y-3 text-sm text-foreground-soft">
        {typeof message === 'string' ? <p>{message}</p> : message}
      </div>
    </Dialog>
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

export function Empty({ icon, title, hint, action }: { icon?: ReactNode; title: string; hint?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 text-center">
      {icon && <div className="text-subtle">{icon}</div>}
      <p className="text-sm font-medium text-foreground-soft">{title}</p>
      {hint && <p className="max-w-sm text-xs text-muted">{hint}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  )
}

export function ErrorText({ children }: { children?: ReactNode }) {
  if (!children) return null
  return <p role="alert" className="rounded-md bg-danger-subtle px-3 py-2 text-xs text-danger-text">{children}</p>
}

export function SuccessText({ children }: { children?: ReactNode }) {
  if (!children) return null
  return <p role="status" className="text-xs text-success-text">{children}</p>
}

export function Th({ children, className = '' }: { children?: ReactNode; className?: string }) {
  return <th scope="col" className={`eg-cell-x eg-cell-y whitespace-nowrap text-left align-middle text-xs font-semibold uppercase tracking-wide text-muted ${className}`}>{children}</th>
}

export function Td({ children, className = '', title }: { children?: ReactNode; className?: string; title?: string }) {
  return <td className={`eg-cell-x eg-cell-y align-middle text-sm text-foreground-soft ${className}`} title={title}>{children}</td>
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
    <nav aria-label="分页" className={`flex flex-wrap items-center justify-between gap-2 border-t border-line-soft text-xs text-muted ${className}`} style={{ padding: 'var(--eg-cell-y) var(--eg-card-pad)' }}>
      <span className="tabular-nums">共 {total.toLocaleString('zh-CN')} 条 · 第 {page}/{totalPages} 页</span>
      <div className="flex flex-wrap items-center gap-2">
        {onPageSizeChange && (
          <label className="flex items-center gap-1.5">
            <span className="sr-only">每页条数</span>
            <select
              aria-label="每页条数"
              value={pageSize}
              onChange={(e) => onPageSizeChange(Number(e.target.value))}
              className="h-7 rounded-md border border-line-strong bg-surface px-2 text-xs text-foreground-soft outline-none focus:border-brand"
            >
              {sizes.map((s) => <option key={s} value={s}>{s} 条/页</option>)}
            </select>
          </label>
        )}
        <div className="flex items-center gap-1">
          <Button size="icon" variant="outline" aria-label="第一页" title="第一页" disabled={page <= 1} onClick={() => goto(1)}>«</Button>
          <Button size="icon" variant="outline" aria-label="上一页" title="上一页" disabled={page <= 1} onClick={() => goto(page - 1)}>‹</Button>
          {pages.map((p, i) => (
            p === null
              ? <span key={`gap-${i}`} className="px-1 text-subtle" aria-hidden="true">…</span>
              : (
                <Button
                  key={p}
                  size="icon"
                  variant={p === page ? 'default' : 'outline'}
                  aria-label={`第 ${p} 页`}
                  aria-current={p === page ? 'page' : undefined}
                  onClick={() => goto(p)}
                >
                  {p}
                </Button>
              )
          ))}
          <Button size="icon" variant="outline" aria-label="下一页" title="下一页" disabled={page >= totalPages} onClick={() => goto(page + 1)}>›</Button>
          <Button size="icon" variant="outline" aria-label="最后一页" title="最后一页" disabled={page >= totalPages} onClick={() => goto(totalPages)}>»</Button>
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

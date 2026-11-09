// shadcn 风格手写最小组件集（任务书 3.2 精神：可控、无重型依赖）
import { type ReactNode, type ButtonHTMLAttributes, type InputHTMLAttributes, useEffect } from 'react'
import { X } from 'lucide-react'

export function Button({ variant = 'default', size = 'md', className = '', ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'default' | 'outline' | 'ghost' | 'danger'; size?: 'sm' | 'md' }) {
  const base = 'inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors disabled:opacity-50 disabled:pointer-events-none'
  const variants = {
    default: 'bg-indigo-600 text-white hover:bg-indigo-700',
    outline: 'border border-slate-300 bg-white text-slate-700 hover:bg-slate-50',
    ghost: 'text-slate-600 hover:bg-slate-100',
    danger: 'bg-red-600 text-white hover:bg-red-700',
  }
  const sizes = { sm: 'h-7 px-2.5 text-xs', md: 'h-9 px-4 text-sm' }
  return <button className={`${base} ${variants[variant]} ${sizes[size]} ${className}`} {...props} />
}

export function Input({ className = '', ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none transition-colors placeholder:text-slate-400 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100 ${className}`}
      {...props}
    />
  )
}

export function Textarea({ className = '', ...props }: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={`w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm outline-none transition-colors placeholder:text-slate-400 focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100 ${className}`}
      {...props}
    />
  )
}

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <label className="block space-y-1.5">
      <span className="text-sm font-medium text-slate-700">{label}</span>
      {children}
      {hint && <span className="block text-xs text-slate-400">{hint}</span>}
    </label>
  )
}

export function Badge({ tone = 'bg-slate-100 text-slate-600', children }: { tone?: string; children: ReactNode }) {
  return <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${tone}`}>{children}</span>
}

export function Card({ className = '', children }: { className?: string; children: ReactNode }) {
  return <div className={`rounded-xl border border-slate-200 bg-white shadow-sm ${className}`}>{children}</div>
}

export function Spinner({ className = '' }: { className?: string }) {
  return <span className={`inline-block h-4 w-4 animate-spin rounded-full border-2 border-slate-300 border-t-indigo-600 ${className}`} />
}

export function Dialog({ open, onClose, title, children, wide }: { open: boolean; onClose: () => void; title: string; children: ReactNode; wide?: boolean }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    if (open) window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])
  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`max-h-[85vh] w-full ${wide ? 'max-w-2xl' : 'max-w-md'} overflow-auto rounded-xl bg-white shadow-xl`}>
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5">
          <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600"><X size={16} /></button>
        </div>
        <div className="px-5 py-4">{children}</div>
      </div>
    </div>
  )
}

export function Drawer({ open, onClose, title, children, wide }: { open: boolean; onClose: () => void; title: string; children: ReactNode; wide?: boolean }) {
  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-slate-900/40" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`flex h-full ${wide ? 'w-[560px]' : 'w-[420px]'} flex-col bg-white shadow-2xl`}>
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5">
          <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
          <button onClick={onClose} className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600"><X size={16} /></button>
        </div>
        <div className="flex-1 overflow-auto px-5 py-4">{children}</div>
      </div>
    </div>
  )
}

export function Empty({ icon, title, hint }: { icon?: ReactNode; title: string; hint?: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 text-center">
      {icon && <div className="text-slate-300">{icon}</div>}
      <p className="text-sm font-medium text-slate-600">{title}</p>
      {hint && <p className="max-w-sm text-xs text-slate-400">{hint}</p>}
    </div>
  )
}

export function ErrorText({ children }: { children?: ReactNode }) {
  if (!children) return null
  return <p className="rounded-md bg-red-50 px-3 py-2 text-xs text-red-600">{children}</p>
}

export function Th({ children, className = '' }: { children?: ReactNode; className?: string }) {
  return <th className={`px-3 py-2.5 text-left text-xs font-semibold uppercase tracking-wide text-slate-500 ${className}`}>{children}</th>
}

export function Td({ children, className = '', title }: { children?: ReactNode; className?: string; title?: string }) {
  return <td className={`px-3 py-2.5 text-sm text-slate-700 ${className}`} title={title}>{children}</td>
}

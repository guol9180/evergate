import type { ReactNode } from 'react'
import { Card } from './ui'
import { withBasePath } from '../lib/runtime'

/** 登录 / 首启设置 / 强制改密共用的居中卡片：Logo、标题、副标题、表单与页脚说明只有这一处版式。 */
export default function AuthCard({ title, subtitle, children, footer }: { title: ReactNode; subtitle?: ReactNode; children: ReactNode; footer?: ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center p-6">
      <Card className="w-full max-w-md bg-surface/95 p-6 shadow-xl backdrop-blur sm:p-8">
        <div className="mb-6 flex flex-col items-center gap-3">
          <span className="flex h-12 w-12 items-center justify-center rounded-xl border border-brand-border bg-brand-subtle">
            <img src={withBasePath('/logo.svg')} alt="" width={28} height={28} />
          </span>
          <h1 className="text-lg font-bold text-foreground">{title}</h1>
          {subtitle && <p className="text-center text-xs text-subtle">{subtitle}</p>}
        </div>
        {children}
        {footer && <div className="mt-4 flex items-center justify-center gap-1.5 text-[11px] text-subtle">{footer}</div>}
      </Card>
    </div>
  )
}

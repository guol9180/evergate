import type { TdHTMLAttributes, ThHTMLAttributes } from 'react'
import { cn } from '../../lib/utils'

export function Th({ children, className, title, ...props }: ThHTMLAttributes<HTMLTableCellElement>) {
  return <th scope="col" title={title} className={cn('eg-cell-x eg-cell-y whitespace-nowrap text-left align-middle text-xs font-semibold uppercase tracking-wide text-muted-foreground', className)} {...props}>{children}</th>
}

export function Td({ children, className, title, ...props }: TdHTMLAttributes<HTMLTableCellElement>) {
  return <td title={title} className={cn('eg-cell-x eg-cell-y align-middle text-sm text-foreground-soft', className)} {...props}>{children}</td>
}

import * as React from "react"
import { cn } from "cn"

function Input({ className, type, ...props }: React.ComponentProps<"input">) {
  return (
    <input
      type={type}
      data-slot="input"
      className={cn(
        // 与本项目既有控件同规格：h-9、md 圆角、面板底色；文本框聚焦一律显示品牌环（focus 而非 focus-visible）
        "h-9 w-full min-w-0 rounded-md border border-line-strong bg-surface px-3 py-1 text-sm text-foreground transition-colors outline-none placeholder:text-subtle focus:border-brand focus:ring-2 focus:ring-brand-subtle disabled:cursor-not-allowed disabled:bg-surface-strong disabled:text-subtle aria-invalid:border-danger-solid aria-invalid:focus:border-danger-solid aria-invalid:focus:ring-danger-subtle",
        className
      )}
      {...props}
    />
  )
}

export { Input }

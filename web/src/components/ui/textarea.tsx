import * as React from "react"
import { cn } from "cn"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        // 行数由调用点的 rows 决定，不启用 field-sizing-content 自动增高
        "w-full rounded-md border border-line-strong bg-surface px-3 py-2 text-sm text-foreground transition-colors outline-none placeholder:text-subtle focus:border-brand focus:ring-2 focus:ring-brand-subtle disabled:cursor-not-allowed disabled:bg-surface-strong disabled:text-subtle aria-invalid:border-danger-solid aria-invalid:focus:border-danger-solid aria-invalid:focus:ring-danger-subtle",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }

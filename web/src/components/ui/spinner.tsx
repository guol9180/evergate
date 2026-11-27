import { cn } from "cn"

export function Spinner({ size = "md", tone = "brand", className = "" }: { size?: "sm" | "md"; tone?: "brand" | "current"; className?: string }) {
  const dims = size === "sm" ? "h-3.5 w-3.5 border-[1.5px]" : "h-4 w-4 border-2"
  const colors = tone === "current" ? "border-current border-t-transparent" : "border-line-strong border-t-brand"
  return <span aria-hidden="true" className={cn("inline-block animate-spin rounded-full", dims, colors, className)} />
}

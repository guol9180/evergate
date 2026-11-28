"use client"

import * as React from "react"
import { createContext, useContext } from "react"
import { cn } from "cn"
import { Tooltip as TooltipPrimitive } from "radix-ui"

function TooltipProvider({
  delayDuration = 0,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipPrimitive.Provider
      data-slot="tooltip-provider"
      delayDuration={delayDuration}
      {...props}
    />
  )
}

function Tooltip({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />
}

function TooltipTrigger({
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />
}

function TooltipContent({
  className,
  sideOffset = 0,
  children,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          "z-50 inline-flex w-fit max-w-xs origin-(--radix-tooltip-content-transform-origin) items-center gap-1.5 rounded-md bg-foreground px-3 py-1.5 text-xs text-background has-data-[slot=kbd]:pr-1.5 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 **:data-[slot=kbd]:relative **:data-[slot=kbd]:isolate **:data-[slot=kbd]:z-50 **:data-[slot=kbd]:rounded-sm data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-[state=delayed-open]:zoom-in-95 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95",
          className
        )}
        {...props}
      >
        {children}
        <TooltipPrimitive.Arrow className="z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-foreground fill-foreground" />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  )
}

/** 根部是否已挂 TooltipProvider：有则共用一份延迟配置，没有（测试 / 独立渲染）就地补一个。 */
const TooltipRootContext = createContext(false)

/** 应用根部的 Tooltip Provider（main.tsx 挂一次），统一延迟与跳过重叠交互的设置。 */
function TooltipRootProvider({
  children,
  delayDuration = 200,
}: {
  children: React.ReactNode
  delayDuration?: number
}) {
  return (
    <TooltipRootContext.Provider value>
      <TooltipProvider delayDuration={delayDuration}>{children}</TooltipProvider>
    </TooltipRootContext.Provider>
  )
}

/**
 * 控件说明 tooltip（项目唯一出处）：图标按钮 / 侧栏按钮统一走这里；数据截断预览仍用原生 title。
 * 无 content 时直接透传 children；根部没挂 Provider 时就地补一个，页面与测试都不必关心装配。
 */
function TooltipWrap({
  content,
  side = "top",
  children,
  className,
}: {
  content?: React.ReactNode
  side?: "top" | "right" | "bottom" | "left"
  children: React.ReactNode
  className?: string
}) {
  const hasRootProvider = useContext(TooltipRootContext)
  if (!content) return <>{children}</>
  const tree = (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side={side} sideOffset={6} className={cn("eg-tooltip", className)}>
        {content}
      </TooltipContent>
    </Tooltip>
  )
  return hasRootProvider ? tree : <TooltipRootProvider>{tree}</TooltipRootProvider>
}

export {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
  TooltipRootProvider,
  TooltipWrap,
}

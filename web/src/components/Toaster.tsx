// 全站 Toast 唯一出处（sonner）：瞬时操作结果（保存 / 删除 / 导入 / 导出）走这里；
// 需要持续阅读的错误用 Alert / FieldError。颜色由 index.css 映射到语义 token。
import type { ReactNode } from 'react'
import { Toaster as Sonner, toast, type ExternalToast } from 'sonner'
import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from 'lucide-react'
import { t, useLocale } from '../lib/i18n'
import { useResolvedTheme } from '../lib/theme'

/** 错误提示停留更久：用户需要读完才能修正。 */
const ERROR_DURATION = 8000

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message
  if (typeof error === 'string') return error
  return ''
}

export const notify = {
  success: (title: ReactNode, options?: ExternalToast) => toast.success(title, options),
  info: (title: ReactNode, options?: ExternalToast) => toast.info(title, options),
  warning: (title: ReactNode, options?: ExternalToast) => toast.warning(title, options),
  /** 统一错误：标题「操作失败」，描述取服务端 / 异常消息；为空时只显示标题。 */
  error: (error: unknown, title?: ReactNode) => {
    const description = messageOf(error)
    return toast.error(title ?? t('toast.failed'), { description: description || undefined, duration: ERROR_DURATION })
  },
}

export function Toaster() {
  const theme = useResolvedTheme()
  // 订阅语言切换：关闭按钮 / 区域的 aria 文案跟随当前语言
  useLocale()
  return (
    <Sonner
      theme={theme}
      position="top-right"
      richColors
      closeButton
      gap={8}
      offset={16}
      icons={{
        success: <CircleCheck size={16} />,
        info: <Info size={16} />,
        warning: <TriangleAlert size={16} />,
        error: <CircleAlert size={16} />,
        close: <X size={14} />,
      }}
      containerAriaLabel={t('toast.region')}
      toastOptions={{ closeButtonAriaLabel: t('common.close') }}
      className="eg-toaster"
    />
  )
}

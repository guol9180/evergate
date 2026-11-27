import { MutationCache } from '@tanstack/react-query'
import { notify } from '../components/Toaster'

declare module '@tanstack/react-query' {
  interface Register {
    /** mutation.meta 类型：页面已内联展示错误（表单顶部 Alert / 字段错误）时跳过全局 Toast */
    mutationMeta: { silentError?: boolean }
  }
}

/**
 * mutation 失败统一反馈的唯一出处：默认弹 Toast；页面自己渲染内联错误的 mutation 用
 * meta.silentError 退出。页面不再逐处手写 onError。
 */
export function createMutationCache() {
  return new MutationCache({
    onError: (error, _variables, _context, mutation) => {
      if (!mutation.meta?.silentError) notify.error(error)
    },
  })
}

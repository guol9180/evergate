// 主题切换（T76）：light / dark / system，持久化到 localStorage；.dark 类由 index.css 的 @custom-variant 消费。
import { useSyncExternalStore } from 'react'

export type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'eg-theme'
const THEME_EVENT = 'eg-theme-change'

export function getTheme(): Theme {
  if (typeof window === 'undefined') return 'system'
  const stored = window.localStorage.getItem(STORAGE_KEY)
  return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
}

export function setTheme(theme: Theme): Theme {
  window.localStorage.setItem(STORAGE_KEY, theme)
  applyTheme(theme)
  return theme
}

/** 解析 system 的实际值，用于切换按钮显示当前生效模式。 */
export function resolvedTheme(theme: Theme = getTheme()): 'light' | 'dark' {
  if (theme === 'system') {
    return typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  return theme
}

export function applyTheme(theme: Theme = getTheme()) {
  const root = document.documentElement
  root.classList.toggle('dark', resolvedTheme(theme) === 'dark')
  // Toast 等挂在根部的三方组件不读 localStorage，靠这个事件跟随 .dark 变化重渲染
  window.dispatchEvent(new Event(THEME_EVENT))
}

/** 当前实际生效的浅 / 深色（含 system 解析结果），供需要显式 theme 属性的三方组件使用。 */
export function useResolvedTheme(): 'light' | 'dark' {
  return useSyncExternalStore(
    (listener) => {
      window.addEventListener(THEME_EVENT, listener)
      return () => window.removeEventListener(THEME_EVENT, listener)
    },
    () => (document.documentElement.classList.contains('dark') ? 'dark' : 'light'),
    () => 'light',
  )
}

let listenerBound = false

/** 应用启动与 system 变化时同步 .dark；只绑定一次 matchMedia 监听（jsdom 等无 matchMedia 环境跳过监听）。 */
export function initTheme() {
  applyTheme()
  if (listenerBound || typeof window === 'undefined' || typeof window.matchMedia !== 'function') return
  listenerBound = true
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (getTheme() === 'system') applyTheme('system')
  })
}


// 主题切换（T76）：light / dark / system，持久化到 localStorage；.dark 类由 index.css 的 @custom-variant 消费。
export type Theme = 'light' | 'dark' | 'system'

const STORAGE_KEY = 'eg-theme'

export function getTheme(): Theme {
  if (typeof window === 'undefined') return 'system'
  const stored = window.localStorage.getItem(STORAGE_KEY)
  return stored === 'light' || stored === 'dark' || stored === 'system' ? stored : 'system'
}

export function setTheme(theme: Theme) {
  window.localStorage.setItem(STORAGE_KEY, theme)
  applyTheme(theme)
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

/** 循环切换 light → dark → system，返回新主题。 */
export function cycleTheme(theme: Theme = getTheme()): Theme {
  const next: Record<Theme, Theme> = { light: 'dark', dark: 'system', system: 'light' }
  setTheme(next[theme])
  return next[theme]
}

import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { ExternalLink, LayoutDashboard, Waypoints, KeyRound, ScrollText, Settings, LogOut, BarChart3, FlaskConical, UserRound, Sun, Moon, Monitor, Languages } from 'lucide-react'
import { useLogout, useMe, useUpdateStatus } from '../api/hooks'
import { PageSkeleton, Tooltip } from './ui'
import { withBasePath } from '../lib/runtime'
import { cycleTheme, getTheme, initTheme, resolvedTheme, type Theme } from '../lib/theme'
import { getLocale, setLocale, t, tf, useLocale, type MessageKey } from '../lib/i18n'
import { useMediaQuery } from '../lib/useMediaQuery'

const nav: { to: string; label: MessageKey; end?: boolean; icon: typeof LayoutDashboard }[] = [
  { to: '/', label: 'nav.dashboard', icon: LayoutDashboard, end: true },
  { to: '/channels', label: 'nav.channels', icon: Waypoints },
  { to: '/tokens', label: 'nav.tokens', icon: KeyRound },
  { to: '/logs', label: 'nav.logs', icon: ScrollText },
  { to: '/playground', label: 'nav.playground', icon: FlaskConical },
  { to: '/usage', label: 'nav.usage', icon: BarChart3 },
  { to: '/settings', label: 'nav.settings', icon: Settings },
]

const themeIcon = (theme: Theme, dark: boolean) => {
  if (theme === 'system') return Monitor
  return dark ? Moon : Sun
}
const themeLabelKey: Record<Theme, MessageKey> = { light: 'theme.light', dark: 'theme.dark', system: 'theme.system' }
const nextTheme: Record<Theme, Theme> = { light: 'dark', dark: 'system', system: 'light' }

export default function Layout() {
  const me = useMe()
  const update = useUpdateStatus(!me.isPending && !me.data?.must_change_password)
  const logout = useLogout()
  const navigate = useNavigate()
  const location = useLocation()
  const [theme, setThemeState] = useState<Theme>(() => getTheme())
  const mainRef = useRef<HTMLElement>(null)
  // 与 index.css 的侧栏断点一致：≥1200px 显示文字标签，此时退出按钮不需要 tooltip
  const sidebarExpanded = useMediaQuery('(min-width: 1200px)')
  // 订阅语言：setLocale 后本组件与所有调用 useLocale 的页面一起重渲染
  useLocale()

  useEffect(() => {
    initTheme()
  }, [])

  useEffect(() => {
    if (me.isError) navigate('/login', { replace: true })
  }, [me.isError, navigate])

  useEffect(() => {
    if (me.data?.must_change_password && location.pathname !== '/change-password') {
      navigate('/change-password', { replace: true })
    }
  }, [me.data, location.pathname, navigate])

  // 路由切换时归零 main 滚动位置，避免新页面沿用上一页的偏移（标题被顶出视口）
  useEffect(() => {
    if (mainRef.current) mainRef.current.scrollTop = 0
  }, [location.pathname])

  if (me.isPending) {
    return (
      <div className="eg-page" role="status" aria-label={t('common.loading')}>
        <PageSkeleton />
      </div>
    )
  }

  const dark = resolvedTheme(theme) === 'dark'
  const ThemeIcon = themeIcon(theme, dark)

  return (
    <div className="eg-shell flex h-full min-w-0">
      <aside className="eg-sidebar flex shrink-0 flex-col border-r border-line bg-surface/95 backdrop-blur">
        <div className="eg-sidebar-brand flex items-center gap-3 px-3 py-5">
          <div className="eg-sidebar-brand-logo flex shrink-0 items-center">
            <img src={withBasePath('/logo.svg')} alt="" width={20} height={20} className="shrink-0" />
          </div>
          <div className="eg-sidebar-meta min-w-0">
            <div className="text-sm font-bold text-foreground">{t('app.name')}</div>
            <div className="text-[10px] text-subtle">{t('app.tagline')}</div>
          </div>
        </div>
        <nav className="flex-1 space-y-1 px-3" aria-label={t('nav.main')}>
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              title={t(item.label)}
              className={({ isActive }) =>
                `eg-nav-link flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${isActive ? 'bg-brand-subtle font-medium text-brand-text' : 'text-muted hover:bg-surface-soft'}`
              }
            >
              <item.icon size={16} aria-hidden="true" />
              <span className="eg-sidebar-label">{t(item.label)}</span>
            </NavLink>
          ))}
        </nav>
        <div className="space-y-3 border-t border-line-soft p-3">
          {/* 与导航项同缩进：px-3 + 16px 图标，保证图标中心与文字左缘都和菜单对齐 */}
          <div className="eg-sidebar-meta flex items-center gap-2.5 px-3 text-xs text-muted">
            <UserRound size={16} aria-hidden="true" className="shrink-0" />
            <span className="truncate">{me.data?.username}</span>
          </div>
          <div className="eg-sidebar-meta px-3 text-[11px] leading-4 text-subtle">
            {update.isPending
              ? t('update.checking')
              : update.isError
                ? t('update.failed')
                : update.data?.development
                  ? t('update.dev')
                  : tf('update.current', { version: String(update.data?.current_version ?? t('common.unknown')) })}
            {update.data?.update_available && (
              <a href={update.data.release_url} target="_blank" rel="noreferrer" className="mt-1 flex items-center gap-1 font-medium text-brand-text hover:underline">
                {tf('update.get', { version: String(update.data.latest_version) })} <ExternalLink size={11} aria-hidden="true" />
              </a>
            )}
          </div>
          <div className="eg-sidebar-actions">
            <Tooltip content={t('locale.switch')} side="right">
              <button
                type="button"
                onClick={() => setLocale(getLocale() === 'en' ? 'zh-CN' : 'en')}
                aria-label={t('locale.switch')}
                className="eg-nav-link eg-icon-btn text-muted hover:bg-surface-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <Languages size={16} className="shrink-0" aria-hidden="true" />
              </button>
            </Tooltip>
            <Tooltip content={tf('theme.title', { current: t(themeLabelKey[theme]) })} side="right">
              <button
                type="button"
                onClick={() => setThemeState(cycleTheme(theme))}
                aria-label={tf('theme.aria', { current: t(themeLabelKey[theme]), next: t(themeLabelKey[nextTheme[theme]]) })}
                className="eg-nav-link eg-icon-btn text-muted hover:bg-surface-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <ThemeIcon size={16} className="shrink-0" aria-hidden="true" />
              </button>
            </Tooltip>
            <Tooltip content={sidebarExpanded ? undefined : t('nav.logout')} side="right">
              <button
                type="button"
                onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login') })}
                className="eg-nav-link eg-icon-btn eg-sidebar-logout gap-2.5 text-sm text-muted hover:bg-surface-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
              >
                <LogOut size={16} className="shrink-0" aria-hidden="true" /> <span className="eg-sidebar-label">{t('nav.logout')}</span>
              </button>
            </Tooltip>
          </div>
        </div>
      </aside>
      <main ref={mainRef} className="min-w-0 flex-1 overflow-auto" id="main-content">
        {/* key 绑定路径：切换页面时重挂内容区以触发入场动画（页面本身在路由切换时就会重挂） */}
        <div key={location.pathname} className="eg-page eg-page-in">
          <Outlet />
        </div>
      </main>
    </div>
  )
}

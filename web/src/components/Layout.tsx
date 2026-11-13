import { useEffect } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { ExternalLink, LayoutDashboard, Waypoints, KeyRound, ScrollText, Settings, LogOut, BarChart3, FlaskConical } from 'lucide-react'
import { useLogout, useMe, useUpdateStatus } from '../api/hooks'
import { Spinner } from './ui'

const nav = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/channels', label: '渠道管理', icon: Waypoints },
  { to: '/tokens', label: '令牌管理', icon: KeyRound },
  { to: '/logs', label: '请求日志', icon: ScrollText },
  { to: '/playground', label: 'Playground', icon: FlaskConical },
  { to: '/usage', label: '用量成本', icon: BarChart3 },
  { to: '/settings', label: '系统设置', icon: Settings },
]

export default function Layout() {
  const me = useMe()
  const update = useUpdateStatus(!me.isPending && !me.data?.must_change_password)
  const logout = useLogout()
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    if (me.isError) navigate('/login', { replace: true })
  }, [me.isError, navigate])

  useEffect(() => {
    if (me.data?.must_change_password && location.pathname !== '/change-password') {
      navigate('/change-password', { replace: true })
    }
  }, [me.data, location.pathname, navigate])

  if (me.isPending) {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-slate-500">
        <Spinner /> 加载中…
      </div>
    )
  }

  return (
    <div className="flex h-full">
      <aside className="flex w-[72px] shrink-0 flex-col border-r border-slate-200/80 bg-white/95 backdrop-blur xl:w-56">
        <div className="flex items-center gap-3 px-4 py-5 xl:px-5">
          <img src="/logo.svg" alt="" width={20} height={20} className="shrink-0" />
          <div className="hidden xl:block">
            <div className="text-sm font-bold text-slate-900">EverGate</div>
            <div className="text-[10px] text-slate-400">AI 网关管理台</div>
          </div>
        </div>
        <nav className="flex-1 space-y-1 px-3">
          {nav.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `flex items-center justify-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors xl:justify-start ${isActive ? 'bg-indigo-50 font-medium text-indigo-700' : 'text-slate-600 hover:bg-slate-50'}`
              }
            >
              <item.icon size={16} />
              <span className="hidden xl:inline">{item.label}</span>
            </NavLink>
          ))}
        </nav>
        <div className="space-y-3 border-t border-slate-100 p-3">
          <div className="hidden px-2 text-xs text-slate-500 xl:block">👤 {me.data?.username}</div>
          <div className="hidden px-2 text-[11px] leading-4 text-slate-400 xl:block">
            {update.isPending
              ? '正在检查更新…'
              : update.isError
                ? '更新检查失败，可稍后重试'
                : update.data?.development
                  ? '开发版本，更新检查已跳过'
                  : `当前 ${update.data?.current_version ?? '未知'}`}
            {update.data?.update_available && (
              <a href={update.data.release_url} target="_blank" rel="noreferrer" className="mt-1 flex items-center gap-1 font-medium text-indigo-600 hover:underline">
                获取 {update.data.latest_version} <ExternalLink size={11} />
              </a>
            )}
          </div>
          <button
            onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login') })}
            title="退出登录"
            className="flex w-full items-center justify-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-600 hover:bg-slate-50 xl:justify-start"
          >
            <LogOut size={16} /> 退出登录
          </button>
        </div>
      </aside>
      <main className="flex-1 overflow-auto">
        <div className="mx-auto max-w-7xl px-5 py-6 xl:px-8">
          <Outlet />
        </div>
      </main>
    </div>
  )
}

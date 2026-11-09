import { useEffect } from 'react'
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { LayoutDashboard, Waypoints, KeyRound, ScrollText, LogOut, TowerControl } from 'lucide-react'
import { useLogout, useMe } from '../api/hooks'
import { Spinner } from './ui'

const nav = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard, end: true },
  { to: '/channels', label: '渠道管理', icon: Waypoints },
  { to: '/tokens', label: '令牌管理', icon: KeyRound },
  { to: '/logs', label: '请求日志', icon: ScrollText },
]

export default function Layout() {
  const me = useMe()
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
      <aside className="flex w-52 shrink-0 flex-col border-r border-slate-200 bg-white">
        <div className="flex items-center gap-2 px-5 py-4">
          <TowerControl size={20} className="text-indigo-600" />
          <div>
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
                `flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors ${isActive ? 'bg-indigo-50 font-medium text-indigo-700' : 'text-slate-600 hover:bg-slate-50'}`
              }
            >
              <item.icon size={16} />
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="border-t border-slate-100 p-3">
          <div className="mb-2 px-2 text-xs text-slate-500">👤 {me.data?.username}</div>
          <button
            onClick={() => logout.mutate(undefined, { onSuccess: () => navigate('/login') })}
            className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-slate-600 hover:bg-slate-50"
          >
            <LogOut size={16} /> 退出登录
          </button>
        </div>
      </aside>
      <main className="flex-1 overflow-auto">
        <div className="mx-auto max-w-6xl px-6 py-6">
          <Outlet />
        </div>
      </main>
    </div>
  )
}

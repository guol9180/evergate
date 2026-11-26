import { type FormEvent, useRef, useState } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import { useLogin, useSetupStatus } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Alert, Button, Field, Input } from '../components/ui'
import AuthCard from '../components/AuthCard'

export default function Login() {
  usePageTitle(t('page.login.title'))
  const login = useLogin()
  const setup = useSetupStatus()
  const navigate = useNavigate()
  const location = useLocation()
  const prefill = (location.state as { username?: string } | null)?.username ?? ''
  const [username, setUsername] = useState(prefill)
  const [password, setPassword] = useState('')
  const passwordRef = useRef<HTMLInputElement>(null)

  // 尚未创建管理员时先走首启设置
  if (setup.data?.setup_required) return <Navigate to="/setup" replace />

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    login.mutate(
      { username, password },
      {
        onSuccess: (data) => navigate(data.must_change_password ? '/change-password' : '/', { replace: true }),
        // 凭据错误最常见的原因是密码：选中密码框，直接重输即可
        onError: () => passwordRef.current?.select(),
      },
    )
  }

  return (
    <AuthCard title={t('login.heading')} subtitle="One gate, every model, zero interruption">
      <form onSubmit={onSubmit} className="space-y-4">
        {/* 提交错误放在表单顶部：先读原因再扫字段，而不是夹在密码框和按钮之间 */}
        {login.isError && <Alert tone="danger">{(login.error as Error).message}</Alert>}
        <Field label={t('login.username')}>
          <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="admin" autoComplete="username" />
        </Field>
        <Field label={t('login.password')}>
          <Input ref={passwordRef} type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" autoComplete="current-password" />
        </Field>
        <Button type="submit" className="w-full" disabled={login.isPending}>
          {login.isPending ? t('login.submitting') : t('login.submit')}
        </Button>
      </form>
    </AuthCard>
  )
}

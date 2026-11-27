import { type FormEvent, useRef, useState } from 'react'
import { Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useLogin, useSetupStatus } from '../api/hooks'
import { safeNext } from '../lib/session'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Alert, Button, Field, Input, focusFirstInvalid } from '../components/ui'
import AuthCard from '../components/AuthCard'

type FieldErrors = { username?: string; password?: string }

export default function Login() {
  usePageTitle(t('page.login.title'))
  const login = useLogin()
  const setup = useSetupStatus()
  const navigate = useNavigate()
  const location = useLocation()
  const [params] = useSearchParams()
  // 会话过期跳来时带回跳地址；只接受站内路径
  const nextPath = safeNext(params.get('next'))
  const redirected = nextPath !== '/'
  const prefill = (location.state as { username?: string } | null)?.username ?? ''
  const [username, setUsername] = useState(prefill)
  const [password, setPassword] = useState('')
  const [errors, setErrors] = useState<FieldErrors>({})
  const formRef = useRef<HTMLFormElement>(null)
  const passwordRef = useRef<HTMLInputElement>(null)

  // 尚未创建管理员时先走首启设置
  if (setup.data?.setup_required) return <Navigate to="/setup" replace />

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    // 空表单在前端拦下：不让一次点击跑到服务端再回一句「参数错误」
    const next: FieldErrors = {}
    if (!username.trim()) next.username = t('login.usernameRequired')
    if (!password) next.password = t('login.passwordRequired')
    setErrors(next)
    if (next.username || next.password) {
      focusFirstInvalid(formRef.current)
      return
    }
    login.mutate(
      { username: username.trim(), password },
      {
        onSuccess: (data) => navigate(data.must_change_password ? '/change-password' : nextPath, { replace: true, state: { next: nextPath } }),
        // 凭据错误最常见的原因是密码：选中密码框，直接重输即可
        onError: () => passwordRef.current?.select(),
      },
    )
  }

  return (
    <AuthCard title={t('login.heading')} subtitle="One gate, every model, zero interruption">
      <form ref={formRef} onSubmit={onSubmit} className="space-y-4" noValidate>
        {redirected && <Alert tone="info">{t('session.expired')}</Alert>}
        {/* 提交错误放在表单顶部：先读原因再扫字段，而不是夹在密码框和按钮之间 */}
        {login.isError && <Alert tone="danger">{(login.error as Error).message}</Alert>}
        <Field label={t('login.username')} error={errors.username}>
          <Input value={username} onChange={(e) => { setUsername(e.target.value); setErrors((current) => ({ ...current, username: undefined })) }} placeholder="admin" autoComplete="username" />
        </Field>
        <Field label={t('login.password')} error={errors.password}>
          <Input ref={passwordRef} type="password" value={password} onChange={(e) => { setPassword(e.target.value); setErrors((current) => ({ ...current, password: undefined })) }} placeholder="••••••••" autoComplete="current-password" />
        </Field>
        <Button type="submit" className="w-full" loading={login.isPending}>
          {t('login.submit')}
        </Button>
      </form>
    </AuthCard>
  )
}

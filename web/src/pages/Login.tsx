import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Navigate, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { useLogin, useSetupStatus } from '../api/hooks'
import { safeNext } from '../lib/session'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Alert, Button, Field, Input } from '../components/ui'
import AuthCard from '../components/AuthCard'
import { errorMessage } from '../lib/errors'

// 登录表单：zod schema 即校验规则与入参类型，文案走 i18n；空表单在前端拦下，不浪费一次请求
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
  const schema = z.object({
    username: z.string().trim().min(1, t('login.usernameRequired')),
    password: z.string().min(1, t('login.passwordRequired')),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { username: prefill, password: '' },
  })
  const { register, handleSubmit, formState, setFocus, setError } = form

  // 尚未创建管理员时先走首启设置
  if (setup.data?.setup_required) return <Navigate to="/setup" replace />

  const onSubmit = handleSubmit(
    (values) =>
      login.mutate(
        { username: values.username.trim(), password: values.password },
        {
          onSuccess: (data) => navigate(data.must_change_password ? '/change-password' : nextPath, { replace: true, state: { next: nextPath } }),
          // 凭据错误最常见的原因是密码：选中密码框，直接重输即可
          onError: () => {
            setError('password', { type: 'server' })
            setFocus('password', { shouldSelect: true })
          },
        },
      ),
    // 校验失败：焦点落到第一个出错字段
    (errors) => setFocus(Object.keys(errors)[0] as 'username' | 'password'),
  )

  return (
    <AuthCard title={t('login.heading')} subtitle="One gate, every model, zero interruption">
      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {redirected && <Alert tone="info">{t('session.expired')}</Alert>}
        {/* 提交错误放在表单顶部：先读原因再扫字段，而不是夹在密码框和按钮之间 */}
        {login.isError && <Alert tone="danger">{errorMessage(login.error)}</Alert>}
        <Field label={t('login.username')} error={formState.errors.username?.message}>
          <Input {...register('username')} placeholder="admin" autoComplete="username" />
        </Field>
        <Field label={t('login.password')} error={formState.errors.password?.message}>
          <Input {...register('password')} type="password" placeholder="••••••••" autoComplete="current-password" />
        </Field>
        <Button type="submit" className="w-full" loading={login.isPending}>
          {t('login.submit')}
        </Button>
      </form>
    </AuthCard>
  )
}
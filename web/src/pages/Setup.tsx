import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Navigate } from 'react-router-dom'
import { ShieldCheck } from 'lucide-react'
import { useSetupAdmin, useSetupStatus } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Alert, Button, Field, Input, Spinner } from '../components/ui'
import AuthCard from '../components/AuthCard'
import { errorMessage } from '../lib/errors'

const usernamePattern = /^[A-Za-z0-9_.-]{3,32}$/

/** 与后端 store.ValidateAdminPassword 对齐：大写 / 小写 / 数字 / 符号至少三类。 */
function passwordClasses(value: string): number {
  return [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((re) => re.test(value)).length
}

/** 首启设置管理员：schema 即校验规则（用户名格式 / 密码强度 / 两次一致），一次性收集全部字段错误。 */
export default function Setup() {
  usePageTitle(t('page.setup.title'))
  const status = useSetupStatus()
  const setup = useSetupAdmin()
  const schema = z
    .object({
      username: z.string().trim().regex(usernamePattern, t('setup.usernameInvalid')),
      password: z.string().min(8, t('setup.tooShort')).refine((value) => passwordClasses(value) >= 3, t('setup.weak')),
      confirm: z.string(),
    })
    .refine((values) => values.password === values.confirm, { message: t('setup.mismatch'), path: ['confirm'] })
  const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { username: 'admin', password: '', confirm: '' } })
  const { register, handleSubmit, formState, setFocus } = form

  if (status.isPending) {
    return (
      <div className="flex h-full items-center justify-center p-6" role="status">
        <Spinner className="h-6 w-6" />
      </div>
    )
  }
  // 刚创建成功时优先用响应里的用户名回填登录页；放前面避免被"已完成设置"分支抢先跳走而丢掉 state
  if (setup.data) return <Navigate to="/login" replace state={{ username: setup.data.username }} />
  if (status.data && !status.data.setup_required) return <Navigate to="/login" replace />

  const onSubmit = handleSubmit(
    (values) => setup.mutate({ username: values.username.trim(), password: values.password }),
    (errors) => setFocus(Object.keys(errors)[0] as 'username' | 'password' | 'confirm'),
  )

  const serverError = setup.isError ? errorMessage(setup.error) : status.isError ? errorMessage(status.error) : ''

  return (
    <AuthCard
      title={t('setup.heading')}
      subtitle={t('setup.hint')}
      footer={<><ShieldCheck size={13} className="shrink-0" aria-hidden="true" /> {t('setup.loopbackNote')}</>}
    >
      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {serverError && <Alert tone="danger">{serverError}</Alert>}
        <Field label={t('setup.username')} hint={t('setup.usernameHint')} error={formState.errors.username?.message}>
          <Input {...register('username')} autoComplete="username" autoFocus />
        </Field>
        <Field label={t('setup.password')} hint={t('setup.passwordHint')} error={formState.errors.password?.message}>
          <Input type="password" {...register('password')} autoComplete="new-password" />
        </Field>
        <Field label={t('setup.confirm')} error={formState.errors.confirm?.message}>
          <Input type="password" {...register('confirm')} autoComplete="new-password" />
        </Field>
        <Button type="submit" className="w-full" loading={setup.isPending}>
          {t('setup.submit')}
        </Button>
      </form>
    </AuthCard>
  )
}
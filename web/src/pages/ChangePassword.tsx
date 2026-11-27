import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { useLocation, useNavigate } from 'react-router-dom'
import { useChangePassword } from '../api/hooks'
import { safeNext } from '../lib/session'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Alert, Button, Field, Input } from '../components/ui'
import { notify } from '../components/Toaster'
import AuthCard from '../components/AuthCard'
import { errorMessage } from '../lib/errors'

export default function ChangePassword() {
  usePageTitle(t('page.changePassword.title'))
  const change = useChangePassword()
  const navigate = useNavigate()
  const location = useLocation()
  // 会话过期 → 登录 → 强制改密：改完仍回到最初要去的页面
  const nextPath = safeNext((location.state as { next?: string } | null)?.next)
  const schema = z
    .object({
      old_password: z.string(),
      new_password: z.string().min(8, t('changePassword.tooShort')),
      confirm: z.string(),
    })
    .refine((values) => values.new_password === values.confirm, { message: t('changePassword.mismatch'), path: ['confirm'] })
  const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { old_password: '', new_password: '', confirm: '' } })
  const { register, handleSubmit, formState, setFocus } = form

  const onSubmit = handleSubmit(
    (values) =>
      change.mutate(
        { old_password: values.old_password, new_password: values.new_password },
        {
          onSuccess: () => {
            notify.success(t('toast.password.changed'))
            navigate(nextPath, { replace: true })
          },
        },
      ),
    (errors) => setFocus(Object.keys(errors)[0] as 'old_password' | 'new_password' | 'confirm'),
  )

  return (
    <AuthCard title={t('changePassword.heading')} subtitle={t('changePassword.hint')}>
      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {change.isError && <Alert tone="danger">{errorMessage(change.error)}</Alert>}
        <Field label={t('changePassword.old')}>
          <Input type="password" {...register('old_password')} autoComplete="current-password" autoFocus />
        </Field>
        <Field label={t('changePassword.new')} error={formState.errors.new_password?.message}>
          <Input type="password" {...register('new_password')} autoComplete="new-password" />
        </Field>
        <Field label={t('changePassword.confirm')} error={formState.errors.confirm?.message}>
          <Input type="password" {...register('confirm')} autoComplete="new-password" />
        </Field>
        <Button type="submit" className="w-full" loading={change.isPending}>
          {t('changePassword.submit')}
        </Button>
      </form>
    </AuthCard>
  )
}
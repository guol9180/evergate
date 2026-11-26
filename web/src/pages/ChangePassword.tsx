import { type FormEvent, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useChangePassword } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Alert, Button, Field, Input, focusFirstInvalid } from '../components/ui'
import { notify } from '../components/Toaster'
import AuthCard from '../components/AuthCard'

type FieldErrors = { newPassword?: string; confirm?: string }

export default function ChangePassword() {
  usePageTitle(t('page.changePassword.title'))
  const change = useChangePassword()
  const navigate = useNavigate()
  const formRef = useRef<HTMLFormElement>(null)
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirm, setConfirm] = useState('')
  const [errors, setErrors] = useState<FieldErrors>({})

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const next: FieldErrors = {}
    if (newPwd.length < 8) next.newPassword = t('changePassword.tooShort')
    if (newPwd !== confirm) next.confirm = t('changePassword.mismatch')
    setErrors(next)
    if (Object.keys(next).length > 0) {
      focusFirstInvalid(formRef.current)
      return
    }
    change.mutate(
      { old_password: oldPwd, new_password: newPwd },
      {
        onSuccess: () => {
          notify.success(t('toast.password.changed'))
          navigate('/', { replace: true })
        },
      },
    )
  }

  return (
    <AuthCard title={t('changePassword.heading')} subtitle={t('changePassword.hint')}>
      <form ref={formRef} onSubmit={onSubmit} className="space-y-4" noValidate>
        {change.isError && <Alert tone="danger">{(change.error as Error).message}</Alert>}
        <Field label={t('changePassword.old')}>
          <Input type="password" value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} autoComplete="current-password" autoFocus />
        </Field>
        <Field label={t('changePassword.new')} error={errors.newPassword}>
          <Input type="password" value={newPwd} onChange={(e) => { setNewPwd(e.target.value); setErrors((current) => ({ ...current, newPassword: undefined })) }} autoComplete="new-password" />
        </Field>
        <Field label={t('changePassword.confirm')} error={errors.confirm}>
          <Input type="password" value={confirm} onChange={(e) => { setConfirm(e.target.value); setErrors((current) => ({ ...current, confirm: undefined })) }} autoComplete="new-password" />
        </Field>
        <Button type="submit" className="w-full" loading={change.isPending}>
          {t('changePassword.submit')}
        </Button>
      </form>
    </AuthCard>
  )
}

import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ShieldAlert } from 'lucide-react'
import { useChangePassword } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Button, Card, ErrorText, Field, Input } from '../components/ui'

export default function ChangePassword() {
  usePageTitle(t('page.changePassword.title'))
  const change = useChangePassword()
  const navigate = useNavigate()
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirm, setConfirm] = useState('')
  const [localError, setLocalError] = useState('')

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (newPwd.length < 8) {
      setLocalError(t('changePassword.tooShort'))
      return
    }
    if (newPwd !== confirm) {
      setLocalError(t('changePassword.mismatch'))
      return
    }
    setLocalError('')
    change.mutate(
      { old_password: oldPwd, new_password: newPwd },
      { onSuccess: () => navigate('/', { replace: true }) },
    )
  }

  return (
    <div className="flex h-full items-center justify-center p-6">
      <Card className="w-full max-w-md bg-surface/95 p-6 shadow-xl backdrop-blur sm:p-8">
        <div className="mb-4 flex flex-col items-center gap-2 text-center">
          <ShieldAlert size={28} className="text-warning-text" />
          <h1 className="text-lg font-bold">{t('changePassword.heading')}</h1>
          <p className="text-xs text-subtle">{t('changePassword.hint')}</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label={t('changePassword.old')}>
            <Input type="password" value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} autoComplete="current-password" />
          </Field>
          <Field label={t('changePassword.new')}>
            <Input type="password" value={newPwd} onChange={(e) => setNewPwd(e.target.value)} autoComplete="new-password" />
          </Field>
          <Field label={t('changePassword.confirm')}>
            <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" />
          </Field>
          <ErrorText>{localError || (change.isError ? (change.error as Error).message : null)}</ErrorText>
          <Button type="submit" className="w-full" disabled={change.isPending}>
            {change.isPending ? t('changePassword.submitting') : t('changePassword.submit')}
          </Button>
        </form>
      </Card>
    </div>
  )
}


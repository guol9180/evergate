import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useLogin } from '../api/hooks'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { Button, Card, ErrorText, Field, Input } from '../components/ui'
import { withBasePath } from '../lib/runtime'

export default function Login() {
  usePageTitle(t('page.login.title'))
  const login = useLogin()
  const navigate = useNavigate()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    login.mutate(
      { username, password },
      {
        onSuccess: (data) => navigate(data.must_change_password ? '/change-password' : '/', { replace: true }),
      },
    )
  }

  return (
    <div className="flex h-full items-center justify-center p-6">
      <Card className="w-full max-w-md bg-surface/95 p-6 shadow-xl backdrop-blur sm:p-8">
        <div className="mb-6 flex flex-col items-center gap-3">
          <span className="flex h-12 w-12 items-center justify-center rounded-xl border border-brand-border bg-brand-subtle">
            <img src={withBasePath('/logo.svg')} alt="" width={28} height={28} />
          </span>
          <h1 className="text-lg font-bold text-foreground">{t('login.heading')}</h1>
          <p className="text-xs text-subtle">One gate, every model, zero interruption</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label={t('login.username')}>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="admin" autoComplete="username" />
          </Field>
          <Field label={t('login.password')}>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" autoComplete="current-password" />
          </Field>
          <ErrorText>{login.isError ? (login.error as Error).message : null}</ErrorText>
          <Button type="submit" className="w-full" disabled={login.isPending}>
            {login.isPending ? t('login.submitting') : t('login.submit')}
          </Button>
        </form>
      </Card>
    </div>
  )
}

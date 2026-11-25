import { type FormEvent, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { ShieldCheck } from 'lucide-react'
import { useSetupAdmin, useSetupStatus } from '../api/hooks'
import { Button, Card, ErrorText, Field, Input, Spinner } from '../components/ui'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { withBasePath } from '../lib/runtime'

const usernamePattern = /^[A-Za-z0-9_.-]{3,32}$/

/** 与后端 store.ValidateAdminPassword 对齐：大写 / 小写 / 数字 / 符号至少三类。 */
function passwordClasses(value: string): number {
  return [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((re) => re.test(value)).length
}

export default function Setup() {
  usePageTitle(t('page.setup.title'))
  const status = useSetupStatus()
  const setup = useSetupAdmin()
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [localError, setLocalError] = useState('')

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

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    const name = username.trim()
    if (!usernamePattern.test(name)) {
      setLocalError(t('setup.usernameInvalid'))
      return
    }
    if (password.length < 8) {
      setLocalError(t('setup.tooShort'))
      return
    }
    if (passwordClasses(password) < 3) {
      setLocalError(t('setup.weak'))
      return
    }
    if (password !== confirm) {
      setLocalError(t('setup.mismatch'))
      return
    }
    setLocalError('')
    setup.mutate({ username: name, password })
  }

  return (
    <div className="flex h-full items-center justify-center p-6">
      <Card className="w-full max-w-md bg-surface/95 p-6 shadow-xl backdrop-blur sm:p-8">
        <div className="mb-6 flex flex-col items-center gap-3">
          <span className="flex h-12 w-12 items-center justify-center rounded-xl border border-brand-border bg-brand-subtle">
            <img src={withBasePath('/logo.svg')} alt="" width={28} height={28} />
          </span>
          <h1 className="text-lg font-bold text-foreground">{t('setup.heading')}</h1>
          <p className="text-center text-xs text-subtle">{t('setup.hint')}</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label={t('setup.username')} hint={t('setup.usernameHint')}>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus />
          </Field>
          <Field label={t('setup.password')} hint={t('setup.passwordHint')}>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
          </Field>
          <Field label={t('setup.confirm')}>
            <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} autoComplete="new-password" />
          </Field>
          <ErrorText>{localError || (setup.isError ? (setup.error as Error).message : status.isError ? (status.error as Error).message : null)}</ErrorText>
          <Button type="submit" className="w-full" disabled={setup.isPending}>
            {setup.isPending ? t('setup.submitting') : t('setup.submit')}
          </Button>
        </form>
        <div className="mt-4 flex items-center justify-center gap-1.5 text-[11px] text-subtle">
          <ShieldCheck size={13} className="shrink-0" aria-hidden="true" />
          {t('setup.loopbackNote')}
        </div>
      </Card>
    </div>
  )
}

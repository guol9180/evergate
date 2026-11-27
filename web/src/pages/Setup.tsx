import { type FormEvent, useRef, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { ShieldCheck } from 'lucide-react'
import { useSetupAdmin, useSetupStatus } from '../api/hooks'
import { Alert, Button, Field, Input, Spinner, focusFirstInvalid } from '../components/ui'
import AuthCard from '../components/AuthCard'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'
import { errorMessage } from '../lib/errors'

const usernamePattern = /^[A-Za-z0-9_.-]{3,32}$/

type FieldErrors = { username?: string; password?: string; confirm?: string }

/** 与后端 store.ValidateAdminPassword 对齐：大写 / 小写 / 数字 / 符号至少三类。 */
function passwordClasses(value: string): number {
  return [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((re) => re.test(value)).length
}

export default function Setup() {
  usePageTitle(t('page.setup.title'))
  const status = useSetupStatus()
  const setup = useSetupAdmin()
  const formRef = useRef<HTMLFormElement>(null)
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [errors, setErrors] = useState<FieldErrors>({})

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
    // 一次性收集全部字段错误：用户不必提交 N 次才看全
    const next: FieldErrors = {}
    if (!usernamePattern.test(name)) next.username = t('setup.usernameInvalid')
    if (password.length < 8) next.password = t('setup.tooShort')
    else if (passwordClasses(password) < 3) next.password = t('setup.weak')
    if (password !== confirm) next.confirm = t('setup.mismatch')
    setErrors(next)
    if (Object.keys(next).length > 0) {
      focusFirstInvalid(formRef.current)
      return
    }
    setup.mutate({ username: name, password })
  }

  const serverError = setup.isError ? errorMessage(setup.error) : status.isError ? errorMessage(status.error) : ''

  return (
    <AuthCard
      title={t('setup.heading')}
      subtitle={t('setup.hint')}
      footer={<><ShieldCheck size={13} className="shrink-0" aria-hidden="true" /> {t('setup.loopbackNote')}</>}
    >
      <form ref={formRef} onSubmit={onSubmit} className="space-y-4" noValidate>
        {serverError && <Alert tone="danger">{serverError}</Alert>}
        <Field label={t('setup.username')} hint={t('setup.usernameHint')} error={errors.username}>
          <Input value={username} onChange={(e) => { setUsername(e.target.value); setErrors((current) => ({ ...current, username: undefined })) }} autoComplete="username" autoFocus />
        </Field>
        <Field label={t('setup.password')} hint={t('setup.passwordHint')} error={errors.password}>
          <Input type="password" value={password} onChange={(e) => { setPassword(e.target.value); setErrors((current) => ({ ...current, password: undefined })) }} autoComplete="new-password" />
        </Field>
        <Field label={t('setup.confirm')} error={errors.confirm}>
          <Input type="password" value={confirm} onChange={(e) => { setConfirm(e.target.value); setErrors((current) => ({ ...current, confirm: undefined })) }} autoComplete="new-password" />
        </Field>
        <Button type="submit" className="w-full" loading={setup.isPending}>
          {t('setup.submit')}
        </Button>
      </form>
    </AuthCard>
  )
}

import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useLogin } from '../api/hooks'
import { Button, Card, ErrorText, Field, Input } from '../components/ui'

export default function Login() {
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
      <Card className="w-full max-w-md bg-white/95 p-8 shadow-xl backdrop-blur">
        <div className="mb-6 flex flex-col items-center gap-3">
          <span className="flex h-12 w-12 items-center justify-center rounded-xl border border-indigo-100 bg-indigo-50">
            <img src="/logo.svg" alt="" width={28} height={28} />
          </span>
          <h1 className="text-lg font-bold text-slate-900">EverGate 管理台</h1>
          <p className="text-xs text-slate-400">One gate, every model, zero interruption</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label="用户名">
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="admin" autoComplete="username" />
          </Field>
          <Field label="密码">
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" autoComplete="current-password" />
          </Field>
          <ErrorText>{login.isError ? (login.error as Error).message : null}</ErrorText>
          <Button type="submit" className="w-full" disabled={login.isPending}>
            {login.isPending ? '登录中…' : '登录'}
          </Button>
        </form>
      </Card>
    </div>
  )
}

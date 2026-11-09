import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ShieldAlert } from 'lucide-react'
import { useChangePassword } from '../api/hooks'
import { Button, Card, ErrorText, Field, Input } from '../components/ui'

export default function ChangePassword() {
  const change = useChangePassword()
  const navigate = useNavigate()
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [confirm, setConfirm] = useState('')
  const [localError, setLocalError] = useState('')

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (newPwd.length < 8) {
      setLocalError('新密码至少 8 位')
      return
    }
    if (newPwd !== confirm) {
      setLocalError('两次输入的新密码不一致')
      return
    }
    setLocalError('')
    change.mutate(
      { old_password: oldPwd, new_password: newPwd },
      { onSuccess: () => navigate('/', { replace: true }) },
    )
  }

  return (
    <div className="flex h-full items-center justify-center bg-slate-50">
      <Card className="w-96 p-8">
        <div className="mb-4 flex flex-col items-center gap-2 text-center">
          <ShieldAlert size={28} className="text-amber-500" />
          <h1 className="text-lg font-bold">首次登录须修改密码</h1>
          <p className="text-xs text-slate-400">默认密码 admin/admin123 已不安全，请立即更换</p>
        </div>
        <form onSubmit={onSubmit} className="space-y-4">
          <Field label="旧密码">
            <Input type="password" value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} />
          </Field>
          <Field label="新密码（≥8 位）">
            <Input type="password" value={newPwd} onChange={(e) => setNewPwd(e.target.value)} />
          </Field>
          <Field label="确认新密码">
            <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </Field>
          <ErrorText>{localError || (change.isError ? (change.error as Error).message : null)}</ErrorText>
          <Button type="submit" className="w-full" disabled={change.isPending}>
            {change.isPending ? '提交中…' : '修改密码'}
          </Button>
        </form>
      </Card>
    </div>
  )
}


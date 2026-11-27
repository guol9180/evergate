import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Login from '../../src/pages/Login'

const loginMutation = vi.hoisted(() => ({
  mustChange: true,
  mutate: vi.fn((_input: unknown, options?: { onSuccess?: (data: { username: string; must_change_password: boolean }) => void }) => {
    options?.onSuccess?.({ username: 'admin', must_change_password: loginMutation.mustChange })
  }),
  isError: false,
  error: null,
  isPending: false,
}))

const setupStatus = vi.hoisted(() => ({
  data: { setup_required: false },
  isPending: false,
  isError: false,
  error: null,
}))

vi.mock('../../src/api/hooks', () => ({
  useLogin: () => loginMutation,
  useSetupStatus: () => setupStatus,
}))

function Landing() {
  const location = useLocation()
  return <div>landing {location.pathname}{location.search}</div>
}

describe('Login', () => {
  it('validates empty credentials inline without calling the API', () => {
    loginMutation.mutate.mockClear()
    render(
      <MemoryRouter initialEntries={['/login']}>
        <Login />
      </MemoryRouter>,
    )
    fireEvent.click(screen.getByRole('button', { name: '登录' }))
    expect(screen.getByText('请输入用户名')).toBeTruthy()
    expect(screen.getByText('请输入密码')).toBeTruthy()
    expect(screen.getByLabelText('用户名').getAttribute('aria-invalid')).toBe('true')
    expect(screen.getByLabelText('密码').getAttribute('aria-invalid')).toBe('true')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(loginMutation.mutate).not.toHaveBeenCalled()
  })

  it('submits credentials and routes to forced password change', async () => {
    render(
      <MemoryRouter initialEntries={['/login']}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/change-password" element={<div>change password</div>} />
        </Routes>
      </MemoryRouter>,
    )

    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'admin123' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))

    expect(loginMutation.mutate).toHaveBeenCalledWith(
      { username: 'admin', password: 'admin123' },
      expect.any(Object),
    )
    await waitFor(() => expect(screen.getByText('change password')).toBeTruthy())
  })

  it('returns to the originating page after a session-expired login', async () => {
    loginMutation.mustChange = false
    render(
      <MemoryRouter initialEntries={['/login?next=%2Flogs%3Fpage%3D2']}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/logs" element={<Landing />} />
        </Routes>
      </MemoryRouter>,
    )
    // 带回跳地址进入登录页时提示"登录已过期"，让用户知道为什么被送回来
    expect(screen.getByRole('status').textContent).toContain('登录已过期')
    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'admin123' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))
    await waitFor(() => expect(screen.getByText('landing /logs?page=2')).toBeTruthy())
  })

  it('ignores off-site or auth-page return addresses', async () => {
    loginMutation.mustChange = false
    render(
      <MemoryRouter initialEntries={['/login?next=https%3A%2F%2Fevil.example%2F']}>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/" element={<Landing />} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.queryByRole('status')).toBeNull()
    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'admin123' } })
    fireEvent.click(screen.getByRole('button', { name: '登录' }))
    await waitFor(() => expect(screen.getByText('landing /')).toBeTruthy())
  })
})

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Login from '../../src/pages/Login'

const loginMutation = vi.hoisted(() => ({
  mutate: vi.fn((_input: unknown, options?: { onSuccess?: (data: { username: string; must_change_password: boolean }) => void }) => {
    options?.onSuccess?.({ username: 'admin', must_change_password: true })
  }),
  isError: false,
  error: null,
  isPending: false,
}))

vi.mock('../../src/api/hooks', () => ({
  useLogin: () => loginMutation,
}))

describe('Login', () => {
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
})

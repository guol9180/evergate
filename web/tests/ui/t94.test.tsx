import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Setup from '../../src/pages/Setup'

const state = vi.hoisted(() => ({
  required: true,
  mutate: vi.fn((input: { username: string; password: string }) => input),
}))

vi.mock('../../src/api/hooks', async () => {
  const React = await import('react')
  return {
    useSetupStatus: () => ({ data: { setup_required: state.required }, isPending: false, isError: false, error: null }),
    useSetupAdmin: () => {
      const [data, setData] = React.useState<{ username: string } | null>(null)
      const mutate = (input: { username: string; password: string }) => {
        state.mutate(input)
        setData({ username: input.username })
      }
      return { data, mutate, isPending: false, isError: false, error: null }
    },
  }
})

function renderSetup() {
  return render(
    <MemoryRouter initialEntries={['/setup']}>
      <Routes>
        <Route path="/setup" element={<Setup />} />
        <Route path="/login" element={<div>login page</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Setup', () => {
  it('rejects weak passwords before calling the API', () => {
    state.required = true
    state.mutate.mockClear()
    renderSetup()

    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'short' } })
    fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'short' } })
    fireEvent.click(screen.getByRole('button', { name: '创建并前往登录' }))

    // 字段级错误：落在密码框下方，并标记 aria-invalid；不再是按钮上方的大色块
    expect(screen.getByText('密码至少 8 位')).toBeTruthy()
    expect(screen.getByLabelText('密码').getAttribute('aria-invalid')).toBe('true')
    expect(screen.queryByRole('alert')).toBeNull()
    expect(state.mutate).not.toHaveBeenCalled()
  })

  it('rejects mismatched confirmation and invalid usernames', () => {
    state.required = true
    state.mutate.mockClear()
    renderSetup()

    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'a!' } })
    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'Admin1234!' } })
    fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'Admin1234!' } })
    fireEvent.click(screen.getByRole('button', { name: '创建并前往登录' }))
    expect(screen.getByText(/用户名需为/)).toBeTruthy()

    fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'Admin1234?' } })
    fireEvent.click(screen.getByRole('button', { name: '创建并前往登录' }))
    expect(screen.getByText('两次输入的密码不一致')).toBeTruthy()
    expect(screen.queryByText(/用户名需为/)).toBeNull()
    expect(state.mutate).not.toHaveBeenCalled()
  })

  it('creates the admin then lands on the login page', async () => {
    state.required = true
    state.mutate.mockClear()
    renderSetup()

    fireEvent.change(screen.getByLabelText('密码'), { target: { value: 'Admin1234!' } })
    fireEvent.change(screen.getByLabelText('确认密码'), { target: { value: 'Admin1234!' } })
    fireEvent.click(screen.getByRole('button', { name: '创建并前往登录' }))

    expect(state.mutate).toHaveBeenCalledWith({ username: 'admin', password: 'Admin1234!' })
    await waitFor(() => expect(screen.getByText('login page')).toBeTruthy())
  })

  it('leaves for the login page when setup is already done', () => {
    state.required = false
    renderSetup()
    expect(screen.getByText('login page')).toBeTruthy()
  })
})

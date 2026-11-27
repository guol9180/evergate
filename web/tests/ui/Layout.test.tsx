import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Layout from '../../src/components/Layout'
import { ApiError } from '../../src/api/client'

const state = vi.hoisted(() => ({ error: null as unknown, refetch: vi.fn() }))

vi.mock('../../src/api/hooks', () => ({
  useMe: () => ({ isPending: false, isError: !!state.error, error: state.error, data: undefined, refetch: state.refetch }),
  useLogout: () => ({ mutate: vi.fn() }),
  useUpdateStatus: () => ({ isPending: false, isError: true, data: undefined }),
}))

function LoginProbe() {
  const location = useLocation()
  return <div>login page {location.search}</div>
}

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<div>dashboard</div>} />
          <Route path="/logs" element={<div>logs</div>} />
        </Route>
        <Route path="/login" element={<LoginProbe />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('Layout route guard', () => {
  it('navigates to login with the original route only on 401', async () => {
    state.error = new ApiError('未登录', 401)
    renderAt('/logs?page=2')

    await waitFor(() => expect(screen.getByText('login page ?next=%2Flogs%3Fpage%3D2')).toBeTruthy())
  })

  it('stays on the page with a retry banner when the console cannot be reached', async () => {
    state.error = new TypeError('Failed to fetch')
    state.refetch.mockClear()
    renderAt('/logs')

    expect(screen.getByText('logs')).toBeTruthy()
    expect(screen.getByText(/无法连接到管理台/)).toBeTruthy()
    expect(screen.queryByText(/login page/)).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(state.refetch).toHaveBeenCalledTimes(1)
  })
})

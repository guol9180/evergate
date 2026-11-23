import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Layout from '../../src/components/Layout'

vi.mock('../../src/api/hooks', () => ({
  useMe: () => ({ isPending: false, isError: true, data: undefined }),
  useLogout: () => ({ mutate: vi.fn() }),
  useUpdateStatus: () => ({ isPending: false, isError: true, data: undefined }),
}))

describe('Layout route guard', () => {
  it('navigates unauthenticated visitors to login', async () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<div>dashboard</div>} />
          </Route>
          <Route path="/login" element={<div>login page</div>} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => expect(screen.getByText('login page')).toBeTruthy())
  })
})

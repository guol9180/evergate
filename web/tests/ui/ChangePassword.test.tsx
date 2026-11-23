import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import ChangePassword from '../../src/pages/ChangePassword'

const changeMutation = vi.hoisted(() => ({
  mutate: vi.fn(),
  isError: false,
  error: null,
  isPending: false,
}))

vi.mock('../../src/api/hooks', () => ({
  useChangePassword: () => changeMutation,
}))

describe('ChangePassword', () => {
  it('rejects mismatched confirmation locally', () => {
    render(
      <MemoryRouter>
        <ChangePassword />
      </MemoryRouter>,
    )

    fireEvent.change(screen.getByLabelText('旧密码'), { target: { value: 'admin123' } })
    fireEvent.change(screen.getByLabelText('新密码（≥8 位）'), { target: { value: 'new-password' } })
    fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'different' } })
    fireEvent.click(screen.getByRole('button', { name: '修改密码' }))

    expect(screen.getByText('两次输入的新密码不一致')).toBeTruthy()
    expect(changeMutation.mutate).not.toHaveBeenCalled()
  })
})

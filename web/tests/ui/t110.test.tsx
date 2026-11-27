import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useUnsavedGuard } from '../../src/lib/useUnsavedGuard'

function Harness({ dirty }: { dirty: boolean }) {
  const guard = useUnsavedGuard(dirty)
  return (
    <>
      <button type="button" onClick={guard.guard(() => screen.getByTestId('closed').setAttribute('data-closed', 'yes'))}>关闭</button>
      <div data-testid="closed" />
      {guard.dialog}
    </>
  )
}

describe('useUnsavedGuard (T110 ②)', () => {
  it('closes immediately when the form is clean', () => {
    render(<Harness dirty={false} />)
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.getByTestId('closed').getAttribute('data-closed')).toBe('yes')
    expect(screen.queryByText('放弃未保存的更改？')).toBeNull()
  })

  it('asks before discarding and only closes after confirmation', () => {
    const onConfirmSpy = vi.fn()
    render(<Harness dirty />)
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    expect(screen.getByText('放弃未保存的更改？')).toBeTruthy()
    expect(screen.getByTestId('closed').getAttribute('data-closed')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(screen.getByTestId('closed').getAttribute('data-closed')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    fireEvent.click(screen.getByRole('button', { name: '放弃更改' }))
    expect(screen.getByTestId('closed').getAttribute('data-closed')).toBe('yes')
    expect(onConfirmSpy).not.toHaveBeenCalled()
  })
})
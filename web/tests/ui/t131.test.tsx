import { createRef } from 'react'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Badge, ConfirmDialog, Disclosure, StatusBadge, Tooltip, TooltipProvider } from '../../src/components/ui'

describe('T131 基础原语迁移', () => {
  it('Badge / StatusBadge 透传 ref 与 rest props，可直接当触发器用', () => {
    const ref = createRef<HTMLSpanElement>()
    const onClick = vi.fn()
    render(
      <>
        <Badge ref={ref} tone="brand" data-testid="badge" title="差异明细" role="button" tabIndex={0} onClick={onClick}>+2 / −1</Badge>
        <StatusBadge tone="success" data-testid="status" aria-label="状态：正常">正常</StatusBadge>
      </>,
    )
    expect(ref.current).toBe(screen.getByTestId('badge'))
    fireEvent.click(screen.getByTestId('badge'))
    expect(onClick).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId('badge').getAttribute('title')).toBe('差异明细')
    expect(screen.getByRole('button', { name: '+2 / −1' }).getAttribute('tabindex')).toBe('0')
    expect(screen.getByTestId('status').getAttribute('aria-label')).toBe('状态：正常')
  })

  it('ConfirmDialog 基于 AlertDialog：alertdialog 语义，确认 / 取消各只回调一次，Esc 走取消', async () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<ConfirmDialog open title="删除渠道？" message="删除后不可恢复" onConfirm={onConfirm} onCancel={onCancel} danger />)
    const dialog = await screen.findByRole('alertdialog')
    expect(dialog.textContent).toContain('删除后不可恢复')
    fireEvent.click(screen.getByRole('button', { name: '确认' }))
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)
    expect(onCancel).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(dialog, { key: 'Escape' })
    await waitFor(() => expect(onCancel).toHaveBeenCalledTimes(2))
  })

  it('Disclosure 基于 Collapsible：触发器是带 aria-expanded 的按钮', async () => {
    render(<Disclosure title="维护任务" defaultOpen={false}><div>维护内容</div></Disclosure>)
    const trigger = screen.getByRole('button', { name: /维护任务/ })
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    fireEvent.click(trigger)
    await waitFor(() => expect(trigger.getAttribute('aria-expanded')).toBe('true'))
  })

  it('Tooltip：没有根部 Provider 时就地补一个，有 Provider 时共用', async () => {
    render(<Tooltip content="提示 A"><button type="button">a</button></Tooltip>)
    fireEvent.focus(screen.getByRole('button', { name: 'a' }))
    expect(await screen.findByText('提示 A')).toBeTruthy()
    cleanup()
    render(<TooltipProvider><Tooltip content="提示 B"><button type="button">b</button></Tooltip></TooltipProvider>)
    fireEvent.focus(screen.getByRole('button', { name: 'b' }))
    expect(await screen.findByText('提示 B')).toBeTruthy()
  })
})

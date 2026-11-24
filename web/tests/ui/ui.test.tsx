import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Button, ConfirmDialog, Dialog, Drawer, Field, Input, Pagination, StatusBadge } from '../../src/components/ui'

function Harness({ open, onClose }: { open: boolean; onClose: () => void }) {
  return (
    <Dialog open={open} onClose={onClose} title="测试对话框" footer={<Button onClick={onClose}>取消</Button>}>
      <Field label="名称"><Input placeholder="请输入" /></Field>
    </Dialog>
  )
}

describe('Dialog accessibility', () => {
  it('exposes dialog semantics and a labelled close button', () => {
    render(<Harness open onClose={() => {}} />)
    const dialog = screen.getByRole('dialog')
    expect(dialog.getAttribute('aria-modal')).toBe('true')
    expect(screen.getByText('测试对话框').id).toBe(dialog.getAttribute('aria-labelledby'))
    expect(screen.getByRole('button', { name: '关闭' })).toBeTruthy()
  })

  it('closes on Escape and returns focus to the trigger', async () => {
    const onClose = vi.fn()
    const { rerender } = render(
      <>
        <button>打开</button>
        <Harness open={false} onClose={onClose} />
      </>,
    )
    const trigger = screen.getByText('打开')
    trigger.focus()
    rerender(
      <>
        <button>打开</button>
        <Harness open onClose={onClose} />
      </>,
    )
    await waitFor(() => expect(document.activeElement).not.toBe(trigger))
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
    rerender(
      <>
        <button>打开</button>
        <Harness open={false} onClose={onClose} />
      </>,
    )
    await waitFor(() => expect(document.activeElement).toBe(trigger))
  })

  it('keeps Tab focus inside the dialog', async () => {
    render(<Harness open onClose={() => {}} />)
    const dialog = screen.getByRole('dialog')
    const focusables = dialog.querySelectorAll<HTMLElement>('button, input')
    const last = focusables[focusables.length - 1]
    last.focus()
    fireEvent.keyDown(window, { key: 'Tab' })
    expect(document.activeElement).toBe(focusables[0])
  })

  it('Drawer also closes on Escape', () => {
    const onClose = vi.fn()
    render(<Drawer open onClose={onClose} title="抽屉"><div>内容</div></Drawer>)
    expect(screen.getByRole('dialog').getAttribute('aria-modal')).toBe('true')
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

describe('ConfirmDialog', () => {
  it('invokes confirm and cancel callbacks and renders as alertdialog', () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(
      <ConfirmDialog open title="删除渠道" message="此操作不可撤销" confirmText="删除" danger onConfirm={onConfirm} onCancel={onCancel} />,
    )
    expect(screen.getByRole('alertdialog')).toBeTruthy()
    expect(screen.getByText('此操作不可撤销')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '删除' }))
    expect(onConfirm).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('disables buttons while loading', () => {
    render(
      <ConfirmDialog open title="删除" message="x" confirmText="删除" loading onConfirm={() => {}} onCancel={() => {}} />,
    )
    expect((screen.getByRole('button', { name: '处理中…' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: '取消' }) as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('StatusBadge', () => {
  it('always renders the status text so color is never the only cue', () => {
    render(<StatusBadge tone="danger">不可用</StatusBadge>)
    expect(screen.getByText('不可用')).toBeTruthy()
  })
})

describe('Field', () => {
  it('associates the label with the injected control id and hint', () => {
    render(
      <>
        <Field label="名称" hint="必填"><Input /></Field>
      </>,
    )
    const label = screen.getByText('名称') as HTMLLabelElement
    const input = screen.getByLabelText('名称') as HTMLInputElement
    expect(label.getAttribute('for')).toBe(input.id)
    expect(input.getAttribute('aria-describedby')).toBe(screen.getByText('必填').id)
  })
})

describe('Pagination', () => {
  it('renders first/last pages, ellipses and the current page', () => {
    const onPageChange = vi.fn()
    render(<Pagination page={5} totalPages={20} total={391} pageSize={20} onPageChange={onPageChange} />)
    expect(screen.getByText('共 391 条 · 第 5/20 页')).toBeTruthy()
    expect(screen.getByRole('button', { name: '第 5 页' }).getAttribute('aria-current')).toBe('page')
    expect(screen.getByRole('button', { name: '第 1 页' })).toBeTruthy()
    expect(screen.getByRole('button', { name: '第 20 页' })).toBeTruthy()
    expect(screen.getAllByText('…').length).toBe(2)
    fireEvent.click(screen.getByRole('button', { name: '下一页' }))
    expect(onPageChange).toHaveBeenCalledWith(6)
  })

  it('lets the user change page size and disables navigation at bounds', () => {
    const onPageSizeChange = vi.fn()
    render(<Pagination page={1} totalPages={1} total={7} pageSize={20} onPageSizeChange={onPageSizeChange} onPageChange={() => {}} />)
    expect((screen.getByRole('button', { name: '上一页' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: '下一页' }) as HTMLButtonElement).disabled).toBe(true)
    fireEvent.change(screen.getByLabelText('每页条数'), { target: { value: '50' } })
    expect(onPageSizeChange).toHaveBeenCalledWith(50)
  })
})

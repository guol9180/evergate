import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Button, ConfirmDialog, ContentSwap, Dialog, Drawer, Empty, Field, Input, Pagination, StatusBadge } from '../../src/components/ui'

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
    // Radix 在 document 上监听 Escape；真实按键从焦点元素冒泡到 document
    fireEvent.keyDown(document.activeElement ?? document, { key: 'Escape', bubbles: true })
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
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true))
    const focusables = dialog.querySelectorAll<HTMLElement>('button, input')
    const last = focusables[focusables.length - 1]
    last.focus()
    fireEvent.keyDown(last, { key: 'Tab', bubbles: true })
    expect(document.activeElement).toBe(focusables[0])
  })

  it('Drawer also closes on Escape', async () => {
    const onClose = vi.fn()
    render(<Drawer open onClose={onClose} title="抽屉"><div>内容</div></Drawer>)
    const drawer = screen.getByRole('dialog')
    expect(drawer.getAttribute('aria-modal')).toBe('true')
    await waitFor(() => expect(drawer.contains(document.activeElement)).toBe(true))
    fireEvent.keyDown(document.activeElement ?? document, { key: 'Escape', bubbles: true })
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

  it('marks the confirm button busy and disables both actions while loading', () => {
    render(
      <ConfirmDialog open title="删除" message="x" confirmText="删除" loading onConfirm={() => {}} onCancel={() => {}} />,
    )
    const confirm = screen.getByRole('button', { name: '删除' }) as HTMLButtonElement
    expect(confirm.disabled).toBe(true)
    expect(confirm.getAttribute('aria-busy')).toBe('true')
    expect(confirm.querySelector('.animate-spin')).toBeTruthy()
    expect((screen.getByRole('button', { name: '取消' }) as HTMLButtonElement).disabled).toBe(true)
  })
})

describe('Button loading', () => {
  it('keeps the label in layout, blocks repeat clicks and shows an aria-busy spinner', () => {
    const onClick = vi.fn()
    render(<Button loading onClick={onClick}>保存</Button>)
    const button = screen.getByRole('button', { name: '保存' }) as HTMLButtonElement
    expect(button.getAttribute('aria-busy')).toBe('true')
    expect(button.disabled).toBe(true)
    expect(button.querySelector('.opacity-0')).toBeTruthy()
    expect(button.querySelector('.animate-spin')).toBeTruthy()
    fireEvent.click(button)
    expect(onClick).not.toHaveBeenCalled()
  })

  it('leaves the button interactive when not loading', () => {
    const onClick = vi.fn()
    render(<Button onClick={onClick}>保存</Button>)
    const button = screen.getByRole('button', { name: '保存' }) as HTMLButtonElement
    expect(button.getAttribute('aria-busy')).toBeNull()
    expect(button.disabled).toBe(false)
    fireEvent.click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
  })
})

describe('Empty', () => {
  it('renders an icon container, title, hint and primary action', () => {
    render(<Empty title="还没有渠道" hint="新建一个渠道" action={<button type="button">新建渠道</button>} />)
    expect(screen.getByText('还没有渠道')).toBeTruthy()
    expect(screen.getByText('新建一个渠道')).toBeTruthy()
    expect(screen.getByRole('button', { name: '新建渠道' })).toBeTruthy()
    expect(document.querySelector('.rounded-full.bg-brand-subtle')).toBeTruthy()
  })

  it('uses the neutral container and clear action for filtered results', () => {
    render(<Empty filtered title="没有匹配的日志" action={<button type="button">清除筛选</button>} />)
    expect(document.querySelector('.rounded-full.bg-surface-strong')).toBeTruthy()
    expect(screen.getByRole('button', { name: '清除筛选' })).toBeTruthy()
  })
})

describe('ContentSwap', () => {
  it('keeps the skeleton while loading and fades it over the loaded content', async () => {
    const { rerender } = render(<ContentSwap loading skeleton={<div>骨架</div>}><div>内容</div></ContentSwap>)
    expect(screen.getByText('骨架')).toBeTruthy()
    expect(screen.queryByText('内容')).toBeNull()
    rerender(<ContentSwap loading={false} skeleton={<div>骨架</div>}><div>内容</div></ContentSwap>)
    await waitFor(() => expect(screen.getByText('内容')).toBeTruthy())
    expect(screen.getByText('骨架').parentElement?.className).toContain('eg-skeleton-overlay')
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

  it('lets the user change page size and disables navigation at bounds', async () => {
    const onPageSizeChange = vi.fn()
    render(<Pagination page={1} totalPages={1} total={7} pageSize={20} onPageSizeChange={onPageSizeChange} onPageChange={() => {}} />)
    expect((screen.getByRole('button', { name: '上一页' }) as HTMLButtonElement).disabled).toBe(true)
    expect((screen.getByRole('button', { name: '下一页' }) as HTMLButtonElement).disabled).toBe(true)
    // Radix Select：Enter 打开，焦点落在当前项，方向键移动，Enter 选中
    const trigger = screen.getByRole('combobox', { name: '每页条数' })
    fireEvent.keyDown(trigger, { key: 'Enter' })
    const option = await screen.findByRole('option', { name: /50/ })
    fireEvent.keyDown(option, { key: 'Enter' })
    expect(onPageSizeChange).toHaveBeenCalledWith(50)
  })
})

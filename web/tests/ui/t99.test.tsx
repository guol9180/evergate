import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Alert, ErrorText, Field, Input, Progress, SegmentedControl, Select, Tooltip } from '../../src/components/ui'
import { notify, Toaster } from '../../src/components/Toaster'

describe('Alert', () => {
  it('maps tones to live-region roles', () => {
    const { rerender } = render(<Alert tone="danger">boom</Alert>)
    expect(screen.getByRole('alert').textContent).toContain('boom')
    rerender(<Alert tone="warning">careful</Alert>)
    expect(screen.getByRole('alert').textContent).toContain('careful')
    rerender(<Alert tone="success">done</Alert>)
    expect(screen.getByRole('status').textContent).toContain('done')
    rerender(<Alert tone="info">note</Alert>)
    expect(screen.getByRole('status').textContent).toContain('note')
    rerender(<Alert tone="neutral">context</Alert>)
    expect(screen.getByRole('status').textContent).toContain('context')
  })

  it('renders title and action slots', () => {
    render(<Alert tone="warning" title="Heads up" action={<button type="button">Fix</button>}>Detail</Alert>)
    expect(screen.getByText('Heads up')).toBeTruthy()
    expect(screen.getByText('Detail')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Fix' })).toBeTruthy()
  })

  it('keeps ErrorText on the shared component', () => {
    const { rerender } = render(<ErrorText>failed</ErrorText>)
    expect(screen.getByRole('alert').textContent).toContain('failed')
    rerender(<ErrorText />)
    expect(screen.queryByRole('alert')).toBeNull()
  })
})

describe('Field error', () => {
  it('marks the control invalid and describes it with the inline error instead of the hint', () => {
    render(<Field label="密码" hint="至少 8 位" error="两次输入不一致"><Input /></Field>)
    const input = screen.getByLabelText('密码')
    expect(input.getAttribute('aria-invalid')).toBe('true')
    expect(input.getAttribute('aria-describedby')).toBe(screen.getByText('两次输入不一致').closest('[data-field-error]')?.id)
    expect(screen.queryByText('至少 8 位')).toBeNull()
    // 字段错误不是 live region：由 aria-describedby 在聚焦时播报，避免整段表单重复播报
    expect(screen.queryByRole('alert')).toBeNull()
  })
})

describe('SegmentedControl', () => {
  it('exposes radiogroup semantics and moves with arrow keys', async () => {
    const onChange = vi.fn()
    render(
      <SegmentedControl
        label="协议"
        value="chat"
        onChange={onChange}
        options={[{ value: 'chat', label: 'Chat' }, { value: 'gemini', label: 'Gemini' }]}
      />,
    )
    const chat = screen.getByRole('radio', { name: 'Chat' })
    const gemini = screen.getByRole('radio', { name: 'Gemini' })
    expect(chat.getAttribute('aria-checked')).toBe('true')
    expect(gemini.getAttribute('tabindex')).toBe('-1')
    // roving focus：方向键作用于当前聚焦的分段
    act(() => chat.focus())
    fireEvent.keyDown(chat, { key: 'ArrowRight' })
    // Radix 在下一拍移动焦点，选中随焦点变化
    await waitFor(() => expect(onChange).toHaveBeenCalledWith('gemini'))
    await waitFor(() => expect(document.activeElement).toBe(gemini))
  })
})

describe('Progress', () => {
  it('exposes progressbar semantics and clamps the value', () => {
    render(<Progress value={140} />)
    expect(screen.getByRole('progressbar').getAttribute('aria-valuenow')).toBe('100')
  })
})

describe('Select (Radix)', () => {
  it('shows the selected label, opens with the keyboard and selects an option', async () => {
    const onValueChange = vi.fn()
    render(
      <Select
        aria-label="渠道"
        value=""
        onValueChange={onValueChange}
        options={[{ value: '', label: '全部' }, { value: 'a', label: '渠道 A' }]}
      />,
    )
    const trigger = screen.getByRole('combobox', { name: '渠道' })
    expect(trigger.textContent).toContain('全部')
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    const option = await screen.findByRole('option', { name: '渠道 A' })
    fireEvent.keyDown(option, { key: 'Enter' })
    expect(onValueChange).toHaveBeenCalledWith('a')
  })
})

describe('Tooltip (Radix)', () => {
  it('shows the control label on pointer hover', async () => {
    render(
      <Tooltip content="删除"><button type="button">x</button></Tooltip>,
    )
    fireEvent.pointerMove(screen.getByText('x'), { pointerType: 'mouse' })
    expect(await screen.findByText('删除', {}, { timeout: 1500 })).toBeTruthy()
  })
})

describe('Toaster (sonner)', () => {
  it('renders transient feedback in the notification region', async () => {
    render(<Toaster />)
    act(() => { notify.success('已保存') })
    expect(await screen.findByText('已保存')).toBeTruthy()
    expect(screen.getByText('已保存').closest('ol')).toBeTruthy()
  })

  it('puts the error message in the description under a generic title', async () => {
    render(<Toaster />)
    act(() => { notify.error(new Error('上游返回 401')) })
    expect(await screen.findByText('操作失败')).toBeTruthy()
    expect(screen.getByText('上游返回 401')).toBeTruthy()
  })
})

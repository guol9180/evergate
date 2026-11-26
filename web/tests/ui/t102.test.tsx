import { useState } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SegmentedControl } from '../../src/components/ui'

const options = [
  { value: 'chat', label: 'Chat' },
  { value: 'responses', label: 'Responses' },
  { value: 'messages', label: 'Messages' },
  { value: 'gemini', label: 'Gemini' },
]

// 与页面一致受控：选中后 value 跟随变化。Radix 对「已选中项」的再次点击不会再触发 onValueChange
function ProtocolPicker({ disabled = false }: { disabled?: boolean }) {
  const [value, setValue] = useState('chat')
  return <SegmentedControl label="协议" value={value} onChange={setValue} options={options} disabled={disabled} />
}

const radio = (name: string) => screen.getByRole('radio', { name })
const checkedLabel = () => screen.getAllByRole('radio').find((item) => item.getAttribute('aria-checked') === 'true')?.textContent

describe('SegmentedControl on Radix RadioGroup (T102)', () => {
  it('moves selection with every arrow key because orientation is not forced', async () => {
    render(<ProtocolPicker />)
    fireEvent.keyDown(radio('Chat'), { key: 'ArrowDown' })
    fireEvent.keyUp(radio('Chat'), { key: 'ArrowDown' })
    // 选中在 keydown 当下生效，不依赖 keyup 时序；焦点由 Radix 在下一拍跟上
    expect(checkedLabel()).toBe('Responses')
    await waitFor(() => expect(document.activeElement).toBe(radio('Responses')))

    fireEvent.keyDown(radio('Responses'), { key: 'ArrowLeft' })
    expect(checkedLabel()).toBe('Chat')
    await waitFor(() => expect(document.activeElement).toBe(radio('Chat')))
  })

  it('makes the group the tab stop and forwards keyboard focus to the checked segment', async () => {
    render(<ProtocolPicker />)
    const group = screen.getByRole('radiogroup', { name: '协议' })
    // 分段注册完成后由组本身承接 Tab，各分段不单独成为 Tab 停靠点
    await waitFor(() => expect(group.getAttribute('tabindex')).toBe('0'))
    expect(screen.getAllByRole('radio').map((item) => item.getAttribute('tabindex'))).toEqual(['-1', '-1', '-1', '-1'])

    await act(async () => group.focus())
    expect(document.activeElement).toBe(radio('Chat'))
    expect(radio('Chat').getAttribute('tabindex')).toBe('0')
  })

  it('disables every segment together with the group', () => {
    render(<ProtocolPicker disabled />)
    for (const item of screen.getAllByRole('radio')) expect((item as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(radio('Gemini'))
    expect(checkedLabel()).toBe('Chat')
  })
})

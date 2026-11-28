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

// 与页面一致受控：选中后 value 跟随变化。已选中项的再次点击不会触发 onChange
function ProtocolPicker({ disabled = false }: { disabled?: boolean }) {
  const [value, setValue] = useState('chat')
  return <SegmentedControl label="协议" value={value} onChange={setValue} options={options} disabled={disabled} />
}

const radio = (name: string) => screen.getByRole('radio', { name })
const checkedLabel = () => screen.getAllByRole('radio').find((item) => item.getAttribute('aria-checked') === 'true')?.textContent

describe('SegmentedControl on shadcn ToggleGroup (T102 / T131)', () => {
  it('moves selection with every arrow key because orientation is not forced', async () => {
    render(<ProtocolPicker />)
    // roving focus：方向键作用于当前聚焦的分段；Radix 在下一拍移动焦点，选中随之变化
    act(() => radio('Chat').focus())
    fireEvent.keyDown(radio('Chat'), { key: 'ArrowDown' })
    fireEvent.keyUp(radio('Chat'), { key: 'ArrowDown' })
    await waitFor(() => expect(checkedLabel()).toBe('Responses'))
    await waitFor(() => expect(document.activeElement).toBe(radio('Responses')))

    fireEvent.keyDown(radio('Responses'), { key: 'ArrowLeft' })
    await waitFor(() => expect(checkedLabel()).toBe('Chat'))
    await waitFor(() => expect(document.activeElement).toBe(radio('Chat')))
  })

  it('keeps roving focus: the checked segment is the tab stop and focus entering the group lands on it', async () => {
    render(<ProtocolPicker />)
    const group = screen.getByRole('radiogroup', { name: '协议' })
    // Roving focus：组本身是 Tab 停靠点，各分段 tabindex=-1
    expect(group.getAttribute('tabindex')).toBe('0')
    expect(screen.getAllByRole('radio').map((item) => item.getAttribute('tabindex'))).toEqual(['-1', '-1', '-1', '-1'])

    // 焦点进入组时转到选中段，该段成为组内停靠点
    await act(async () => group.focus())
    expect(document.activeElement).toBe(radio('Chat'))
    await waitFor(() => expect(radio('Chat').getAttribute('tabindex')).toBe('0'))

    fireEvent.keyDown(radio('Chat'), { key: 'ArrowRight' })
    await waitFor(() => expect(checkedLabel()).toBe('Responses'))
    await waitFor(() => expect(document.activeElement).toBe(radio('Responses')))
    expect(radio('Responses').getAttribute('tabindex')).toBe('0')
    expect(radio('Chat').getAttribute('tabindex')).toBe('-1')
  })

  it('disables every segment together with the group', () => {
    render(<ProtocolPicker disabled />)
    for (const item of screen.getAllByRole('radio')) expect((item as HTMLButtonElement).disabled).toBe(true)
    fireEvent.click(radio('Gemini'))
    expect(checkedLabel()).toBe('Chat')
  })
})

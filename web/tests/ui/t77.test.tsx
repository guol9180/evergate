import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ModelMappingEditor from '../../src/components/ModelMappingEditor'
import { clientEnvSnippet, copyText } from '../../src/lib/clipboard'

describe('ModelMappingEditor', () => {
  it('adds a row when both fields are filled and drops empty rows', () => {
    const onChange = vi.fn()
    const view = render(<ModelMappingEditor value={{ gpt4: 'gpt-4o' }} onChange={onChange} />)
    fireEvent.change(screen.getByLabelText('新增对外模型'), { target: { value: 'claude' } })
    // 只填一个字段时不提交半成品，但输入不能丢：该行变成"第 2 行"，末尾再追加空行
    expect(onChange).toHaveBeenLastCalledWith({ gpt4: 'gpt-4o' })
    expect((screen.getByLabelText('对外模型 2') as HTMLInputElement).value).toBe('claude')
    fireEvent.change(screen.getByLabelText('上游模型 2'), { target: { value: 'claude-3' } })
    expect(onChange).toHaveBeenLastCalledWith({ gpt4: 'gpt-4o', claude: 'claude-3' })
    view.unmount()
  })

  it('removes an existing mapping and toggles to JSON mode with validation', () => {
    const onChange = vi.fn()
    render(<ModelMappingEditor value={{ gpt4: 'gpt-4o' }} onChange={onChange} />)
    fireEvent.click(screen.getByRole('button', { name: '删除第 1 行映射' }))
    expect(onChange).toHaveBeenLastCalledWith({})

    fireEvent.click(screen.getByRole('button', { name: '切换 JSON' }))
    const textarea = screen.getByRole('textbox')
    fireEvent.change(textarea, { target: { value: '[1,2]' } })
    expect(screen.getByRole('alert').textContent).toContain('对象')
    fireEvent.change(textarea, { target: { value: '{"a":"b"}' } })
    expect(screen.queryByRole('alert')).toBeNull()
    expect(onChange).toHaveBeenLastCalledWith({ a: 'b' })
  })
})

describe('clipboard fallback', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('uses navigator.clipboard when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('navigator', { clipboard: { writeText } })
    expect(await copyText('x')).toBe(true)
    expect(writeText).toHaveBeenCalledWith('x')
  })

  it('falls back to execCommand when clipboard API is missing', async () => {
    vi.stubGlobal('navigator', {})
    const exec = vi.fn().mockReturnValue(true)
    document.execCommand = exec
    expect(await copyText('y')).toBe(true)
    expect(exec).toHaveBeenCalledWith('copy')
  })

  it('reports failure when both paths fail', async () => {
    vi.stubGlobal('navigator', {})
    document.execCommand = vi.fn().mockReturnValue(false)
    expect(await copyText('z')).toBe(false)
  })
})

describe('clientEnvSnippet', () => {
  it('derives Anthropic root from the OpenAI base URL and includes the key once per client', () => {
    const s = clientEnvSnippet('eg-abc', 'http://localhost:8787/v1')
    expect(s).toContain('OPENAI_BASE_URL=http://localhost:8787/v1')
    expect(s).toContain('OPENAI_API_KEY=eg-abc')
    expect(s).toContain('ANTHROPIC_BASE_URL=http://localhost:8787')
    expect(s).toContain('ANTHROPIC_AUTH_TOKEN=eg-abc')
    expect(s).not.toContain('ANTHROPIC_BASE_URL=http://localhost:8787/v1')
  })
})

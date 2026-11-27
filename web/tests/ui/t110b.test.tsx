import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import Tokens from '../../src/pages/Tokens'

vi.mock('../../src/api/hooks', () => ({
  useTokens: () => ({ data: { items: [] }, isPending: false }),
  useModelCatalog: () => ({ data: { items: [] }, isPending: false, isError: false }),
  useCreateToken: () => ({ mutate: vi.fn(), isPending: false, isError: false, data: undefined }),
  useUpdateToken: () => ({ mutate: vi.fn(), isPending: false, isError: false }),
  useDeleteToken: () => ({ mutate: vi.fn(), isPending: false }),
}))

describe('Token dialog dirty guard (T110 ②)', () => {
  it('asks before discarding edits and closes cleanly when untouched', async () => {
    render(<MemoryRouter><Tokens /></MemoryRouter>)
    fireEvent.click(screen.getAllByRole('button', { name: '新建令牌' })[0])
    const dialog = await screen.findByRole('dialog', {}, { timeout: 4000 })
    // 未改动：Esc 直接关闭
    fireEvent.keyDown(dialog, { key: 'Escape', bubbles: true })
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull(), { timeout: 4000 })

    fireEvent.click(screen.getAllByRole('button', { name: '新建令牌' })[0])
    const reopened = await screen.findByRole('dialog', {}, { timeout: 4000 })
    fireEvent.change(screen.getByLabelText('名称'), { target: { value: 'cli-token' } })
    fireEvent.keyDown(reopened, { key: 'Escape', bubbles: true })
    expect(await screen.findByText('放弃未保存的更改？', {}, { timeout: 4000 })).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: '取消' }))
    // 取消放弃：表单还在，输入没丢
    expect(screen.getByLabelText('名称')).toHaveProperty('value', 'cli-token')
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', bubbles: true })
    fireEvent.click(await screen.findByRole('button', { name: '放弃更改' }, { timeout: 4000 }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull(), { timeout: 4000 })
  })

  it('validates the name inline before calling the API', async () => {
    render(<MemoryRouter><Tokens /></MemoryRouter>)
    fireEvent.click(screen.getAllByRole('button', { name: '新建令牌' })[0])
    await screen.findByRole('dialog', {}, { timeout: 4000 })
    fireEvent.click(screen.getByRole('button', { name: '保存' }))
    expect(await screen.findByText('请输入令牌名称', {}, { timeout: 4000 })).toBeTruthy()
    expect(screen.getByLabelText('名称').getAttribute('aria-invalid')).toBe('true')
  })
})
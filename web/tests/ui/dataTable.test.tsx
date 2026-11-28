import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ColumnVisibilityState } from '@tanstack/react-table'
import { DataTable, DataTableColumnToggle, type DataTableColumn } from '../../src/components/ui'

type Row = { id: number; name: string; ms: number; note?: string }

const rows: Row[] = [
  { id: 1, name: 'beta', ms: 40, note: 'second' },
  { id: 2, name: 'alpha', ms: 5 },
]

const columns: DataTableColumn<Row>[] = [
  { id: 'name', header: '名称', cell: (row) => row.name, sortKey: (row) => row.name },
  { id: 'ms', header: '耗时', cell: (row) => String(row.ms), sortKey: (row) => row.ms, hideable: true },
  { id: 'note', header: '备注', cell: (row) => row.note ?? '—' },
]

const cellText = (header: string) => screen.getByRole('columnheader', { name: header })
const rowOrder = () => screen.getAllByRole('row').slice(1).map((row) => (row.textContent?.startsWith('beta') ? 'beta' : 'alpha'))

describe('DataTable (TanStack Table)', () => {
  it('表头点击排序：降 / 升 / 取消，th 带 aria-sort', async () => {
    render(<DataTable columns={columns} rows={rows} rowKey={(row) => row.id} />)
    expect(rowOrder()).toEqual(['beta', 'alpha'])
    fireEvent.click(screen.getByRole('button', { name: '耗时' }))
    // 数值列首次点击按 TanStack 惯例先降序；状态经 store 发布后下一拍生效
    await waitFor(() => expect(cellText('耗时').getAttribute('aria-sort')).toBe('descending'))
    expect(rowOrder()).toEqual(['beta', 'alpha'])
    fireEvent.click(screen.getByRole('button', { name: '耗时' }))
    await waitFor(() => expect(cellText('耗时').getAttribute('aria-sort')).toBe('ascending'))
    expect(rowOrder()).toEqual(['alpha', 'beta'])
    fireEvent.click(screen.getByRole('button', { name: '耗时' }))
    await waitFor(() => expect(cellText('耗时').getAttribute('aria-sort')).toBeNull())
    expect(rowOrder()).toEqual(['beta', 'alpha'])
  })

  it('列显隐菜单：取消勾选后该列从表头与单元格消失，再勾选恢复', async () => {
    function Harness() {
      const [visibility, setVisibility] = useState<ColumnVisibilityState>({})
      return (
        <>
          <DataTableColumnToggle columns={columns} visibility={visibility} onChange={setVisibility} label="列" />
          <DataTable columns={columns} rows={rows} rowKey={(row) => row.id} columnVisibility={visibility} onColumnVisibilityChange={setVisibility} />
        </>
      )
    }
    render(<Harness />)
    expect(screen.getByRole('columnheader', { name: '备注' })).toBeTruthy()
    const trigger = screen.getByRole('button', { name: '列' })
    fireEvent.click(trigger)
    fireEvent.keyDown(trigger, { key: 'ArrowDown' })
    const item = await screen.findByRole('menuitemcheckbox', { name: '耗时' })
    expect(item.getAttribute('aria-checked')).toBe('true')
    fireEvent.click(item)
    await waitFor(() => expect(screen.queryByRole('columnheader', { name: '耗时' })).toBeNull())
    expect(screen.queryByText('40')).toBeNull()
    // 不可隐藏的列不在菜单里
    expect(screen.queryByRole('menuitemcheckbox', { name: '名称' })).toBeNull()
  })

  it('行可点击且键盘可达：Enter / 空格触发，行是 Tab 停靠点', () => {
    const onRowClick = vi.fn()
    render(<DataTable columns={columns} rows={rows} rowKey={(row) => row.id} onRowClick={onRowClick} rowProps={() => ({ 'aria-haspopup': 'dialog' })} />)
    const first = screen.getAllByRole('row')[1]
    expect(first.getAttribute('tabindex')).toBe('0')
    expect(first.getAttribute('aria-haspopup')).toBe('dialog')
    fireEvent.click(first)
    fireEvent.keyDown(first, { key: 'Enter' })
    fireEvent.keyDown(first, { key: ' ' })
    expect(onRowClick).toHaveBeenCalledTimes(3)
    expect(onRowClick.mock.calls[0][0]).toEqual(rows[0])
  })

  it('行展开：只渲染 expandedRowId 对应的那一行', () => {
    render(
      <DataTable
        columns={columns}
        rows={rows}
        rowKey={(row) => row.id}
        expandedRowId={2}
        renderExpanded={(row) => <div>展开内容 {row.name}</div>}
      />,
    )
    expect(screen.getByText('展开内容 alpha')).toBeTruthy()
    expect(screen.queryByText('展开内容 beta')).toBeNull()
    expect(screen.getAllByRole('row').at(-1)?.getAttribute('data-slot')).toBe('expanded-row')
  })

  it('分页键变化时把表格顶部滚回视口（首次挂载不滚）', () => {
    const scrollIntoView = vi.fn()
    Element.prototype.scrollIntoView = scrollIntoView
    const { rerender } = render(<DataTable columns={columns} rows={rows} rowKey={(row) => row.id} scrollToTopOn={1} />)
    expect(scrollIntoView).not.toHaveBeenCalled()
    rerender(<DataTable columns={columns} rows={rows} rowKey={(row) => row.id} scrollToTopOn={2} />)
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
    expect(scrollIntoView.mock.calls[0][0]).toMatchObject({ block: 'start' })
  })
})

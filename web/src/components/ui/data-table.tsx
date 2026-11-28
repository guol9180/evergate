// DataTable（TanStack Table v9）：全站表格唯一出处——列定义、表头排序、列显隐、行点击 / 键盘可达、行展开。
// 视觉仍是项目的 thead / Th / Td 令牌；这里只负责把列定义映射成表格结构与交互。
import { Fragment, useEffect, useMemo, useRef, useState, type HTMLAttributes, type ReactNode } from 'react'
import {
  columnVisibilityFeature,
  createSortedRowModel,
  rowSortingFeature,
  tableFeatures,
  useTable,
  type ColumnDef,
  type ColumnVisibilityState,
  type Row,
  type SortingState,
} from '@tanstack/react-table'
import { ArrowDown, ArrowUp, ChevronsUpDown, Columns3 } from 'lucide-react'
import { cn } from '../../lib/utils'
import { Th, Td } from './table-cells'
import { Button } from './button'
import { DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuTrigger } from './dropdown-menu'
import { TooltipWrap as Tooltip } from './tooltip'

const features = tableFeatures({
  rowSortingFeature,
  columnVisibilityFeature,
  sortedRowModel: createSortedRowModel(),
})

type Features = typeof features

// 表格最小宽度用类名而不是 inline style：页面传的 lg:min-w-0 要在宽屏覆盖它（inline style 会压过类名）。
// 键必须是源码里出现过的字面量，Tailwind 才会生成对应工具类。
const MIN_WIDTH_CLASS: Record<number, string> = {
  240: 'min-w-[240px]',
  300: 'min-w-[300px]',
  340: 'min-w-[340px]',
  420: 'min-w-[420px]',
  520: 'min-w-[520px]',
  560: 'min-w-[560px]',
  620: 'min-w-[620px]',
  680: 'min-w-[680px]',
  700: 'min-w-[700px]',
  720: 'min-w-[720px]',
}

export interface DataTableColumn<T> {
  id: string
  header: ReactNode
  cell: (row: T, index: number) => ReactNode
  /** 单元格类名；窄屏折叠规则（hidden min-[...]:table-cell）写在这里 */
  className?: string
  headerClassName?: string
  headerTitle?: string
  /** 给出排序键即启用表头点击排序（客户端排序，只作用于当前传入的行） */
  sortKey?: (row: T) => string | number | null | undefined
  /** 允许用户隐藏此列（默认否）；列显隐开关由调用方放在页面操作区 */
  hideable?: boolean
  /** 列显隐菜单里的文字；缺省用 header（仅当 header 是字符串） */
  toggleLabel?: string
}

export interface DataTableProps<T> {
  columns: readonly DataTableColumn<T>[]
  rows: readonly T[]
  rowKey: (row: T, index: number) => string | number
  /** 表格类名，默认 w-full + minWidth 对应的 min-w-[…]；窄屏放开的表格传 lg:min-w-0 */
  className?: string
  /** 最小宽度（px），默认 720；必须是 MIN_WIDTH_CLASS 里出现过的值 */
  minWidth?: number
  rowClassName?: (row: T) => string | undefined
  /** 整行可点击；同时 Enter / 空格触发，行获得 tabIndex=0 */
  onRowClick?: (row: T) => void
  rowProps?: (row: T) => Omit<HTMLAttributes<HTMLTableRowElement>, 'onClick' | 'onKeyDown' | 'className'>
  /** 行展开：返回该行的展开内容；expandedRowId 决定哪一行展开 */
  renderExpanded?: (row: T) => ReactNode
  expandedRowId?: string | number | null
  sorting?: SortingState
  onSortingChange?: (next: SortingState) => void
  defaultSorting?: SortingState
  columnVisibility?: ColumnVisibilityState
  onColumnVisibilityChange?: (next: ColumnVisibilityState) => void
  /** 该值变化时（通常是页码）把表格顶部滚回视口：翻页后不必再手动滚回去看表头 */
  scrollToTopOn?: unknown
}

function compareValues(a: string | number | null | undefined, b: string | number | null | undefined): number {
  if (a == null && b == null) return 0
  if (a == null) return 1
  if (b == null) return -1
  if (typeof a === 'number' && typeof b === 'number') return a - b
  return String(a).localeCompare(String(b), 'zh', { numeric: true })
}

/** 单元格类名里的响应式显示规则（hidden / min-[…]:table-cell）同步到表头，避免表头与单元格列数错位。 */
function responsiveDisplay(className?: string): string {
  return (className ?? '')
    .split(/\s+/)
    .filter((token) => token === 'hidden' || token === 'table-cell' || token.endsWith(':table-cell'))
    .join(' ')
}

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  className,
  minWidth = 720,
  rowClassName,
  onRowClick,
  rowProps,
  renderExpanded,
  expandedRowId,
  sorting: controlledSorting,
  onSortingChange: controlledOnSortingChange,
  defaultSorting,
  columnVisibility: controlledVisibility,
  onColumnVisibilityChange: controlledOnVisibilityChange,
  scrollToTopOn,
}: DataTableProps<T>) {
  const [internalSorting, setInternalSorting] = useState<SortingState>(defaultSorting ?? [])
  const [internalVisibility, setInternalVisibility] = useState<ColumnVisibilityState>({})
  const sorting = controlledSorting ?? internalSorting
  const columnVisibility = controlledVisibility ?? internalVisibility

  // TanStack v9 要求行类型满足 Record<string, any>，泛型 T 无法证明；内部只能用 any，对外的列 / 行类型由 DataTableColumn<T> 保证
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const columnDefs = useMemo<ColumnDef<Features, any>[]>(
    () =>
      columns.map((column) => ({
        id: column.id,
        // v9 只有带 accessor 的列才能排序：排序键同时充当 accessor
        ...(column.sortKey ? { accessorFn: (row: T) => column.sortKey!(row) ?? null } : {}),
        // v9 的 header / cell 是「字符串或渲染函数」：用闭包把项目的列定义包进去
        header: () => column.header,
        cell: (ctx) => column.cell(ctx.row.original, ctx.row.index),
        enableSorting: column.sortKey != null,
        sortingFn: column.sortKey
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          ? (a: Row<Features, any>, b: Row<Features, any>) => compareValues(column.sortKey!(a.original as T), column.sortKey!(b.original as T))
          : undefined,
        enableHiding: column.hideable === true,
      })),
    [columns],
  )
  const columnById = useMemo(() => new Map(columns.map((column) => [column.id, column])), [columns])
  const rootRef = useRef<HTMLDivElement>(null)
  const mounted = useRef(false)
  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true
      return
    }
    rootRef.current?.scrollIntoView({ block: 'start', behavior: 'smooth' })
  }, [scrollToTopOn])

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const table = useTable<Features, any>({
    features,
    data: rows as T[],
    columns: columnDefs,
    state: { sorting, columnVisibility },
    onSortingChange: (updater) => {
      const next = typeof updater === 'function' ? updater(sorting) : updater
      if (controlledOnSortingChange) controlledOnSortingChange(next)
      else setInternalSorting(next)
    },
    onColumnVisibilityChange: (updater) => {
      const next = typeof updater === 'function' ? updater(columnVisibility) : updater
      if (controlledOnVisibilityChange) controlledOnVisibilityChange(next)
      else setInternalVisibility(next)
    },
    getRowId: (row, index) => String(rowKey(row, index)),
  })

  return (
    <div ref={rootRef} className="overflow-x-auto">
      <table className={cn('w-full', minWidth && (MIN_WIDTH_CLASS[minWidth] ?? `min-w-[${minWidth}px]`), className)}>
        <thead className="border-b border-line-soft bg-surface-soft">
          {table.getHeaderGroups().map((headerGroup) => (
            <tr key={headerGroup.id}>
              {headerGroup.headers.map((header) => {
                const column = columnById.get(header.column.id)
                const sorted = header.column.getIsSorted()
                const canSort = header.column.getCanSort()
                return (
                  <Th
                    key={header.id}
                    className={cn(responsiveDisplay(column?.className), column?.headerClassName)}
                    title={column?.headerTitle}
                    aria-sort={sorted === 'asc' ? 'ascending' : sorted === 'desc' ? 'descending' : undefined}
                  >
                    {canSort ? (
                      <button
                        type="button"
                        onClick={header.column.getToggleSortingHandler()}
                        className="group inline-flex items-center gap-1 rounded uppercase tracking-wide outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-brand"
                      >
                        <table.FlexRender header={header} />
                        {/* 未排序时图标不占位（悬停才出现），否则十来列的表头会被图标撑出容器 */}
                        {sorted === 'asc' ? (
                          <ArrowDown size={12} aria-hidden="true" />
                        ) : sorted === 'desc' ? (
                          <ArrowUp size={12} aria-hidden="true" />
                        ) : (
                          <ChevronsUpDown size={12} className="hidden text-subtle group-hover:inline group-focus-visible:inline" aria-hidden="true" />
                        )}
                      </button>
                    ) : (
                      <table.FlexRender header={header} />
                    )}
                  </Th>
                )
              })}
            </tr>
          ))}
        </thead>
        <tbody className="divide-y divide-line-soft">
          {table.getRowModel().rows.map((row) => {
            const original = row.original
            const expanded = renderExpanded != null && expandedRowId != null && String(expandedRowId) === row.id
            const extra = rowProps?.(original) ?? {}
            return (
              <Fragment key={row.id}>
                <tr
                  {...extra}
                  data-state={expanded ? 'open' : undefined}
                  className={cn(onRowClick && 'cursor-pointer outline-none hover:bg-surface-soft focus-visible:bg-brand-subtle', rowClassName?.(original))}
                  tabIndex={onRowClick ? (extra.tabIndex ?? 0) : undefined}
                  onClick={onRowClick ? () => onRowClick(original) : undefined}
                  onKeyDown={
                    onRowClick
                      ? (event) => {
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault()
                            onRowClick(original)
                          }
                        }
                      : undefined
                  }
                >
                  {row.getVisibleCells().map((cell) => (
                    <Td key={cell.id} className={columnById.get(cell.column.id)?.className}>
                      <table.FlexRender cell={cell} />
                    </Td>
                  ))}
                </tr>
                {expanded && (
                  <tr data-slot="expanded-row">
                    <td colSpan={row.getVisibleCells().length} className="eg-cell-x eg-cell-y bg-surface-soft">
                      {renderExpanded(original)}
                    </td>
                  </tr>
                )}
              </Fragment>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

/** 列显隐菜单：与 DataTable 共用列定义与 columnVisibility 状态，放在页头操作区。 */
export function DataTableColumnToggle<T>({ columns, visibility, onChange, label }: {
  columns: readonly DataTableColumn<T>[]
  visibility: ColumnVisibilityState
  onChange: (next: ColumnVisibilityState) => void
  label: string
}) {
  const hideable = columns.filter((column) => column.hideable)
  if (hideable.length === 0) return null
  return (
    <DropdownMenu>
      <Tooltip content={label}>
        <DropdownMenuTrigger asChild>
          <Button type="button" variant="outline" size="icon-sm" aria-label={label}>
            <Columns3 size={14} aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
      </Tooltip>
      <DropdownMenuContent align="end" className="z-[60] min-w-40 border border-line bg-surface shadow-lg">
        {hideable.map((column) => (
          <DropdownMenuCheckboxItem
            key={column.id}
            checked={visibility[column.id] !== false}
            onCheckedChange={(checked) => onChange({ ...visibility, [column.id]: checked })}
            onSelect={(event) => event.preventDefault()}
            className="text-sm"
          >
            {column.toggleLabel ?? (typeof column.header === 'string' ? column.header : column.id)}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

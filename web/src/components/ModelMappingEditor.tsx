import { useEffect, useState } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import { Button, ErrorText, Textarea } from './ui'

type Mapping = Record<string, string>
type Row = { from: string; to: string }

const EMPTY_ROW: Row = { from: '', to: '' }

function toRows(value: Mapping): Row[] {
  return Object.entries(value).map(([from, to]) => ({ from, to }))
}

function mappingFrom(rows: Row[]): Mapping {
  const next: Mapping = {}
  for (const { from, to } of rows) {
    if (from.trim() && to.trim()) next[from.trim()] = to.trim()
  }
  return next
}

/**
 * 模型映射编辑器：默认「对外模型 → 上游模型」两列表格，可切换到 JSON 高级模式。
 * 行状态保存在本地（末行恒为空供新增），否则只填一个字段时受控重建会把输入吞掉；
 * 仅当外部 value 与本地提交结果不一致（预设/重置）时才重新同步。
 */
export default function ModelMappingEditor({ value, onChange }: { value: Mapping; onChange: (next: Mapping) => void }) {
  const [mode, setMode] = useState<'table' | 'json'>('table')
  const [rows, setRows] = useState<Row[]>(() => [...toRows(value), EMPTY_ROW])
  const [jsonText, setJsonText] = useState(() => (Object.keys(value).length ? JSON.stringify(value, null, 2) : ''))
  const [jsonError, setJsonError] = useState('')

  useEffect(() => {
    if (JSON.stringify(mappingFrom(rows)) !== JSON.stringify(value)) {
      setRows([...toRows(value), EMPTY_ROW])
    }
  }, [value])

  const commit = (next: Row[]) => {
    const withTail = next.length === 0 || next[next.length - 1].from.trim() || next[next.length - 1].to.trim() ? [...next, EMPTY_ROW] : next
    setRows(withTail)
    onChange(mappingFrom(withTail))
  }

  const updateRow = (index: number, field: keyof Row, text: string) => {
    commit(rows.map((row, i) => (i === index ? { ...row, [field]: text } : row)))
  }

  const removeRow = (index: number) => commit(rows.filter((_, i) => i !== index))

  const switchToJson = () => {
    setJsonText(Object.keys(value).length ? JSON.stringify(value, null, 2) : '')
    setJsonError('')
    setMode('json')
  }

  const applyJson = (text: string) => {
    setJsonText(text)
    if (!text.trim()) {
      setJsonError('')
      onChange({})
      return
    }
    try {
      const parsed: unknown = JSON.parse(text)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('必须是 {对外模型: 上游模型} 对象')
      const next: Mapping = {}
      for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
        if (typeof v !== 'string') throw new Error(`键 ${k} 的值必须是字符串`)
        next[k] = v
      }
      setJsonError('')
      onChange(next)
    } catch (err) {
      setJsonError((err as Error).message)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-sm font-medium text-foreground-soft">模型映射（可选）</span>
        <Button type="button" size="sm" variant="ghost" onClick={() => (mode === 'table' ? switchToJson() : setMode('table'))}>
          {mode === 'table' ? '切换 JSON' : '切换表格'}
        </Button>
      </div>
      {mode === 'table' ? (
        <>
          <div className="overflow-x-auto rounded-lg border border-line">
            <table className="w-full min-w-[420px] text-sm">
              <thead className="bg-surface-soft">
                <tr>
                  <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">对外模型（客户端请求的模型名）</th>
                  <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">上游模型（真实发送给供应商）</th>
                  <th scope="col" className="w-12" />
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {rows.map((row, i) => {
                  const isNew = i === rows.length - 1 && !row.from && !row.to
                  return (
                    <tr key={i}>
                      <td className="eg-cell-x eg-cell-y">
                        <input
                          aria-label={isNew ? '新增对外模型' : `对外模型 ${i + 1}`}
                          value={row.from}
                          placeholder={isNew ? '例如 gpt-4' : ''}
                          onChange={(e) => updateRow(i, 'from', e.target.value)}
                          className="h-8 w-full rounded-md border border-line-strong bg-surface px-2 text-sm text-foreground outline-none focus:border-brand"
                        />
                      </td>
                      <td className="eg-cell-x eg-cell-y">
                        <input
                          aria-label={isNew ? '新增上游模型' : `上游模型 ${i + 1}`}
                          value={row.to}
                          placeholder={isNew ? '例如 gpt-4o-2024' : ''}
                          onChange={(e) => updateRow(i, 'to', e.target.value)}
                          className="h-8 w-full rounded-md border border-line-strong bg-surface px-2 text-sm text-foreground outline-none focus:border-brand"
                        />
                      </td>
                      <td className="eg-cell-x eg-cell-y text-right">
                        {isNew ? (
                          <span className="inline-flex items-center gap-1 text-xs text-subtle"><Plus size={12} aria-hidden="true" />新增</span>
                        ) : (
                          <button type="button" onClick={() => removeRow(i)} aria-label={`删除第 ${i + 1} 行映射`} title="删除" className="rounded p-1 text-subtle hover:bg-danger-subtle hover:text-danger-text">
                            <Trash2 size={13} aria-hidden="true" />
                          </button>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <p className="text-xs text-subtle">留空表示不映射；对外模型必须同时出现在「支持的模型」列表中。</p>
        </>
      ) : (
        <>
          <Textarea rows={4} value={jsonText} onChange={(e) => applyJson(e.target.value)} placeholder='{"gpt-4": "gpt-4o-2024"}' aria-invalid={jsonError ? true : undefined} className="font-mono text-xs" />
          <ErrorText>{jsonError}</ErrorText>
        </>
      )}
    </div>
  )
}

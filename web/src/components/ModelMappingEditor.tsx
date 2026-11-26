import { useEffect, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { Button, FieldError, IconButton, Input, Textarea } from './ui'
import { t, tf } from '../lib/i18n'

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
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error(t('mapping.jsonInvalid'))
      const next: Mapping = {}
      for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
        if (typeof v !== 'string') throw new Error(tf('mapping.valueInvalid', { key: k }))
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
        <span className="text-sm font-medium text-foreground-soft">{t('mapping.title')}</span>
        <Button type="button" size="sm" variant="ghost" onClick={() => (mode === 'table' ? switchToJson() : setMode('table'))}>
          {mode === 'table' ? t('mapping.toJson') : t('mapping.toTable')}
        </Button>
      </div>
      {mode === 'table' ? (
        <>
          <div className="overflow-x-auto rounded-lg border border-line">
            <table className="w-full min-w-[420px] text-sm">
              <thead className="bg-surface-soft">
                <tr>
                  <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">{t('mapping.publicHeader')}</th>
                  <th scope="col" className="eg-cell-x eg-cell-y text-left text-xs font-semibold text-muted">{t('mapping.upstreamHeader')}</th>
                  <th scope="col" className="w-12" />
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {rows.map((row, i) => {
                  const isNew = i === rows.length - 1 && !row.from && !row.to
                  return (
                    <tr key={i}>
                      <td className="eg-cell-x eg-cell-y">
                        <Input
                          aria-label={isNew ? t('mapping.newPublicAria') : tf('mapping.publicAria', { n: i + 1 })}
                          value={row.from}
                          placeholder={isNew ? t('mapping.publicPlaceholder') : ''}
                          onChange={(e) => updateRow(i, 'from', e.target.value)}
                          className="h-8 px-2"
                        />
                      </td>
                      <td className="eg-cell-x eg-cell-y">
                        <Input
                          aria-label={isNew ? t('mapping.newUpstreamAria') : tf('mapping.upstreamAria', { n: i + 1 })}
                          value={row.to}
                          placeholder={isNew ? t('mapping.upstreamPlaceholder') : ''}
                          onChange={(e) => updateRow(i, 'to', e.target.value)}
                          className="h-8 px-2"
                        />
                      </td>
                      <td className="eg-cell-x eg-cell-y text-right">
                        {isNew ? (
                          <span className="inline-block whitespace-nowrap text-xs text-subtle" title={t('mapping.addRowTitle')}>{t('mapping.addRow')}</span>
                        ) : (
                          <IconButton danger onClick={() => removeRow(i)} label={tf('mapping.deleteRowAria', { n: i + 1 })}>
                            <Trash2 size={13} aria-hidden="true" />
                          </IconButton>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <p className="text-xs text-subtle">{t('mapping.hint')}</p>
        </>
      ) : (
        <>
          <Textarea rows={4} value={jsonText} onChange={(e) => applyJson(e.target.value)} placeholder='{"gpt-4": "gpt-4o-2024"}' aria-invalid={jsonError ? true : undefined} className="font-mono text-xs" />
          {jsonError && <FieldError>{jsonError}</FieldError>}
        </>
      )}
    </div>
  )
}

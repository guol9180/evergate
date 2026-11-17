import { useMemo, useState } from 'react'
import { Check, Search } from 'lucide-react'
import { Badge, Button, ErrorText, Input, Spinner } from './ui'

export interface ModelPickerOption {
  publicModel: string
  upstreamModel: string
  context?: string
}

interface ModelPickerProps {
  title: string
  options: ModelPickerOption[]
  initialModels: string[]
  loading?: boolean
  error?: string
  truncated?: boolean
  protocol?: string
  onApply: (models: string[], mode: 'append' | 'replace') => void
  onCancel: () => void
}

export default function ModelPicker({
  title,
  options,
  initialModels,
  loading = false,
  error,
  truncated = false,
  protocol,
  onApply,
  onCancel,
}: ModelPickerProps) {
  const [query, setQuery] = useState('')
  const [mode, setMode] = useState<'append' | 'replace'>('append')
  const [selected, setSelected] = useState(() => {
    const available = new Set(options.map((option) => option.publicModel))
    return new Set(initialModels.filter((model) => available.has(model)))
  })

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase()
    if (!needle) return options
    return options.filter((option) =>
      `${option.publicModel} ${option.upstreamModel} ${option.context ?? ''}`.toLowerCase().includes(needle),
    )
  }, [options, query])

  function toggle(model: string) {
    setSelected((current) => {
      const next = new Set(current)
      if (next.has(model)) next.delete(model)
      else next.add(model)
      return next
    })
  }

  return (
    <div className="rounded-lg border border-indigo-200 bg-indigo-50/40 p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <div className="text-sm font-medium text-slate-800">{title}</div>
          <div className="mt-0.5 text-xs text-slate-500">勾选后确认；不会自动保存当前表单。</div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {protocol && <Badge tone="bg-indigo-100 text-indigo-700">{protocol}</Badge>}
          <Badge tone="bg-white text-slate-600 ring-1 ring-inset ring-slate-200">{options.length} 项</Badge>
          <Badge tone="bg-white text-emerald-700 ring-1 ring-inset ring-emerald-200">已选 {selected.size}</Badge>
        </div>
      </div>

      <div className="relative mt-3">
        <Search size={14} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-slate-400" />
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="pl-8"
          placeholder="搜索模型或映射"
          aria-label={`搜索${title}`}
          disabled={loading}
        />
      </div>

      {loading ? (
        <div className="flex items-center justify-center gap-2 py-8 text-sm text-slate-500"><Spinner /> 拉取中…</div>
      ) : error ? (
        <div className="mt-3"><ErrorText>{error}</ErrorText></div>
      ) : visible.length === 0 ? (
        <div className="py-8 text-center text-sm text-slate-400">
          {options.length === 0 ? '上游没有返回可选择的模型' : '没有匹配的模型'}
        </div>
      ) : (
        <div className="mt-3 max-h-64 overflow-y-auto rounded-md border border-slate-200 bg-white">
          {visible.map((option, index) => {
            const checked = selected.has(option.publicModel)
            const mapped = option.publicModel !== option.upstreamModel
            return (
              <label
                key={`${option.publicModel}|${option.upstreamModel}|${option.context ?? ''}|${index}`}
                className="flex cursor-pointer items-start gap-2.5 border-b border-slate-100 px-3 py-2.5 last:border-b-0 hover:bg-slate-50"
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => toggle(option.publicModel)}
                  className="mt-0.5 h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
                />
                <span className="min-w-0 flex-1">
                  <span className="block break-all text-sm font-medium text-slate-700">{option.publicModel}</span>
                  <span className={`mt-0.5 block break-all text-xs ${mapped ? 'text-indigo-600' : 'text-slate-400'}`}>
                    {mapped ? `${option.publicModel} → ${option.upstreamModel}` : `上游同名 ${option.upstreamModel}`}
                    {option.context && <span className="text-slate-400"> · {option.context}</span>}
                  </span>
                </span>
                {checked && <Check size={15} className="mt-0.5 shrink-0 text-emerald-600" />}
              </label>
            )
          })}
        </div>
      )}

      {truncated && (
        <div className="mt-2 text-xs text-amber-700">结果已达到 10 页或 500 条上限，仅展示已拉取部分。</div>
      )}

      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-xs text-slate-600">
          <span>写入方式</span>
          <label className="inline-flex cursor-pointer items-center gap-1">
            <input type="radio" checked={mode === 'append'} onChange={() => setMode('append')} />
            追加
          </label>
          <label className="inline-flex cursor-pointer items-center gap-1">
            <input type="radio" checked={mode === 'replace'} onChange={() => setMode('replace')} />
            替换
          </label>
        </div>
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={onCancel}>取消</Button>
          <Button type="button" disabled={selected.size === 0 || loading || Boolean(error)} onClick={() => onApply([...selected].sort(), mode)}>
            确认写入
          </Button>
        </div>
      </div>
    </div>
  )
}

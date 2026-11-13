import { useEffect, useMemo, useRef, useState } from 'react'
import { FlaskConical, Play, Square } from 'lucide-react'
import { useChannels } from '../api/hooks'
import { runPlayground, type PlaygroundMeta } from '../api/playground'
import RetryTimeline from '../components/RetryTimeline'
import { Badge, Button, Card, Empty, Select, Spinner, Switch, Textarea } from '../components/ui'
import { fmtInt, fmtMs, kindText } from '../lib/format'

const MAX_PROMPT = 16000
const MAX_SYSTEM = 8000

export default function Playground() {
  const channels = useChannels()
  const [protocol, setProtocol] = useState<'chat' | 'responses'>('chat')
  const [model, setModel] = useState('')
  const [system, setSystem] = useState('')
  const [prompt, setPrompt] = useState('')
  const [stream, setStream] = useState(true)
  const [output, setOutput] = useState('')
  const [meta, setMeta] = useState<PlaygroundMeta | null>(null)
  const [error, setError] = useState('')
  const [running, setRunning] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const outputRef = useRef<HTMLDivElement>(null)

  const models = useMemo(() => {
    const set = new Set<string>()
    for (const channel of channels.data?.items ?? []) {
      if (!channel.enabled) continue
      for (const item of channel.models ?? []) set.add(item)
    }
    return [...set].sort()
  }, [channels.data])

  useEffect(() => {
    if (models.length === 0) return
    if (!model || !models.includes(model)) setModel(models[0])
  }, [models, model])

  useEffect(() => {
    const box = outputRef.current
    if (box) box.scrollTop = box.scrollHeight
  }, [output])

  async function send() {
    if (running || !model || !prompt.trim()) return
    const controller = new AbortController()
    abortRef.current = controller
    setRunning(true)
    setError('')
    setOutput('')
    setMeta(null)
    try {
      const result = await runPlayground(
        { protocol, model, system: system.trim(), prompt: prompt.trim(), stream },
        { onDelta: (delta) => setOutput((current) => current + delta), signal: controller.signal },
      )
      setMeta(result.meta)
      if (result.streamError) {
        setError(result.streamError)
      } else if (result.meta && result.meta.status >= 400) {
        setError(result.meta.error_message || `上游返回 ${result.meta.status}`)
      } else if (!result.text && result.meta?.status === 200) {
        setError('上游返回成功，但没有可显示的文本内容')
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      setError(message === 'The user aborted a request.' ? '已中断本次请求，已输出的内容会保留。' : message)
    } finally {
      abortRef.current = null
      setRunning(false)
    }
  }

  function stop() {
    abortRef.current?.abort()
  }

  return (
    <div className="space-y-4">
      <header className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-lg font-bold"><FlaskConical size={18} /> Playground</h1>
          <p className="mt-1 max-w-2xl text-xs leading-5 text-slate-500">
            用管理台会话直接验证渠道与协议兼容性。请求复用网关调度与健康规则，并写入请求日志的 Playground 来源。
          </p>
        </div>
        <Badge tone="self-start bg-indigo-50 text-indigo-700 ring-1 ring-inset ring-indigo-100">管理台专用 · 不暴露密钥</Badge>
      </header>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,380px)_minmax(0,1fr)]">
        <Card className="min-w-0 self-start p-4 sm:p-5">
          <div className="space-y-4">
            <div>
              <div className="mb-1.5 flex items-baseline justify-between gap-2">
                <span className="text-sm font-medium text-slate-700">协议</span>
                <span className="text-[11px] text-slate-400">Chat Completions / Responses API</span>
              </div>
              <div className="grid grid-cols-2 gap-2">
                {([
                  ['chat', 'Chat'],
                  ['responses', 'Responses'],
                ] as const).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setProtocol(value)}
                    disabled={running}
                    className={`h-9 rounded-md border text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 focus-visible:ring-offset-2 disabled:opacity-60 ${
                      protocol === value
                        ? 'border-indigo-300 bg-indigo-50 text-indigo-700'
                        : 'border-slate-300 bg-white text-slate-600 hover:bg-slate-50'
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <span className="text-sm font-medium text-slate-700">模型</span>
              <Select
                value={model}
                onChange={(e) => setModel(e.target.value)}
                disabled={running || models.length === 0}
                aria-label="模型"
              >
                {models.length === 0 && <option value="">暂无可用模型</option>}
                {models.map((item) => <option key={item} value={item}>{item}</option>)}
              </Select>
              {models.length === 0 && (
                <p className="text-xs text-amber-600">请先在「渠道管理」添加启用渠道与模型。</p>
              )}
            </div>

            <div className="space-y-1.5">
              <div className="flex items-baseline justify-between">
                <span className="text-sm font-medium text-slate-700">System 消息 <span className="font-normal text-slate-400">可选</span></span>
                <span className="text-[11px] tabular-nums text-slate-400">{system.length}/{MAX_SYSTEM}</span>
              </div>
              <Textarea
                rows={3}
                value={system}
                maxLength={MAX_SYSTEM}
                onChange={(e) => setSystem(e.target.value)}
                placeholder="例如：你是一个简洁的中文助手"
                disabled={running}
              />
            </div>

            <div className="space-y-1.5">
              <div className="flex items-baseline justify-between">
                <span className="text-sm font-medium text-slate-700">User 消息</span>
                <span className="text-[11px] tabular-nums text-slate-400">{prompt.length}/{MAX_PROMPT}</span>
              </div>
              <Textarea
                rows={7}
                value={prompt}
                maxLength={MAX_PROMPT}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="输入要发送给模型的内容"
                disabled={running}
              />
            </div>

            <div className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 bg-slate-50/60 px-3 py-2.5">
              <div className="min-w-0">
                <div className="text-sm text-slate-700">流式输出</div>
                <div className="text-[11px] text-slate-400">逐段渲染增量内容</div>
              </div>
              <Switch checked={stream} onChange={setStream} disabled={running} label="流式输出" />
            </div>

            <div className="flex gap-2 pt-0.5">
              <Button onClick={send} disabled={running || !model || !prompt.trim()} className="flex-1">
                {running ? <Spinner className="h-3.5 w-3.5 border-white/40 border-t-white" /> : <Play size={14} />}
                {running ? '请求中…' : '发送'}
              </Button>
              <Button variant="outline" onClick={stop} disabled={!running} aria-label="停止请求">
                <Square size={13} /> 停止
              </Button>
            </div>
          </div>
        </Card>

        <Card className="flex min-w-0 flex-col overflow-hidden">
          <div className="flex flex-wrap items-center gap-2 border-b border-slate-100 px-4 py-3 sm:px-5">
            <h2 className="text-sm font-semibold text-slate-800">响应</h2>
            <Badge tone="bg-slate-100 text-slate-600">{protocol === 'chat' ? 'Chat Completions' : 'Responses'}</Badge>
            {meta && (
              <div className="ml-auto flex items-center gap-2">
                <span className={`rounded-full px-2 py-0.5 text-[11px] font-semibold ring-1 ring-inset ${statusPill(meta.status)}`}>
                  HTTP {meta.status}
                </span>
                <span className="text-[11px] text-slate-400">{meta.retry_trace.length} 次尝试</span>
              </div>
            )}
          </div>

          <div ref={outputRef} aria-live="polite" className="max-h-[58vh] min-h-[300px] flex-1 overflow-auto px-4 py-4 sm:px-5">
            {output ? (
              <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-slate-700">
                {output}
                {running && <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-indigo-500 align-middle" />}
              </pre>
            ) : running ? (
              <div className="flex items-center gap-2 py-12 text-sm text-slate-500"><Spinner /> 等待首个 token…</div>
            ) : (
              <Empty icon={<FlaskConical size={34} />} title="还没有响应" hint="选择协议与模型，输入消息后发送；流式响应会逐段显示。" />
            )}
          </div>

          {error && (
            <div className="border-t border-red-100 bg-red-50 px-4 py-2.5 text-xs leading-5 text-red-600 sm:px-5">
              <span className="break-all">{error}</span>
            </div>
          )}

          {meta && (
            <div className="border-t border-slate-100 bg-slate-50/60 px-4 py-4 sm:px-5">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-slate-500">命中链路</h3>
                <span className="text-[11px] text-slate-400">已写入请求日志 · source=playground</span>
              </div>
              <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
                <MetaItem label="渠道" value={meta.channel_name || '—'} hint={meta.channel_id ? `#${meta.channel_id}` : ''} />
                <MetaItem label="上游模型" value={meta.upstream_model || '—'} />
                <MetaItem label="首 token" value={fmtMs(meta.first_token_ms || undefined)} />
                <MetaItem label="总耗时" value={fmtMs(meta.total_ms || undefined)} />
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5 text-[11px] text-slate-600">
                {meta.usage.usage_seen ? (
                  <>
                    <UsageChip label="输入" value={fmtInt(meta.usage.prompt_tokens)} />
                    <UsageChip label="输出" value={fmtInt(meta.usage.completion_tokens)} />
                    <UsageChip label="合计" value={fmtInt(meta.usage.total_tokens)} />
                    {meta.usage.cached_tokens > 0 && <UsageChip label="cached" value={fmtInt(meta.usage.cached_tokens)} />}
                    {meta.usage.reasoning_tokens > 0 && <UsageChip label="reasoning" value={fmtInt(meta.usage.reasoning_tokens)} />}
                  </>
                ) : (
                  <span className="text-slate-400">上游未返回 usage</span>
                )}
              </div>
              {meta.error_message && (
                <div className="mt-3 rounded-lg border border-red-100 bg-red-50 px-3 py-2 text-xs text-red-600">
                  {meta.error_type && <span className="mr-2 font-semibold">{kindText(meta.error_type)}</span>}
                  <span className="break-all">{meta.error_message}</span>
                </div>
              )}
              <div className="mt-4 border-t border-slate-200/70 pt-3">
                <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-500">重试轨迹</h4>
                <RetryTimeline attempts={meta.retry_trace} />
              </div>
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}

function statusPill(code: number): string {
  if (code >= 200 && code < 300) return 'bg-emerald-50 text-emerald-700 ring-emerald-200'
  if (code >= 400 && code < 500) return 'bg-amber-50 text-amber-700 ring-amber-200'
  return 'bg-red-50 text-red-700 ring-red-200'
}

function UsageChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-md bg-white px-2 py-1 ring-1 ring-inset ring-slate-200">
      <span className="text-slate-400">{label}</span>
      <span className="font-medium tabular-nums text-slate-700">{value}</span>
    </span>
  )
}

function MetaItem({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] text-slate-400">{label}</div>
      <div className="break-all tabular-nums text-slate-700">
        {value}
        {hint && <span className="ml-1 text-xs text-slate-400">{hint}</span>}
      </div>
    </div>
  )
}

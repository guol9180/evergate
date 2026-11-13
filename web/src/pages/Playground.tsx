import { useEffect, useMemo, useRef, useState } from 'react'
import { FlaskConical, Play, Square } from 'lucide-react'
import { useChannels } from '../api/hooks'
import { runPlayground, type PlaygroundMeta } from '../api/playground'
import RetryTimeline from '../components/RetryTimeline'
import { Badge, Button, Card, Empty, ErrorText, Field, Spinner, Textarea } from '../components/ui'
import { fmtInt, fmtMs, kindText, statusTone } from '../lib/format'

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
      setError(message === 'The user aborted a request.' ? '已中断本次请求' : message)
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
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h1 className="flex items-center gap-2 text-lg font-bold"><FlaskConical size={18} /> Playground</h1>
          <p className="mt-0.5 text-xs text-slate-500">用管理台会话直接验证渠道与协议兼容性，请求会进入请求日志并标记为 Playground 来源。</p>
        </div>
        <Badge tone="bg-indigo-50 text-indigo-700">管理台专用 · 不暴露密钥</Badge>
      </div>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,380px)_minmax(0,1fr)]">
        <Card className="min-w-0 space-y-4 p-4">
          <div>
            <span className="text-sm font-medium text-slate-700">协议</span>
            <div className="mt-1.5 grid grid-cols-2 gap-2">
              {([
                ['chat', 'Chat Completions'],
                ['responses', 'Responses'],
              ] as const).map(([value, label]) => (
                <button
                  key={value}
                  type="button"
                  onClick={() => setProtocol(value)}
                  disabled={running}
                  className={`rounded-md border px-3 py-2 text-xs font-medium transition-colors ${
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

          <Field label="模型">
            <select
              value={model}
              onChange={(e) => setModel(e.target.value)}
              disabled={running || models.length === 0}
              className="h-9 w-full rounded-md border border-slate-300 bg-white px-3 text-sm outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-100 disabled:bg-slate-50"
            >
              {models.length === 0 && <option value="">暂无可用模型</option>}
              {models.map((item) => <option key={item} value={item}>{item}</option>)}
            </select>
          </Field>
          {models.length === 0 && (
            <p className="text-xs text-amber-600">请先在「渠道管理」添加启用渠道与模型。</p>
          )}

          <Field label="System 消息（可选）" hint={`${system.length}/${MAX_SYSTEM}`}>
            <Textarea
              rows={3}
              value={system}
              maxLength={MAX_SYSTEM}
              onChange={(e) => setSystem(e.target.value)}
              placeholder="例如：你是一个简洁的中文助手"
              disabled={running}
            />
          </Field>

          <Field label="User 消息" hint={`${prompt.length}/${MAX_PROMPT}`}>
            <Textarea
              rows={8}
              value={prompt}
              maxLength={MAX_PROMPT}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="输入要发送给模型的内容"
              disabled={running}
            />
          </Field>

          <label className="flex items-center justify-between rounded-md border border-slate-200 bg-slate-50/60 px-3 py-2">
            <span className="text-sm text-slate-700">流式输出</span>
            <input
              type="checkbox"
              checked={stream}
              onChange={(e) => setStream(e.target.checked)}
              disabled={running}
              className="h-4 w-4 accent-indigo-600"
            />
          </label>

          <div className="flex gap-2">
            <Button onClick={send} disabled={running || !model || !prompt.trim()} className="flex-1">
              {running ? <Spinner className="h-3.5 w-3.5 border-white/40 border-t-white" /> : <Play size={14} />}
              {running ? '请求中…' : '发送'}
            </Button>
            <Button variant="outline" onClick={stop} disabled={!running}>
              <Square size={13} /> 停止
            </Button>
          </div>
        </Card>

        <div className="min-w-0 space-y-4">
          <Card className="min-w-0 overflow-hidden">
            <div className="flex items-center justify-between border-b border-slate-100 px-4 py-3">
              <h2 className="text-sm font-semibold text-slate-800">响应</h2>
              {meta && <Badge tone="bg-slate-100 text-slate-600">{meta.protocol === 'chat' ? 'Chat Completions' : 'Responses'}</Badge>}
            </div>
            <div className="min-h-[320px] px-4 py-4">
              {output ? (
                <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-slate-700">{output}</pre>
              ) : running ? (
                <div className="flex items-center gap-2 py-10 text-sm text-slate-500"><Spinner /> 等待首个 token…</div>
              ) : (
                <Empty icon={<FlaskConical size={34} />} title="还没有响应" hint="选择协议与模型，输入消息后发送；流式响应会逐段显示。" />
              )}
            </div>
          </Card>

          <ErrorText>{error}</ErrorText>
          {meta && <MetaPanel meta={meta} />}
        </div>
      </div>
    </div>
  )
}

function MetaPanel({ meta }: { meta: PlaygroundMeta }) {
  return (
    <Card className="min-w-0 p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-slate-800">命中链路</h2>
        <div className="flex items-center gap-2 text-xs text-slate-500">
          <span className={`font-semibold ${statusTone(meta.status)}`}>HTTP {meta.status}</span>
          <span>{meta.retry_trace.length} 次尝试</span>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-4">
        <MetaItem label="渠道" value={meta.channel_name || '—'} hint={meta.channel_id ? `#${meta.channel_id}` : ''} />
        <MetaItem label="上游模型" value={meta.upstream_model || '—'} />
        <MetaItem label="首 token" value={fmtMs(meta.first_token_ms || undefined)} />
        <MetaItem label="总耗时" value={fmtMs(meta.total_ms || undefined)} />
      </div>
      <div className="mt-3 rounded-lg bg-slate-50 px-3 py-2 text-xs text-slate-600">
        {meta.usage.usage_seen ? (
          <span>
            输入 {fmtInt(meta.usage.prompt_tokens)} · 输出 {fmtInt(meta.usage.completion_tokens)} · 合计 {fmtInt(meta.usage.total_tokens)}
            {meta.usage.cached_tokens > 0 && <> · cached {fmtInt(meta.usage.cached_tokens)}</>}
            {meta.usage.reasoning_tokens > 0 && <> · reasoning {fmtInt(meta.usage.reasoning_tokens)}</>}
          </span>
        ) : (
          <span>上游未返回 usage</span>
        )}
      </div>
      {meta.error_message && (
        <div className="mt-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600">
          {meta.error_type && <span className="mr-2 font-semibold">{kindText(meta.error_type)}</span>}
          <span className="break-all">{meta.error_message}</span>
        </div>
      )}
      <div className="mt-4">
        <h3 className="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-500">重试轨迹</h3>
        <RetryTimeline attempts={meta.retry_trace} />
      </div>
    </Card>
  )
}

function MetaItem({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] text-slate-400">{label}</div>
      <div className="break-all text-slate-700">{value}{hint && <span className="ml-1 text-xs text-slate-400">{hint}</span>}</div>
    </div>
  )
}

import { useEffect, useMemo, useRef, useState } from 'react'
import { FlaskConical, Play, Square } from 'lucide-react'
import { useChannels } from '../api/hooks'
import { runPlayground, type PlaygroundMeta } from '../api/playground'
import RetryTimeline from '../components/RetryTimeline'
import { Badge, Button, Card, Empty, PageHeader, Select, SettingRow, Spinner, Textarea } from '../components/ui'
import { describeUpstreamError, fmtInt, fmtMs, fmtRequestCost, httpStatusTone } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'

const MAX_PROMPT = 16000
const MAX_SYSTEM = 8000

export default function Playground() {
  usePageTitle(t('page.playground.title'))
  const channels = useChannels()
  const [protocol, setProtocol] = useState<'chat' | 'responses' | 'anthropic' | 'gemini'>('chat')
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
        setError(message === 'The user aborted a request.' ? t('playground.aborted') : message)
    } finally {
      abortRef.current = null
      setRunning(false)
    }
  }

  function stop() {
    abortRef.current?.abort()
  }

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={<span className="flex items-center gap-2"><FlaskConical size={18} /> {t('page.playground.title')}</span>}
        description={t('page.playground.desc')}
        actions={<Badge tone="brand">管理台专用 · 不暴露密钥</Badge>}
      />

      <div className="eg-grid lg:grid-cols-[clamp(280px,24vw,420px)_minmax(0,1fr)]">
        <Card className="min-w-0 self-start">
          <div className="space-y-[var(--eg-control-gap)]">
            <div>
              <div className="mb-1.5 flex items-baseline justify-between gap-2">
                <span className="text-sm font-medium text-foreground-soft">协议</span>
                <span className="text-[11px] text-subtle">Chat / Responses / Messages / Gemini</span>
              </div>
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-2">
                {([
                  ['chat', 'Chat'],
                  ['responses', 'Responses'],
                  ['anthropic', 'Messages'],
                  ['gemini', 'Gemini'],
                ] as const).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => setProtocol(value)}
                    disabled={running}
                    className={`h-9 rounded-md border text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 disabled:opacity-60 ${
                      protocol === value
                        ? 'border-brand-border bg-brand-subtle text-brand-text'
                        : 'border-line-strong bg-surface text-muted hover:bg-surface-soft'
                    }`}
                  >
                    {label}
                  </button>
                ))}
              </div>
            </div>

            <div className="space-y-1.5">
              <span className="text-sm font-medium text-foreground-soft">模型</span>
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
                <p className="text-xs text-warning-text">请先在「渠道管理」添加启用渠道与模型。</p>
              )}
            </div>

            <div className="space-y-1.5">
              <div className="flex items-baseline justify-between">
                <span className="text-sm font-medium text-foreground-soft">System 消息 <span className="font-normal text-subtle">可选</span></span>
                <span className="text-[11px] tabular-nums text-subtle">{system.length}/{MAX_SYSTEM}</span>
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
                <span className="text-sm font-medium text-foreground-soft">User 消息</span>
                <span className="text-[11px] tabular-nums text-subtle">{prompt.length}/{MAX_PROMPT}</span>
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

            <SettingRow
              title={t('playground.stream')}
              description={t('playground.streamHint')}
              checked={stream}
              onChange={setStream}
              disabled={running}
              label={t('playground.stream')}
            />

            <div className="flex gap-2 pt-0.5">
              <Button onClick={send} disabled={running || !model || !prompt.trim()} className="flex-1">
                {running ? <Spinner className="h-3.5 w-3.5 border-white/40 border-t-white" /> : <Play size={14} />}
                {running ? t('common.sending') : t('common.send')}
              </Button>
              <Button variant="outline" onClick={stop} disabled={!running} aria-label="停止请求">
                <Square size={13} /> {t('common.stop')}
              </Button>
            </div>
          </div>
        </Card>

        <Card flush className="flex min-w-0 flex-col overflow-hidden">
          <div className="flex flex-wrap items-center gap-2 border-b border-line-soft" style={{ padding: 'var(--eg-cell-y) var(--eg-card-pad)' }}>
            <h2 className="text-sm font-semibold text-foreground-soft">响应</h2>
            <Badge tone="neutral">{protocolLabel(protocol)}</Badge>
            {meta && (
              <div className="ml-auto flex items-center gap-2">
                <Badge tone={httpStatusTone(meta.status)} className="font-semibold">HTTP {meta.status}</Badge>
                <span className="text-[11px] text-subtle">{meta.retry_trace.length} 次尝试</span>
              </div>
            )}
          </div>

          <div ref={outputRef} aria-live="polite" className="max-h-[58vh] min-h-[120px] overflow-auto" style={{ padding: 'var(--eg-card-pad)' }}>
            {output ? (
              <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-foreground-soft">
                {output}
                {running && <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-brand align-middle" />}
              </pre>
            ) : running ? (
              <div className="flex items-center gap-2 py-12 text-sm text-muted"><Spinner /> {t('playground.waiting')}</div>
            ) : (
              <Empty icon={<FlaskConical size={34} />} title={t('empty.playground.title')} hint={t('empty.playground.hint')} />
            )}
          </div>

          {error && (
            <div className="border-t border-danger-border bg-danger-subtle text-xs leading-5 text-danger-text" style={{ padding: 'var(--eg-cell-y) var(--eg-card-pad)' }}>
              <span className="break-words" title={error}>{describeUpstreamError(error).text}</span>
            </div>
          )}

          {meta && (
            <div className="border-t border-line-soft bg-surface-soft" style={{ padding: 'var(--eg-card-pad)' }}>
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">命中链路</h3>
                <span className="text-[11px] text-subtle">已写入请求日志 · 来源：调试台</span>
              </div>
              <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
                <MetaItem label="渠道" value={meta.channel_name || '—'} hint={meta.channel_id ? `#${meta.channel_id}` : ''} />
                <MetaItem label="上游模型" value={meta.upstream_model || '—'} />
                <MetaItem label="首 token" value={fmtMs(meta.first_token_ms || undefined)} />
                <MetaItem label="总耗时" value={fmtMs(meta.total_ms || undefined)} />
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5 text-[11px] text-muted">
                {meta.usage.usage_seen ? (
                  <>
                    <UsageChip label="输入" value={fmtInt(meta.usage.prompt_tokens)} />
                    <UsageChip label="输出" value={fmtInt(meta.usage.completion_tokens)} />
                    <UsageChip label="合计" value={fmtInt(meta.usage.total_tokens)} />
                    <UsageChip label="成本" value={fmtRequestCost(meta.cost_micros, meta.cost_status)} />
                    {meta.usage.cached_tokens > 0 && <UsageChip label="缓存" value={fmtInt(meta.usage.cached_tokens)} />}
                    {meta.usage.reasoning_tokens > 0 && <UsageChip label="推理" value={fmtInt(meta.usage.reasoning_tokens)} />}
                  </>
                ) : (
                  <span className="text-subtle">上游未返回 usage</span>
                )}
              </div>
              {/* 错误正文只在响应区底部的横幅展示一次；此处仅保留重试轨迹，避免同一段错误出现三遍 */}
              <div className="mt-4 border-t border-line pt-3">
                <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted">重试轨迹</h4>
                <RetryTimeline attempts={meta.retry_trace} />
              </div>
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}

function protocolLabel(protocol: 'chat' | 'responses' | 'anthropic' | 'gemini'): string {
  switch (protocol) {
    case 'responses': return 'Responses API'
    case 'anthropic': return 'Anthropic Messages'
    case 'gemini': return 'Gemini API'
    default: return 'Chat Completions'
  }
}

function UsageChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-md bg-surface px-2 py-1 ring-1 ring-inset ring-line">
      <span className="text-subtle">{label}</span>
      <span className="font-medium tabular-nums text-foreground-soft">{value}</span>
    </span>
  )
}

function MetaItem({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] text-subtle">{label}</div>
      <div className="break-all tabular-nums text-foreground-soft">
        {value}
        {hint && <span className="ml-1 text-xs text-subtle">{hint}</span>}
      </div>
    </div>
  )
}

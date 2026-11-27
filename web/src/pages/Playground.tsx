import { useEffect, useMemo, useRef, useState } from 'react'
import { Code2, FlaskConical, Play, Square, Trash2 } from 'lucide-react'
import { useChannels } from '../api/hooks'
import { runPlayground, type PlaygroundMeta, type PlaygroundTurn } from '../api/playground'
import RetryTimeline from '../components/RetryTimeline'
import { Alert, Badge, Button, Card, Disclosure, Empty, Field, IconButton, Input, PageHeader, SegmentedControl, Select, SettingRow, Spinner, Textarea } from '../components/ui'
import { describeUpstreamError, fmtInt, fmtMs, fmtRequestCost, httpStatusTone } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'

const MAX_PROMPT = 16000
const MAX_SYSTEM = 8000
/** 历史轮次上限与服务端 playgroundMaxTurns 一致 */
const MAX_TURNS = 40
type Protocol = 'chat' | 'responses' | 'anthropic' | 'gemini'
const PROTOCOLS: readonly { value: Protocol; label: string }[] = [
  { value: 'chat', label: 'Chat' },
  { value: 'responses', label: 'Responses' },
  { value: 'anthropic', label: 'Messages' },
  { value: 'gemini', label: 'Gemini' },
]

export default function Playground() {
  usePageTitle(t('page.playground.title'))
  const channels = useChannels()
  const [protocol, setProtocol] = useState<Protocol>('chat')
  const [model, setModel] = useState('')
  const [system, setSystem] = useState('')
  const [prompt, setPrompt] = useState('')
  const [stream, setStream] = useState(true)
  // 采样参数按字符串编辑，提交时转数字；留空 = 不下发（上游默认）
  const [temperature, setTemperature] = useState('')
  const [topP, setTopP] = useState('')
  const [maxTokens, setMaxTokens] = useState('')
  const [turns, setTurns] = useState<PlaygroundTurn[]>([])
  const [output, setOutput] = useState('')
  const [meta, setMeta] = useState<PlaygroundMeta | null>(null)
  const [error, setError] = useState('')
  const [running, setRunning] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const threadRef = useRef<HTMLDivElement>(null)

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
    const box = threadRef.current
    if (box) box.scrollTop = box.scrollHeight
  }, [output, turns])

  // 离开页面自动中止在途请求，避免后台继续消耗上游额度
  useEffect(() => () => abortRef.current?.abort(), [])

  function resetConversation() {
    setTurns([])
    setOutput('')
    setMeta(null)
    setError('')
  }

  function params(): { temperature?: number; top_p?: number; max_tokens?: number } {
    const num = (value: string) => (value.trim() === '' ? undefined : Number(value))
    return { temperature: num(temperature), top_p: num(topP), max_tokens: num(maxTokens) }
  }

  async function send() {
    if (running || !model || !prompt.trim()) return
    const userText = prompt.trim()
    const history = turns.slice(0, MAX_TURNS)
    const controller = new AbortController()
    abortRef.current = controller
    setRunning(true)
    setError('')
    setOutput('')
    setMeta(null)
    setTurns([...history, { role: 'user', content: userText }])
    setPrompt('')
    let acc = ''
    try {
      const result = await runPlayground(
        { protocol, model, system: system.trim(), history, prompt: userText, stream, ...params() },
        {
          onDelta: (delta) => { acc += delta; setOutput(acc) },
          signal: controller.signal,
        },
      )
      setMeta(result.meta)
      const text = result.text || acc
      setTurns((current) => [...current, { role: 'assistant', content: text }])
      setOutput('')
      if (result.streamError) {
        setError(result.streamError)
      } else if (result.meta && result.meta.status >= 400) {
        setError(result.meta.error_message || tf('playground.upstreamStatus', { status: result.meta.status }))
      } else if (!text && result.meta?.status === 200) {
        setError(t('playground.noText'))
      }
    } catch (err) {
      // 中断 / 失败时保留已生成的部分，作为一轮 assistant 消息留在对话里
      if (acc) setTurns((current) => [...current, { role: 'assistant', content: acc }])
      setOutput('')
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

  const showEmpty = turns.length === 0 && !output && !error

  return (
    <div className="eg-page-stack">
      <PageHeader
        title={<span className="flex items-center gap-2"><FlaskConical size={18} /> {t('page.playground.title')}</span>}
        description={t('page.playground.desc')}
        actions={<Badge tone="brand">{t('playground.adminOnly')}</Badge>}
      />

      <div className="eg-grid lg:grid-cols-[clamp(280px,24vw,420px)_minmax(0,1fr)]">
        <Card className="min-w-0 self-start">
          <div className="space-y-[var(--eg-control-gap)]">
            <div>
              <div className="mb-1.5 text-sm font-medium text-foreground-soft">{t('playground.protocol')}</div>
              <SegmentedControl
                label={t('playground.protocol')}
                value={protocol}
                onChange={setProtocol}
                options={PROTOCOLS}
                disabled={running}
                className="grid-cols-2 sm:grid-cols-4 lg:grid-cols-2"
              />
            </div>

            <Field label={t('playground.model')} hint={models.length === 0 ? t('playground.noModelsHint') : undefined}>
              <Select
                value={model}
                onValueChange={setModel}
                disabled={running || models.length === 0}
                options={models.length === 0 ? [{ value: '', label: t('playground.noModels') }] : models.map((item) => ({ value: item, label: item }))}
              />
            </Field>

            <div className="space-y-1.5">
              <div className="flex items-baseline justify-between">
                <span className="text-sm font-medium text-foreground-soft">{t('playground.system')} <span className="font-normal text-subtle">{t('common.optional')}</span></span>
                <span className="text-[11px] tabular-nums text-subtle">{system.length}/{MAX_SYSTEM}</span>
              </div>
              <Textarea
                rows={3}
                value={system}
                maxLength={MAX_SYSTEM}
                onChange={(e) => setSystem(e.target.value)}
                placeholder={t('playground.systemPlaceholder')}
                disabled={running}
                aria-label={t('playground.system')}
              />
            </div>

            <Disclosure title={t('playground.params')} defaultOpen={false}>
              <div className="eg-grid eg-grid-auto-wide">
                <ParamField label={t('playground.temperature')} hint="0 – 2" value={temperature} onChange={setTemperature} disabled={running} />
                <ParamField label={t('playground.topP')} hint="0 – 1" value={topP} onChange={setTopP} disabled={running} />
                <ParamField label={t('playground.maxTokens')} hint="1 – 32768" value={maxTokens} onChange={setMaxTokens} disabled={running} />
              </div>
            </Disclosure>

            <SettingRow
              title={t('playground.stream')}
              description={t('playground.streamHint')}
              checked={stream}
              onChange={setStream}
              disabled={running}
              label={t('playground.stream')}
            />

            <div className="space-y-1.5">
              <div className="flex items-baseline justify-between">
                <span className="text-sm font-medium text-foreground-soft">{t('playground.user')}</span>
                <span className="text-[11px] tabular-nums text-subtle">{prompt.length}/{MAX_PROMPT}</span>
              </div>
              <Textarea
                rows={5}
                value={prompt}
                maxLength={MAX_PROMPT}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder={t('playground.userPlaceholder')}
                disabled={running}
                aria-label={t('playground.user')}
                onKeyDown={(e) => {
                  // Ctrl/⌘ + Enter 发送，Enter 保持换行（多行提示词更常见）
                  if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
                    e.preventDefault()
                    void send()
                  }
                }}
              />
            </div>

            <div className="flex gap-2 pt-0.5">
              <Button onClick={() => void send()} loading={running} disabled={!model || !prompt.trim()} className="flex-1">
                <Play size={14} />
                {t('common.send')}
              </Button>
              <Button variant="outline" onClick={stop} disabled={!running} aria-label={t('playground.stopAria')}>
                <Square size={13} /> {t('common.stop')}
              </Button>
            </div>
          </div>
        </Card>

        <Card flush className="flex min-w-0 flex-col overflow-hidden">
          <div className="flex flex-wrap items-center gap-2 border-b border-line-soft" style={{ padding: 'var(--eg-cell-y) var(--eg-card-pad)' }}>
            <h2 className="text-sm font-semibold text-foreground-soft">{t('playground.conversation')}</h2>
            <Badge tone="neutral">{protocolLabel(protocol)}</Badge>
            {meta && (
              <div className="ml-auto flex items-center gap-2">
                <Badge tone={httpStatusTone(meta.status)} className="font-semibold">HTTP {meta.status}</Badge>
                <span className="text-[11px] text-subtle">{tf('playground.attempts', { n: meta.retry_trace.length })}</span>
              </div>
            )}
            <IconButton
              label={t('playground.reset')}
              side="top"
              variant="outline"
              disabled={running || turns.length === 0}
              onClick={resetConversation}
            >
              <Trash2 size={13} />
            </IconButton>
          </div>

          <div ref={threadRef} aria-live="polite" className="max-h-[52vh] min-h-[160px] overflow-auto" style={{ padding: 'var(--eg-card-pad)' }}>
            {error && (
              <Alert tone="danger" className="mb-3" data-upstream="true">
                <span className="break-words" title={error}>{describeUpstreamError(error).text}</span>
              </Alert>
            )}
            {turns.map((turn, index) => (
              <TurnBubble key={index} role={turn.role} content={turn.content} />
            ))}
            {output && <TurnBubble role="assistant" content={output} streaming />}
            {!output && running && (
              <div className="flex items-center gap-2 py-6 text-sm text-muted-foreground"><Spinner /> {t('playground.waiting')}</div>
            )}
            {showEmpty && (
              <div className="py-6">
                <Empty icon={<FlaskConical size={34} />} title={t('empty.playground.title')} hint={t('empty.playground.hint')} />
              </div>
            )}
          </div>

          {meta && (
            <div className="border-t border-line-soft bg-surface-soft" style={{ padding: 'var(--eg-card-pad)' }}>
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{t('playground.trace')}</h3>
                <span className="text-[11px] text-subtle">{t('playground.loggedFrom')}</span>
              </div>
              <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
                <MetaItem label={t('table.channel')} value={meta.channel_name || '—'} hint={meta.channel_id ? `#${meta.channel_id}` : ''} />
                <MetaItem label={t('table.upstreamModel')} value={meta.upstream_model || '—'} />
                <MetaItem label={t('table.firstToken')} value={fmtMs(meta.first_token_ms || undefined)} />
                <MetaItem label={t('table.totalMs')} value={fmtMs(meta.total_ms || undefined)} />
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5 text-[11px] text-muted-foreground">
                {meta.usage.usage_seen ? (
                  <>
                    <UsageChip label={t('usage.input')} value={fmtInt(meta.usage.prompt_tokens)} />
                    <UsageChip label={t('usage.output')} value={fmtInt(meta.usage.completion_tokens)} />
                    <UsageChip label={t('usage.total')} value={fmtInt(meta.usage.total_tokens)} />
                    <UsageChip label={t('table.cost')} value={fmtRequestCost(meta.cost_micros, meta.cost_status)} />
                    {meta.usage.cached_tokens > 0 && <UsageChip label={t('usage.cached')} value={fmtInt(meta.usage.cached_tokens)} />}
                    {meta.usage.reasoning_tokens > 0 && <UsageChip label={t('usage.reasoning')} value={fmtInt(meta.usage.reasoning_tokens)} />}
                  </>
                ) : (
                  <span className="text-subtle">{t('playground.noUsage')}</span>
                )}
              </div>
              {meta.upstream_request && (
                <Disclosure
                  title={<span className="flex items-center gap-1.5"><Code2 size={13} /> {t('playground.upstreamRequest')}</span>}
                  badge={meta.upstream_request.truncated ? <Badge tone="warning">{t('playground.upstreamTruncated')}</Badge> : undefined}
                  defaultOpen={false}
                  className="mt-4 border-t border-line pt-3"
                >
                  <div className="mb-2 break-all font-mono text-[11px] text-subtle">
                    <span className="font-semibold text-foreground-soft">{meta.upstream_request.method}</span> {meta.upstream_request.url}
                  </div>
                  <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-lg border border-line bg-surface p-3 font-mono text-[11px] leading-5 text-foreground-soft">
                    {prettyJSON(meta.upstream_request.body)}
                  </pre>
                </Disclosure>
              )}
              <div className="mt-4 border-t border-line pt-3">
                <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{t('logs.trace.title')}</h4>
                <RetryTimeline attempts={meta.retry_trace} />
              </div>
            </div>
          )}
        </Card>
      </div>
    </div>
  )
}

/** 对话轮次：左侧角色标签 + 正文；assistant 流式输出时末尾带闪烁光标。 */
function TurnBubble({ role, content, streaming }: { role: PlaygroundTurn['role']; content: string; streaming?: boolean }) {
  const assistant = role === 'assistant'
  return (
    <div className={`mb-4 flex gap-3 ${assistant ? '' : 'flex-row-reverse'}`}>
      <span className={`mt-0.5 inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-[10px] font-semibold ${assistant ? 'bg-brand/10 text-brand' : 'bg-surface-soft text-muted-foreground ring-1 ring-inset ring-line'}`}>
        {assistant ? 'A' : 'U'}
      </span>
      <div className={`min-w-0 max-w-[88%] rounded-xl px-3.5 py-2.5 ${assistant ? 'bg-surface-soft' : 'bg-brand/10'}`}>
        <div className="mb-1 text-[11px] font-medium text-subtle">{assistant ? t('playground.assistant') : t('playground.user')}</div>
        <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-foreground-soft">
          {content}
          {streaming && <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-brand align-middle" />}
        </pre>
      </div>
    </div>
  )
}

function ParamField({ label, hint, value, onChange, disabled }: { label: string; hint: string; value: string; onChange: (value: string) => void; disabled: boolean }) {
  return (
    <Field label={label} hint={hint}>
      <Input type="number" inputMode="decimal" value={value} disabled={disabled} onChange={(e) => onChange(e.target.value)} aria-label={label} />
    </Field>
  )
}

/** 上游请求体尽量按 JSON 缩进展示；截断后的残缺 JSON 原样显示。 */
function prettyJSON(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

function protocolLabel(protocol: Protocol): string {
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

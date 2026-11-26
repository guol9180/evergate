import { useEffect, useMemo, useRef, useState } from 'react'
import { FlaskConical, Play, Square } from 'lucide-react'
import { useChannels } from '../api/hooks'
import { runPlayground, type PlaygroundMeta } from '../api/playground'
import RetryTimeline from '../components/RetryTimeline'
import { Alert, Badge, Button, Card, Empty, Field, PageHeader, SegmentedControl, Select, SettingRow, Spinner, Textarea } from '../components/ui'
import { describeUpstreamError, fmtInt, fmtMs, fmtRequestCost, httpStatusTone } from '../lib/format'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'

const MAX_PROMPT = 16000
const MAX_SYSTEM = 8000
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
        setError(result.meta.error_message || tf('playground.upstreamStatus', { status: result.meta.status }))
      } else if (!result.text && result.meta?.status === 200) {
        setError(t('playground.noText'))
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

  // 非流式失败时服务端把错误放进 response.error，extractPlaygroundText 会把它当正文返回；顶部已有横幅，正文区不再重复
  const outputIsErrorOnly = !!error && output === tf('playground.upstreamError', { message: describeUpstreamError(error).text })

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

            <div className="space-y-1.5">
              <div className="flex items-baseline justify-between">
              <span className="text-sm font-medium text-foreground-soft">{t('playground.user')}</span>
                <span className="text-[11px] tabular-nums text-subtle">{prompt.length}/{MAX_PROMPT}</span>
              </div>
              <Textarea
                rows={7}
                value={prompt}
                maxLength={MAX_PROMPT}
                onChange={(e) => setPrompt(e.target.value)}
              placeholder={t('playground.userPlaceholder')}
                disabled={running}
                aria-label={t('playground.user')}
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
              <Button onClick={send} loading={running} disabled={!model || !prompt.trim()} className="flex-1">
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
            <h2 className="text-sm font-semibold text-foreground-soft">{t('playground.response')}</h2>
            <Badge tone="neutral">{protocolLabel(protocol)}</Badge>
            {meta && (
              <div className="ml-auto flex items-center gap-2">
                <Badge tone={httpStatusTone(meta.status)} className="font-semibold">HTTP {meta.status}</Badge>
                <span className="text-[11px] text-subtle">{tf('playground.attempts', { n: meta.retry_trace.length })}</span>
              </div>
            )}
          </div>

          <div ref={outputRef} aria-live="polite" className="max-h-[58vh] min-h-[120px] overflow-auto" style={{ padding: 'var(--eg-card-pad)' }}>
            {/* 错误放在响应区顶部、正文之前：先看到"为什么失败"再看已生成的部分输出，不再贴卡片底边 */}
            {error && (
              <Alert tone="danger" className="mb-3" data-upstream="true">
                <span className="break-words" title={error}>{describeUpstreamError(error).text}</span>
              </Alert>
            )}
            {output && !outputIsErrorOnly ? (
              <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-foreground-soft">
                {output}
                {running && <span className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-brand align-middle" />}
              </pre>
            ) : running ? (
              <div className="flex items-center gap-2 py-12 text-sm text-muted"><Spinner /> {t('playground.waiting')}</div>
            ) : error ? (
              // 纯错误响应没有正文可看：顶部横幅已经说清楚，不再在正文区重复一遍
              null
            ) : (
              <Empty icon={<FlaskConical size={34} />} title={t('empty.playground.title')} hint={t('empty.playground.hint')} />
            )}
          </div>

          {meta && (
            <div className="border-t border-line-soft bg-surface-soft" style={{ padding: 'var(--eg-card-pad)' }}>
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-semibold uppercase tracking-wide text-muted">{t('playground.trace')}</h3>
                <span className="text-[11px] text-subtle">{t('playground.loggedFrom')}</span>
              </div>
              <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
              <MetaItem label={t('table.channel')} value={meta.channel_name || '—'} hint={meta.channel_id ? `#${meta.channel_id}` : ''} />
              <MetaItem label={t('table.upstreamModel')} value={meta.upstream_model || '—'} />
              <MetaItem label={t('table.firstToken')} value={fmtMs(meta.first_token_ms || undefined)} />
              <MetaItem label={t('table.totalMs')} value={fmtMs(meta.total_ms || undefined)} />
              </div>
              <div className="mt-3 flex flex-wrap gap-1.5 text-[11px] text-muted">
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
              {/* 错误正文只在响应区底部的横幅展示一次；此处仅保留重试轨迹，避免同一段错误出现三遍 */}
              <div className="mt-4 border-t border-line pt-3">
                <h4 className="mb-3 text-xs font-semibold uppercase tracking-wide text-muted">{t('logs.trace.title')}</h4>
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

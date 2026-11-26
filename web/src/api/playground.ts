import type { RetryAttempt } from './types'
import { extractPlaygroundDelta, extractPlaygroundText, type PlaygroundPayload } from '../lib/playgroundProtocol'
import { withBasePath } from '../lib/runtime'
import { t } from '../lib/i18n'

export interface PlaygroundInput {
  protocol: 'chat' | 'responses' | 'anthropic' | 'gemini'
  model: string
  system: string
  prompt: string
  stream: boolean
}

export interface PlaygroundUsage {
  prompt_tokens: number
  completion_tokens: number
  total_tokens: number
  cached_tokens: number
  reasoning_tokens: number
  usage_seen: boolean
}

export interface PlaygroundMeta {
  protocol: 'chat' | 'responses' | 'anthropic' | 'gemini'
  model: string
  status: number
  channel_id: number
  channel_name: string
  upstream_model: string
  first_token_ms: number
  total_ms: number
  usage: PlaygroundUsage
  cost_micros: number
  cost: number
  cost_status: string
  retry_trace: RetryAttempt[]
  error_type: string
  error_message: string
}

export interface PlaygroundResult {
  text: string
  meta: PlaygroundMeta | null
  streamError: string
}

interface StreamHandlers {
  onDelta: (delta: string) => void
  signal?: AbortSignal
}

const PLAYGROUND_SESSION_ID = createPlaygroundSessionID()

// runPlayground 直接调用管理台 Playground 端点：SSE 实时回调，非流式一次性返回。
export async function runPlayground(input: PlaygroundInput, { onDelta, signal }: StreamHandlers): Promise<PlaygroundResult> {
  const resp = await fetch(withBasePath('/admin/api/playground'), {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json', 'X-Session-Id': PLAYGROUND_SESSION_ID },
    body: JSON.stringify(input),
    signal,
  })
  const contentType = resp.headers.get('content-type') || ''
  if (!resp.ok) {
    const payload = await safeJSON(resp)
    throw new Error(payload?.message || `HTTP ${resp.status}`)
  }
  if (contentType.includes('text/event-stream')) {
    return readSSE(resp, onDelta)
  }
  const payload = await resp.json()
  if (payload?.ok === false) throw new Error(payload.message || t('playground.requestFailed'))
  const response = payload?.data?.response
  const text = extractPlaygroundText(response)
  if (text) onDelta(text)
  return { text, meta: payload?.data?.meta ?? null, streamError: '' }
}

async function safeJSON(resp: Response): Promise<{ message?: string } | null> {
  try {
    return await resp.json()
  } catch {
    return null
  }
}

async function readSSE(resp: Response, onDelta: (delta: string) => void): Promise<PlaygroundResult> {
  if (!resp.body) throw new Error(t('playground.noStream'))
  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let text = ''
  let meta: PlaygroundMeta | null = null
  let streamError = ''

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    buffer = buffer.replace(/\r\n/g, '\n')
    let boundary = buffer.indexOf('\n\n')
    while (boundary >= 0) {
      const block = buffer.slice(0, boundary)
      buffer = buffer.slice(boundary + 2)
      const { event, data } = parseSSEBlock(block)
      if (data) {
        if (event === 'evergate.meta') {
          try {
            meta = JSON.parse(data) as PlaygroundMeta
          } catch {
      streamError = t('playground.metaFailed')
          }
        } else if (data.trim() !== '[DONE]') {
          let chunk: PlaygroundPayload | null = null
          try {
            chunk = JSON.parse(data)
          } catch {
            chunk = null
          }
          if (chunk?.error?.message) streamError = String(chunk.error.message)
          const delta = extractPlaygroundDelta(chunk)
          if (delta) {
            text += delta
            onDelta(delta)
          }
        }
      }
      boundary = buffer.indexOf('\n\n')
    }
  }
  return { text, meta, streamError }
}

function createPlaygroundSessionID(): string {
  const uuid = globalThis.crypto?.randomUUID?.()
  if (uuid) return `playground-${uuid}`
  return `playground-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function parseSSEBlock(block: string): { event: string; data: string } {
  let event = ''
  const data: string[] = []
  for (const line of block.split('\n')) {
    if (line.startsWith('event:')) {
      event = line.slice(6).trim()
    } else if (line.startsWith('data:')) {
      data.push(line.slice(5).trimStart())
    }
  }
  return { event, data: data.join('\n') }
}

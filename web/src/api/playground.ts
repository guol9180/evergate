import type { RetryAttempt } from './types'

export interface PlaygroundInput {
  protocol: 'chat' | 'responses'
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
  protocol: 'chat' | 'responses'
  model: string
  status: number
  channel_id: number
  channel_name: string
  upstream_model: string
  first_token_ms: number
  total_ms: number
  usage: PlaygroundUsage
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

// runPlayground 直接调用管理台 Playground 端点：SSE 实时回调，非流式一次性返回。
export async function runPlayground(input: PlaygroundInput, { onDelta, signal }: StreamHandlers): Promise<PlaygroundResult> {
  const resp = await fetch('/admin/api/playground', {
    method: 'POST',
    credentials: 'same-origin',
    headers: { 'Content-Type': 'application/json' },
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
  if (payload?.ok === false) throw new Error(payload.message || 'Playground 请求失败')
  const response = payload?.data?.response
  const text = extractText(response)
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
  if (!resp.body) throw new Error('当前浏览器不支持流式读取')
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
            streamError = 'Playground 元数据解析失败'
          }
        } else if (data.trim() !== '[DONE]') {
          let chunk: any
          try {
            chunk = JSON.parse(data)
          } catch {
            chunk = null
          }
          if (chunk?.error?.message) streamError = String(chunk.error.message)
          const delta = extractDelta(chunk)
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

function extractDelta(chunk: any): string {
  if (!chunk) return ''
  if (chunk.type === 'response.output_text.delta' && typeof chunk.delta === 'string') return chunk.delta
  const content = chunk.choices?.[0]?.delta?.content
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content.map((part: any) => (typeof part?.text === 'string' ? part.text : '')).join('')
  }
  return ''
}

function extractText(response: any): string {
  if (response == null) return ''
  if (typeof response === 'string') return response
  const chatContent = response.choices?.[0]?.message?.content
  if (typeof chatContent === 'string') return chatContent
  if (Array.isArray(response.output)) {
    const parts: string[] = []
    for (const item of response.output) {
      for (const part of item?.content ?? []) {
        if (typeof part?.text === 'string') parts.push(part.text)
      }
    }
    if (parts.length > 0) return parts.join('')
  }
  if (response.error?.message) return `上游错误：${response.error.message}`
  return JSON.stringify(response, null, 2)
}

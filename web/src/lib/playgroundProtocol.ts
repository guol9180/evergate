// 带 .ts 后缀：node --experimental-strip-types 跑单测时不做扩展名解析
import { tf } from './i18n.ts'

type TextPart = { text?: unknown }

/** 四种协议流式 chunk / 非流式响应的最小结构；只描述本模块读取的字段。 */
export type PlaygroundPayload = {
  type?: string
  delta?: string | { type?: string; text?: unknown }
  choices?: Array<{ delta?: { content?: string | TextPart[] }; message?: { content?: string | TextPart[] } }>
  candidates?: Array<{ content?: { parts?: TextPart[] } }>
  output?: Array<{ content?: TextPart[] }>
  content?: string | TextPart[]
  error?: { message?: unknown }
}

export function extractPlaygroundDelta(chunk: PlaygroundPayload | null | undefined): string {
  if (!chunk) return ''

  if (chunk.type === 'response.output_text.delta' && typeof chunk.delta === 'string') {
    return chunk.delta
  }

  const blockDelta = chunk.delta
  if (chunk.type === 'content_block_delta' && blockDelta && typeof blockDelta === 'object' &&
    blockDelta.type === 'text_delta' && typeof blockDelta.text === 'string') {
    return blockDelta.text
  }

  const chatContent = chunk.choices?.[0]?.delta?.content
  if (typeof chatContent === 'string') return chatContent
  if (Array.isArray(chatContent)) {
    return chatContent.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('')
  }

  const parts = chunk.candidates?.[0]?.content?.parts
  if (Array.isArray(parts)) {
    return parts.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('')
  }
  return ''
}

export function extractPlaygroundText(response: PlaygroundPayload | string | null | undefined): string {
  if (response == null) return ''
  if (typeof response === 'string') return response

  const chatContent = response.choices?.[0]?.message?.content
  if (typeof chatContent === 'string') return chatContent
  if (Array.isArray(chatContent)) {
    const text = chatContent.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('')
    if (text) return text
  }

  if (Array.isArray(response.output)) {
    const parts: string[] = []
    for (const item of response.output) {
      for (const part of item?.content ?? []) {
        if (typeof part?.text === 'string') parts.push(part.text)
      }
    }
    if (parts.length > 0) return parts.join('')
  }

  if (Array.isArray(response.content)) {
    const text = response.content.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('')
    if (text) return text
  }

  const geminiParts = response.candidates?.[0]?.content?.parts
  if (Array.isArray(geminiParts)) {
    const text = geminiParts.map((part) => (typeof part?.text === 'string' ? part.text : '')).join('')
    if (text) return text
  }

  if (response.error?.message) return tf('playground.upstreamError', { message: String(response.error.message) })
  return JSON.stringify(response, null, 2)
}

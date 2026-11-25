// 带 .ts 后缀：node --experimental-strip-types 跑单测时不做扩展名解析
import { tf } from './i18n.ts'

export function extractPlaygroundDelta(chunk: any): string {
  if (!chunk) return ''

  if (chunk.type === 'response.output_text.delta' && typeof chunk.delta === 'string') {
    return chunk.delta
  }

  if (chunk.type === 'content_block_delta' && chunk.delta?.type === 'text_delta' && typeof chunk.delta.text === 'string') {
    return chunk.delta.text
  }

  const chatContent = chunk.choices?.[0]?.delta?.content
  if (typeof chatContent === 'string') return chatContent
  if (Array.isArray(chatContent)) {
    return chatContent.map((part: any) => (typeof part?.text === 'string' ? part.text : '')).join('')
  }

  const parts = chunk.candidates?.[0]?.content?.parts
  if (Array.isArray(parts)) {
    return parts.map((part: any) => (typeof part?.text === 'string' ? part.text : '')).join('')
  }
  return ''
}

export function extractPlaygroundText(response: any): string {
  if (response == null) return ''
  if (typeof response === 'string') return response

  const chatContent = response.choices?.[0]?.message?.content
  if (typeof chatContent === 'string') return chatContent
  if (Array.isArray(chatContent)) {
    const text = chatContent.map((part: any) => (typeof part?.text === 'string' ? part.text : '')).join('')
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
    const text = response.content.map((part: any) => (typeof part?.text === 'string' ? part.text : '')).join('')
    if (text) return text
  }

  const geminiParts = response.candidates?.[0]?.content?.parts
  if (Array.isArray(geminiParts)) {
    const text = geminiParts.map((part: any) => (typeof part?.text === 'string' ? part.text : '')).join('')
    if (text) return text
  }

  if (response.error?.message) return tf('playground.upstreamError', { message: response.error.message })
  return JSON.stringify(response, null, 2)
}

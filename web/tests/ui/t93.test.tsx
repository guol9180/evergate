import { describe, expect, it } from 'vitest'
import { modelListSnippets } from '../../src/lib/clipboard'

describe('modelListSnippets', () => {
  it('covers the OpenAI, Anthropic and Gemini model-list endpoints', () => {
    const snippet = modelListSnippets('http://gw.test/v1')
    expect(snippet).toContain('curl "$EVERGATE_BASE_URL/models" -H "Authorization: Bearer $EVERGATE_API_KEY"')
    expect(snippet).toContain('curl "$EVERGATE_BASE_URL/models" -H "x-api-key: $EVERGATE_API_KEY" -H "anthropic-version: 2023-06-01"')
    expect(snippet).toContain('curl "http://gw.test/v1beta/models" -H "x-goog-api-key: $EVERGATE_API_KEY"')
  })
})

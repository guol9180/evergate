import assert from 'node:assert/strict'
import test from 'node:test'
import { extractPlaygroundDelta, extractPlaygroundText } from '../src/lib/playgroundProtocol.ts'

test('extract streaming deltas for all four protocols', () => {
  assert.equal(extractPlaygroundDelta({ choices: [{ delta: { content: 'chat' } }] }), 'chat')
  assert.equal(extractPlaygroundDelta({ type: 'response.output_text.delta', delta: 'responses' }), 'responses')
  assert.equal(extractPlaygroundDelta({
    type: 'content_block_delta',
    delta: { type: 'text_delta', text: 'messages' },
  }), 'messages')
  assert.equal(extractPlaygroundDelta({
    candidates: [{ content: { parts: [{ text: 'gem' }, { text: 'ini' }] } }],
  }), 'gemini')
})

test('extract non-stream text for all four protocols', () => {
  assert.equal(extractPlaygroundText({ choices: [{ message: { content: 'chat' } }] }), 'chat')
  assert.equal(extractPlaygroundText({
    output: [{ content: [{ type: 'output_text', text: 'responses' }] }],
  }), 'responses')
  assert.equal(extractPlaygroundText({
    content: [{ type: 'text', text: 'messages' }],
  }), 'messages')
  assert.equal(extractPlaygroundText({
    candidates: [{ content: { parts: [{ text: 'gem' }, { text: 'ini' }] } }],
  }), 'gemini')
})

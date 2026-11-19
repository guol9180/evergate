import assert from 'node:assert/strict'
import test from 'node:test'
import { CHANNEL_PRESETS, findChannelPreset } from '../src/lib/channelPresets.ts'

test('OpenCode Go preset fills the official OpenAI-compatible endpoint', () => {
  const preset = findChannelPreset('opencode-go')
  assert.ok(preset)
  assert.equal(preset.baseUrl, 'https://opencode.ai/inference/go/openai/v1')
  assert.equal(preset.type, 'openai-compatible')
  assert.equal(preset.supportsResponses, true)
  assert.equal(preset.supportsAnthropic, false)
  assert.match(preset.modelsPlaceholder, /glm-5\.3-flash/)
})

test('channel preset ids stay unique', () => {
  const ids = CHANNEL_PRESETS.map((preset) => preset.id)
  assert.equal(new Set(ids).size, ids.length)
})

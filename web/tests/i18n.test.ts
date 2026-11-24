import assert from 'node:assert/strict'
import test from 'node:test'
import { dictionaryDiff, dict, getLocale, setLocale, t } from '../src/lib/i18n.ts'

test('zh-CN and en dictionaries share the same key set', () => {
  const diff = dictionaryDiff()
  assert.deepEqual(diff.missingInEn, [])
  assert.deepEqual(diff.missingInZh, [])
  for (const key of Object.keys(dict['zh-CN'])) {
    assert.ok((dict['zh-CN'] as Record<string, string>)[key].length > 0, `zh-CN.${key} is empty`)
    assert.ok((dict.en as Record<string, string>)[key].length > 0, `en.${key} is empty`)
  }
})

test('t() follows the active locale and falls back to zh-CN', () => {
  setLocale('en')
  assert.equal(getLocale(), 'en')
  assert.equal(t('nav.channels'), 'Channels')
  setLocale('zh-CN')
  assert.equal(t('nav.channels'), '渠道管理')
})

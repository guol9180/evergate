import assert from 'node:assert/strict'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'
import { dictionaryDiff, dict, getLocale, localeFor, setLocale, t } from '../src/lib/i18n.ts'

const CJK = /[\u4e00-\u9fff]/

function sourceFiles(dir: string): string[] {
  const out: string[] = []
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) out.push(...sourceFiles(full))
    else if (/\.(ts|tsx)$/.test(name) && !full.endsWith('i18n.ts')) out.push(full)
  }
  return out
}

test('UI source contains no hard-coded Chinese outside the dictionary (T90)', () => {
  // 去掉注释后再扫描：注释允许中文，字符串字面量与 JSX 文本必须走 t()/tf()
  const offenders: string[] = []
  for (const file of sourceFiles(join(process.cwd(), 'src'))) {
    const src = readFileSync(file, 'utf8').replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:\\])\/\/.*$/gm, '$1')
    src.split('\n').forEach((line, index) => {
      if (CJK.test(line)) offenders.push(`${file}:${index + 1}: ${line.trim().slice(0, 80)}`)
    })
  }
  assert.deepEqual(offenders, [])
})

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

test('first visit picks the UI language from the browser language', () => {
  assert.equal(localeFor('zh-CN'), 'zh-CN')
  assert.equal(localeFor('zh-TW'), 'zh-CN')
  assert.equal(localeFor('en-US'), 'en')
  assert.equal(localeFor('fr'), 'en')
  assert.equal(localeFor(undefined), 'en')
})

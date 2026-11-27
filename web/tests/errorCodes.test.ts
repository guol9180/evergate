import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import test from 'node:test'
import { dict } from '../src/lib/i18n.ts'

const root = join(process.cwd(), '..')

/** Go 侧出现的全部稳定 code：admin 的 fail(c, …, "code", …)、FailCode(...) 与 importFailure 字面量。 */
function goCodes(): Set<string> {
  const codes = new Set<string>()
  const files = [
    ...readdirSync(join(root, 'internal/admin')).filter((name) => name.endsWith('.go') && !name.endsWith('_test.go')).map((name) => join(root, 'internal/admin', name)),
    join(root, 'internal/relay/playground.go'),
  ]
  const patterns = [/\bfail\(c, [^,]+, "([a-z_]+)", /g, /FailCode\(c, [^,]+, "([a-z_]+)", /g, /code: "([a-z_]+)"/g]
  for (const file of files) {
    const src = readFileSync(file, 'utf8')
    for (const pattern of patterns) for (const match of src.matchAll(pattern)) codes.add(match[1])
  }
  return codes
}

test('every backend error code has zh and en copy and a frontend mapping (T109 ③)', () => {
  const errorsSrc = readFileSync(join(process.cwd(), 'src/lib/errors.ts'), 'utf8')
  const mapped = new Set([...errorsSrc.matchAll(/^ {2}([a-z_]+): 'error\.[a-z_]+',$/gm)].map((match) => match[1]))
  const zh = dict['zh-CN'] as Record<string, string>
  const en = dict.en as Record<string, string>
  const codes = goCodes()
  assert.ok(codes.size > 50, `expected the full code list, got ${codes.size}`)
  for (const code of codes) {
    assert.ok(zh[`error.${code}`], `zh-CN missing error.${code}`)
    assert.ok(en[`error.${code}`], `en missing error.${code}`)
    assert.ok(mapped.has(code), `errors.ts does not map ${code}`)
  }
  for (const code of mapped) assert.ok(codes.has(code), `errors.ts maps unknown code ${code}`)
})

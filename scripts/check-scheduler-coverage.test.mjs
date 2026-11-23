import assert from 'node:assert/strict'
import test from 'node:test'
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const script = fileURLToPath(new URL('./check-scheduler-coverage.mjs', import.meta.url))

test('coverage gate accepts and rejects the statement threshold', () => {
  const dir = mkdtempSync(join(tmpdir(), 'evergate-coverage-'))
  try {
    const profile = join(dir, 'cover.txt')
    for (const [coverage, code] of [['89.9', 1], ['90.0', 0]]) {
      writeFileSync(profile, `total:\t(statements)\t${coverage}%\n`)
      const result = spawnSync(process.execPath, [script, profile], { encoding: 'utf8' })
      assert.equal(result.status, code, result.stderr)
    }
  } finally {
    rmSync(dir, { recursive: true, force: true })
  }
})

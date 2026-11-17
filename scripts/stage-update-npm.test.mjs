import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { stageUpdatePackage, updateAssetNames } from './stage-update-npm.mjs'

test('stage update npm package requires platform assets and accepts optional macOS dmg', async () => {
  const root = mkdtempSync(path.join(os.tmpdir(), 'evergate-update-npm-'))
  try {
    const version = '0.4.0'
    const names = updateAssetNames(version)
    const release = path.join(root, 'release')
    mkdirSync(release)
    for (const name of [names.windows, names.linux, names.checksums]) {
      writeFileSync(path.join(release, name), name)
    }
    const output = path.join(root, 'staged')
    const result = await stageUpdatePackage(version, release, output)
    assert.deepEqual(result.artifacts, [names.windows, names.linux, names.checksums])
    const pkg = JSON.parse(readFileSync(path.join(output, 'package.json'), 'utf8'))
    assert.equal(pkg.name, '@heguoliang/evergate-updates')
    assert.equal(pkg.version, version)

    writeFileSync(path.join(release, names.macOS), 'dmg')
    const withMac = await stageUpdatePackage(version, release, output)
    assert.ok(withMac.artifacts.includes(names.macOS))
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

test('stage update npm package rejects missing required assets', async () => {
  const root = mkdtempSync(path.join(os.tmpdir(), 'evergate-update-npm-'))
  try {
    await assert.rejects(
      () => stageUpdatePackage('0.4.0', path.join(root, 'missing'), path.join(root, 'out')),
      /ENOENT/,
    )
  } finally {
    rmSync(root, { recursive: true, force: true })
  }
})

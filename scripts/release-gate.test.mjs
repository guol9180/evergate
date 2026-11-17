import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import test from 'node:test'
import {
  PLATFORM_PACKAGES,
  WRAPPER_PACKAGE,
  extractChangelogSection,
  parseReleaseTag,
  releaseAssetNames,
  runCli,
  validatePublicAssets,
  validateStagedPackages,
} from './release-gate.mjs'

test('release tags select stable or prerelease npm channels', () => {
  assert.deepEqual(parseReleaseTag('v0.2.0'), {
    tag: 'v0.2.0',
    version: '0.2.0',
    prerelease: false,
    channel: 'stable',
    npmTag: 'latest',
  })
  assert.equal(parseReleaseTag('v0.2.0-rc.3').npmTag, 'next')
  assert.equal(parseReleaseTag('v0.2.0-beta.12').prerelease, true)
  assert.throws(() => parseReleaseTag('0.2.0'), /invalid release tag/)
  assert.throws(() => parseReleaseTag('v0.2.0-preview.1'), /invalid release tag/)
  assert.throws(() => parseReleaseTag('v0.2.0-rc.0'), /invalid release tag/)
})

test('changelog gate requires one exact non-empty section', () => {
  const markdown = '# Changes\r\n\r\n## v0.2.0-rc.2\r\n\r\n- fixed\r\n\r\n## v0.2.0-rc.1\r\n\r\n- older\r\n'
  assert.equal(extractChangelogSection(markdown, 'v0.2.0-rc.2'), '- fixed\n')
  assert.throws(() => extractChangelogSection(markdown, 'v0.2.0'), /exactly one exact header/)
  const duplicate = `${markdown}\n## v0.2.0-rc.1\n- duplicate\n`
  assert.throws(
    () => extractChangelogSection(duplicate, 'v0.2.0-rc.1'),
    /exactly one exact header/,
  )
  assert.throws(() => extractChangelogSection('## v0.2.0\n\n## v0.1.0\n- old', 'v0.2.0'), /is empty/)
  assert.throws(() => extractChangelogSection('## v0.2.0 — notes\n- text', 'v0.2.0'), /exactly one exact header/)
})

test('staged npm gate requires four matching package versions', () => {
  const output = mkdtempSync(path.join(os.tmpdir(), 'evergate-release-gate-'))
  try {
    const version = '0.2.0-rc.1'
    for (const [dir, name] of PLATFORM_PACKAGES) {
      mkdirSync(path.join(output, dir), { recursive: true })
      writeFileSync(path.join(output, dir, 'package.json'), JSON.stringify({ name, version }))
    }
    const [wrapperDir, wrapperName] = WRAPPER_PACKAGE
    mkdirSync(path.join(output, wrapperDir), { recursive: true })
    const optionalDependencies = Object.fromEntries(PLATFORM_PACKAGES.map(([, name]) => [name, version]))
    writeFileSync(
      path.join(output, wrapperDir, 'package.json'),
      JSON.stringify({ name: wrapperName, version, optionalDependencies }),
    )
    assert.doesNotThrow(() => validateStagedPackages(output, version))

    writeFileSync(path.join(output, wrapperDir, 'package.json'), JSON.stringify({ name: wrapperName, version: '0.2.0', optionalDependencies }))
    assert.throws(() => validateStagedPackages(output, version), /version is "0.2.0"/)
  } finally {
    rmSync(output, { recursive: true, force: true })
  }
})

test('public release assets use one exact versioned name per platform', () => {
  const names = releaseAssetNames('0.3.0')
  assert.deepEqual(Object.values(names), [
    'evergate_0.3.0_windows_amd64_setup.exe',
    'evergate_0.3.0_macos_arm64_setup.dmg',
    'evergate_0.3.0_linux_amd64.tar.gz',
    'evergate_0.3.0_checksums.txt',
  ])
  assert.doesNotThrow(() => validatePublicAssets(Object.values(names), '0.3.0'))
  assert.throws(
    () => validatePublicAssets([...Object.values(names).slice(0, 3), 'evergate-windows-amd64.exe'], '0.3.0'),
    /checksums.*exactly once|unexpected public release asset/,
  )
  assert.throws(
    () => validatePublicAssets([...Object.values(names), names.linux], '0.3.0'),
    /exactly once/,
  )
})

function git(cwd, args) {
  const result = spawnSync('git', args, { cwd, encoding: 'utf8' })
  if (result.status !== 0) {
    throw new Error(`git ${args.join(' ')} failed: ${result.error?.message || result.stderr}`)
  }
  return result.stdout.trim()
}

test('CLI dry-run selects prerelease channel and rejects tags outside main', () => {
  const repo = mkdtempSync(path.join(os.tmpdir(), 'evergate-release-cli-'))
  try {
    git(repo, ['init', '-b', 'main'])
    git(repo, ['config', 'user.name', 'Release Gate Test'])
    git(repo, ['config', 'user.email', 'release-gate@example.invalid'])
    const changelog = path.join(repo, 'CHANGELOG.md')
    writeFileSync(changelog, '# Changes\n\n## v0.2.0-rc.1\n\n- prerelease\n\n## v0.2.0-rc.2\n\n- side branch\n')
    writeFileSync(path.join(repo, 'README.md'), 'test\n')
    git(repo, ['add', '.'])
    git(repo, ['commit', '-m', 'main'])
    git(repo, ['tag', 'v0.2.0-rc.1'])

    const output = path.join(repo, 'github-output')
    assert.doesNotThrow(() =>
      runCli(
        ['--tag', 'v0.2.0-rc.1', '--changelog', changelog, '--main-ref', 'main'],
        { GITHUB_OUTPUT: output },
        repo,
      ),
    )
    const githubOutput = readFileSync(output, 'utf8')
    assert.match(githubOutput, /^version=0\.2\.0-rc\.1$/m)
    assert.match(githubOutput, /^npm_tag=next$/m)
    assert.match(githubOutput, /^prerelease=true$/m)
    assert.match(githubOutput, /- prerelease/)

    git(repo, ['checkout', '-b', 'side'])
    writeFileSync(path.join(repo, 'README.md'), 'side\n')
    git(repo, ['add', '.'])
    git(repo, ['commit', '-m', 'side'])
    assert.throws(
      () =>
        runCli(
          ['--tag', 'v0.2.0-rc.2', '--changelog', changelog, '--main-ref', 'main'],
          { GITHUB_OUTPUT: '' },
          repo,
        ),
      /is not in main/,
    )
  } finally {
    rmSync(repo, { recursive: true, force: true })
  }
})

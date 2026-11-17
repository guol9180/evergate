#!/usr/bin/env node
import { appendFileSync, readFileSync } from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { spawnSync } from 'node:child_process'
import { pathToFileURL } from 'node:url'

const RELEASE_TAG = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-(rc|beta)\.([1-9]\d*))?$/

export const PLATFORM_PACKAGES = [
  ['evergate-win32-x64', '@heguoliang/evergate-win32-x64'],
  ['evergate-linux-amd64', '@heguoliang/evergate-linux-amd64'],
  ['evergate-darwin-arm64', '@heguoliang/evergate-darwin-arm64'],
]

export const WRAPPER_PACKAGE = ['evergate', '@heguoliang/evergate']

export function parseReleaseTag(tag) {
  const match = RELEASE_TAG.exec(tag)
  if (!match) {
    throw new Error(`invalid release tag ${JSON.stringify(tag)}; expected vX.Y.Z, vX.Y.Z-rc.N or vX.Y.Z-beta.N`)
  }
  const prerelease = Boolean(match[4])
  return {
    tag,
    version: tag.slice(1),
    prerelease,
    channel: prerelease ? match[4] : 'stable',
    npmTag: prerelease ? 'next' : 'latest',
  }
}

export function extractChangelogSection(markdown, tag) {
  const lines = String(markdown).replace(/\r\n?/g, '\n').split('\n')
  const header = `## ${tag}`
  const matches = []
  for (let index = 0; index < lines.length; index += 1) {
    if (lines[index] === header) matches.push(index)
  }
  if (matches.length !== 1) {
    throw new Error(`CHANGELOG.md must contain exactly one exact header ${JSON.stringify(header)} (found ${matches.length})`)
  }
  let end = matches[0] + 1
  while (end < lines.length && !lines[end].startsWith('## ')) end += 1
  const section = lines.slice(matches[0] + 1, end).join('\n').trim()
  if (!section) {
    throw new Error(`CHANGELOG.md section ${JSON.stringify(header)} is empty`)
  }
  return `${section}\n`
}

function readPackage(dir, expectedName) {
  const file = path.join(dir, 'package.json')
  let pkg
  try {
    pkg = JSON.parse(readFileSync(file, 'utf8'))
  } catch (error) {
    throw new Error(`cannot read staged package ${file}: ${error.message}`)
  }
  if (pkg.name !== expectedName) {
    throw new Error(`staged package ${file} name is ${JSON.stringify(pkg.name)}, want ${JSON.stringify(expectedName)}`)
  }
  return pkg
}

export function validateStagedPackages(outputDir, version) {
  const platformNames = PLATFORM_PACKAGES.map(([, name]) => name)
  for (const [dir, name] of PLATFORM_PACKAGES) {
    const pkg = readPackage(path.join(outputDir, dir), name)
    if (pkg.version !== version) {
      throw new Error(`staged package ${name} version is ${JSON.stringify(pkg.version)}, want ${JSON.stringify(version)}`)
    }
  }
  const [wrapperDir, wrapperName] = WRAPPER_PACKAGE
  const wrapper = readPackage(path.join(outputDir, wrapperDir), wrapperName)
  if (wrapper.version !== version) {
    throw new Error(`staged package ${wrapperName} version is ${JSON.stringify(wrapper.version)}, want ${JSON.stringify(version)}`)
  }
  const optional = wrapper.optionalDependencies ?? {}
  for (const name of platformNames) {
    if (optional[name] !== version) {
      throw new Error(`staged package ${wrapperName} optional dependency ${name} is ${JSON.stringify(optional[name])}, want ${JSON.stringify(version)}`)
    }
  }
}

export function assertTagOnMain(mainRef, cwd = process.cwd()) {
  const verify = spawnSync('git', ['rev-parse', '--verify', `${mainRef}^{commit}`], {
    cwd,
    encoding: 'utf8',
  })
  if (verify.status !== 0) {
    const detail = verify.error?.message || String(verify.stderr || '').trim() || `exit ${verify.status}`
    throw new Error(`main ref ${JSON.stringify(mainRef)} is unavailable: ${detail}`)
  }
  const ancestry = spawnSync('git', ['merge-base', '--is-ancestor', 'HEAD', mainRef], {
    cwd,
    encoding: 'utf8',
  })
  if (ancestry.status !== 0) {
    throw new Error(`tag commit ${process.env.GITHUB_SHA || 'HEAD'} is not in ${mainRef}`)
  }
}

function writeGithubOutputs(outputFile, release, body, mainRef) {
  if (!outputFile) return
  appendFileSync(
    outputFile,
    [
      `version=${release.version}`,
      `npm_tag=${release.npmTag}`,
      `prerelease=${release.prerelease}`,
      `body<<RELEASE_GATE_BODY`,
      body.trimEnd(),
      'RELEASE_GATE_BODY',
      '',
    ].join('\n'),
  )
  console.log(`release channel: ${release.channel}; main ancestry: ${mainRef}`)
}

export function runCli(argv = process.argv.slice(2), env = process.env, cwd = process.cwd()) {
  const options = { changelog: 'CHANGELOG.md', mainRef: 'origin/main', stagedDir: '' }
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index]
    if (arg === '--tag' || arg === '--changelog' || arg === '--main-ref' || arg === '--staged-dir') {
      const value = argv[++index]
      if (!value) throw new Error(`${arg} requires a value`)
      if (arg === '--tag') options.tag = value
      if (arg === '--changelog') options.changelog = value
      if (arg === '--main-ref') options.mainRef = value
      if (arg === '--staged-dir') options.stagedDir = value
      continue
    }
    throw new Error(`unknown argument ${JSON.stringify(arg)}`)
  }
  if (!options.tag) throw new Error('--tag is required')

  const release = parseReleaseTag(options.tag)
  const body = extractChangelogSection(readFileSync(options.changelog, 'utf8'), options.tag)
  assertTagOnMain(options.mainRef, cwd)
  if (options.stagedDir) validateStagedPackages(options.stagedDir, release.version)
  writeGithubOutputs(env.GITHUB_OUTPUT, release, body, options.mainRef)
  console.log(`release gate passed: ${release.tag} (${release.version}, npm --tag ${release.npmTag})`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    runCli()
  } catch (error) {
    console.error(`release gate failed: ${error.message}`)
    process.exitCode = 1
  }
}

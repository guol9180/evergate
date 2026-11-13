#!/usr/bin/env node
// 生成可发布到 npm 的四个包：1 个主包 + 3 个平台子包。
// 用法：node scripts/stage-npm.mjs <version> <binary-dir> <output-dir>
// 版本号来自 git tag（可带 v 前缀），主包与子包版本必须完全一致。
import { chmod, cp, mkdir, readFile, rm, stat, writeFile } from 'node:fs/promises'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const args = process.argv.slice(2)
if (args.length !== 3) {
  console.error('usage: node scripts/stage-npm.mjs <version> <binary-dir> <output-dir>')
  process.exit(2)
}

const version = args[0].replace(/^v/, '')
if (!/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(version)) {
  console.error(`invalid semver version: ${args[0]}`)
  process.exit(2)
}

const binaryDir = path.resolve(args[1])
const outputDir = path.resolve(args[2])

const platforms = [
  { name: 'evergate-win32-x64', os: 'win32', cpu: 'x64', source: 'evergate-windows-amd64.exe', binary: 'evergate.exe' },
  { name: 'evergate-linux-amd64', os: 'linux', cpu: 'x64', source: 'evergate-linux-amd64', binary: 'evergate' },
  { name: 'evergate-darwin-arm64', os: 'darwin', cpu: 'arm64', source: 'evergate-darwin-arm64', binary: 'evergate' },
]

const platformTemplate = JSON.parse(await readFile(path.join(root, 'npm/platform/package.json'), 'utf8'))
const wrapperTemplate = JSON.parse(await readFile(path.join(root, 'npm/wrapper/package.json'), 'utf8'))

await rm(outputDir, { recursive: true, force: true })
await mkdir(outputDir, { recursive: true })

for (const platform of platforms) {
  const source = path.join(binaryDir, platform.source)
  await stat(source)
  const dir = path.join(outputDir, platform.name)
  await mkdir(path.join(dir, 'bin'), { recursive: true })
  const target = path.join(dir, 'bin', platform.binary)
  await cp(source, target)
  if (platform.os !== 'win32') await chmod(target, 0o755)
  await cp(path.join(root, 'LICENSE'), path.join(dir, 'LICENSE'))
  await cp(path.join(root, 'npm/platform/README.md'), path.join(dir, 'README.md'))

  const pkg = {
    ...platformTemplate,
    name: platform.name,
    version,
    description: `EverGate ${platform.os}/${platform.cpu} binary (installed automatically by the evergate package)`,
    os: [platform.os],
    cpu: [platform.cpu],
  }
  await writeFile(path.join(dir, 'package.json'), `${JSON.stringify(pkg, null, 2)}\n`)
  const size = (await stat(target)).size / 1024 / 1024
  console.log(`staged ${platform.name}@${version} (${size.toFixed(1)} MB)`)
}

const wrapperDir = path.join(outputDir, 'evergate')
await mkdir(path.join(wrapperDir, 'bin'), { recursive: true })
await cp(path.join(root, 'npm/wrapper/bin/evergate.js'), path.join(wrapperDir, 'bin/evergate.js'))
await cp(path.join(root, 'npm/wrapper/README.md'), path.join(wrapperDir, 'README.md'))
await cp(path.join(root, 'LICENSE'), path.join(wrapperDir, 'LICENSE'))

const wrapper = { ...wrapperTemplate, version }
for (const name of Object.keys(wrapper.optionalDependencies)) {
  wrapper.optionalDependencies[name] = version
}
await writeFile(path.join(wrapperDir, 'package.json'), `${JSON.stringify(wrapper, null, 2)}\n`)
console.log(`staged evergate@${version}`)
console.log(`output: ${outputDir}`)

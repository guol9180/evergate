#!/usr/bin/env node
// EverGate npm 启动垫片：按 os/cpu 解析平台子包并转发参数。
// 主包故意不包含二进制，平台子包由 optionalDependencies 自动选择安装。
'use strict'

const { spawn } = require('node:child_process')
const path = require('node:path')

const PACKAGES = {
  'win32-x64': { name: '@heguoliang/evergate-win32-x64', binary: 'evergate.exe' },
  'linux-x64': { name: '@heguoliang/evergate-linux-amd64', binary: 'evergate' },
  'darwin-arm64': { name: '@heguoliang/evergate-darwin-arm64', binary: 'evergate' },
}

const key = `${process.platform}-${process.arch}`
const target = PACKAGES[key]
if (!target) {
  console.error(`evergate: unsupported platform ${key}`)
  console.error('Supported: win32-x64, linux-x64, darwin-arm64')
  console.error('Download a standalone binary: https://github.com/guol9180/evergate/releases')
  process.exit(1)
}

let binary
try {
  const pkgRoot = path.dirname(require.resolve(`${target.name}/package.json`))
  binary = path.join(pkgRoot, 'bin', target.binary)
} catch {
  console.error(`evergate: platform package ${target.name} is missing`)
  console.error('Reinstall with: npm i -g @heguoliang/evergate')
  process.exit(1)
}

const child = spawn(binary, process.argv.slice(2), { stdio: 'inherit' })
child.on('error', (err) => {
  console.error(`evergate: failed to start ${binary}: ${err.message}`)
  process.exit(1)
})
child.on('exit', (code, signal) => {
  if (signal) {
    process.kill(process.pid, signal)
    return
  }
  process.exit(code ?? 1)
})

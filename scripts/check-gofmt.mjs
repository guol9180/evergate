// gofmt 门禁（T106）：只检查项目 Go 源码目录，跨平台避免把第三方 Go 文件算进来。
import { execFileSync } from 'node:child_process'

const output = execFileSync('gofmt', ['-l', 'cmd', 'internal', 'scripts'], { encoding: 'utf8' })
if (output.trim()) {
  console.error('gofmt needed:')
  console.error(output.trimEnd())
  process.exit(1)
}

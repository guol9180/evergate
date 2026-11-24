#!/usr/bin/env node
import { readFileSync } from 'node:fs'

// relay 包语句覆盖率门禁（T74）。基线 81.0%（2026-11-24 实测），阈值不得高于基线。
const GATE = 81

const profile = process.argv[2]
if (!profile) {
  console.error('usage: check-relay-coverage.mjs <cover-func-output>')
  process.exit(2)
}

const output = readFileSync(profile, 'utf8')
const match = output.match(/^total:\s+\(statements\)\s+([\d.]+)%$/m)
if (!match) {
  console.error('coverage report is missing a statement total')
  process.exit(2)
}
const coverage = Number(match[1])
if (!Number.isFinite(coverage) || coverage < GATE) {
  console.error(`relay statement coverage ${coverage}% is below ${GATE}%`)
  process.exit(1)
}
console.log(`relay statement coverage ${coverage}% (gate: ${GATE}%)`)

#!/usr/bin/env node
import { readFileSync } from 'node:fs'

const profile = process.argv[2]
if (!profile) {
  console.error('usage: check-scheduler-coverage.mjs <cover-func-output>')
  process.exit(2)
}

const output = readFileSync(profile, 'utf8')
const match = output.match(/^total:\s+\(statements\)\s+([\d.]+)%$/m)
if (!match) {
  console.error('coverage report is missing a statement total')
  process.exit(2)
}
const coverage = Number(match[1])
if (!Number.isFinite(coverage) || coverage < 90) {
  console.error(`scheduler statement coverage ${coverage}% is below 90%`)
  process.exit(1)
}
console.log(`scheduler statement coverage ${coverage}% (gate: 90%)`)

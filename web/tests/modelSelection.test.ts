import assert from 'node:assert/strict'
import test from 'node:test'
import { applyModelSelection } from '../src/lib/modelSelection.ts'

test('append merges, de-duplicates and preserves current order', () => {
  assert.deepEqual(
    applyModelSelection(['manual-a', 'shared'], ['shared', 'upstream-b', 'upstream-b'], 'append'),
    ['manual-a', 'shared', 'upstream-b'],
  )
})

test('replace without catalog falls back to the whole selection', () => {
  assert.deepEqual(
    applyModelSelection(['manual-a', 'old-model'], ['alias-a', 'alias-b', 'alias-a'], 'replace'),
    ['alias-a', 'alias-b'],
  )
})

test('replace swaps catalog models but keeps models typed outside the catalog', () => {
  assert.deepEqual(
    applyModelSelection(['manual-a', 'old-model', 'kept'], ['alias-a', 'alias-b'], 'replace', ['old-model', 'alias-a', 'alias-b']),
    ['manual-a', 'kept', 'alias-a', 'alias-b'],
  )
})

test('replace with nothing selected only removes catalog models', () => {
  assert.deepEqual(
    applyModelSelection(['manual-a', 'old-model'], [], 'replace', ['old-model']),
    ['manual-a'],
  )
})

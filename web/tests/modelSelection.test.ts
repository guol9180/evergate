import assert from 'node:assert/strict'
import test from 'node:test'
import { applyModelSelection } from '../src/lib/modelSelection.ts'

test('append merges, de-duplicates and preserves current order', () => {
  assert.deepEqual(
    applyModelSelection(['manual-a', 'shared'], ['shared', 'upstream-b', 'upstream-b'], 'append'),
    ['manual-a', 'shared', 'upstream-b'],
  )
})

test('replace uses only the confirmed catalog selection', () => {
  assert.deepEqual(
    applyModelSelection(['manual-a', 'old-model'], ['alias-a', 'alias-b', 'alias-a'], 'replace'),
    ['alias-a', 'alias-b'],
  )
})

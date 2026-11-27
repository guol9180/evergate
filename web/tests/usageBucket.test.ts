import assert from 'node:assert/strict'
import test from 'node:test'
import { bucketByLocalDay, localDayKey } from '../src/lib/usage.ts'

const point = (hour: string, requests: number, cost: number) => ({
  hour, requests, success: requests, tokens_in: 0, tokens_out: 0, cost_micros: cost, cost: cost / 1e6,
  priced_requests: 0, unpriced_requests: 0, no_usage_requests: 0, unknown_cost_requests: 0,
})

test('UTC hourly points are summed into the user local day (T109 ⑤)', () => {
  const noon = new Date('2026-11-27T12:00:00')
  const midnight = new Date(noon.getTime() - 12 * 3_600_000)
  const same = bucketByLocalDay([point(midnight.toISOString(), 2, 10), point(new Date(midnight.getTime() + 3_600_000).toISOString(), 3, 15)])
  assert.equal(same.length, 1)
  assert.equal(same[0].date, localDayKey(noon.toISOString()))
  assert.equal(same[0].requests, 5)
  assert.equal(same[0].cost_micros, 25)
  // 前一天 23:00 的小时点归前一天，不并入「今天」
  const late = new Date(midnight.getTime() - 3_600_000)
  const split = bucketByLocalDay([point(late.toISOString(), 1, 1), point(midnight.toISOString(), 1, 1)])
  assert.equal(split.length, 2)
  assert.equal(split[0].date < split[1].date, true)
  assert.equal(split[1].date, localDayKey(noon.toISOString()))
  assert.deepEqual(bucketByLocalDay([]), [])
})
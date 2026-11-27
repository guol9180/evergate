import assert from 'node:assert/strict'
import test from 'node:test'
import { localDateKey, localDayKey } from '../src/lib/usage.ts'

// 固定为 UTC+9：本地 01:00 对应 UTC 前一天 16:00，正是「把 UTC 日期当本地日期」会出错的时间段
process.env.TZ = 'Asia/Tokyo'

test('localDateKey 取本地日期，不取 UTC 日期', () => {
  const earlyMorning = new Date(2026, 10, 27, 1, 0, 0)
  assert.equal(earlyMorning.toISOString().slice(0, 10), '2026-11-26') // 对照：UTC 口径确实落到前一天
  assert.equal(localDateKey(earlyMorning), '2026-11-27')
})

test('localDayKey 与 localDateKey 同口径', () => {
  assert.equal(localDayKey(new Date(2026, 10, 27, 1, 0, 0).toISOString()), '2026-11-27')
})

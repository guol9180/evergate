import type { RetryAttempt } from '../api/types'
import { kindText } from '../lib/format'
import { Badge } from './ui'

// 重试轨迹时间线：请求日志与 Playground 共用同一套命中链路展示。
export default function RetryTimeline({ attempts }: { attempts: RetryAttempt[] | null | undefined }) {
  if (!attempts || attempts.length === 0) {
    return <p className="text-xs text-slate-400">无尝试记录</p>
  }
  return (
    <div className="space-y-0">
      {attempts.map((a, i) => {
        const ok = !a.error_type && !a.error_message
        return (
          <div key={i} className="flex gap-3">
            <div className="flex flex-col items-center">
              <span className={`mt-1 h-2.5 w-2.5 rounded-full ${ok ? 'bg-emerald-500' : 'bg-red-500'}`} />
              {i < attempts.length - 1 && <span className="w-px flex-1 bg-slate-200" />}
            </div>
            <div className="min-w-0 flex-1 pb-5">
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="font-medium">尝试 {a.attempt}</span>
                <span className="break-all text-slate-500">→ {a.channel_name} · key #{a.key_id}</span>
                {!!a.status_code && <span className="text-xs text-slate-400">HTTP {a.status_code}</span>}
                <span className="ml-auto text-xs text-slate-400">{a.elapsed_ms}ms</span>
              </div>
              {ok ? (
                <div className="mt-0.5 text-xs text-emerald-600">成功</div>
              ) : (
                <div className="mt-0.5 text-xs">
                  <Badge tone="bg-red-100 text-red-700">{kindText(a.error_type)}</Badge>
                  <span className="ml-2 break-all text-slate-500">{a.error_message?.slice(0, 120)}</span>
                </div>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}

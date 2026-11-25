import type { RetryAttempt } from '../api/types'
import { describeUpstreamError, kindText } from '../lib/format'
import { t, tf } from '../lib/i18n'
import { Badge } from './ui'

// 重试轨迹时间线：请求日志与 Playground 共用同一套命中链路展示。
export default function RetryTimeline({ attempts }: { attempts: RetryAttempt[] | null | undefined }) {
  if (!attempts || attempts.length === 0) {
    return <p className="text-xs text-subtle">{t('retry.none')}</p>
  }
  return (
    <div className="space-y-0">
      {attempts.map((a, i) => {
        const ok = !a.error_type && !a.error_message
        const fallback = !!a.reason
      const upstream = describeUpstreamError(a.error_message)
        return (
          <div key={i} className="flex gap-3">
            <div className="flex flex-col items-center">
              <span className={`mt-1 h-2.5 w-2.5 rounded-full ${fallback ? 'bg-warning-solid' : ok ? 'bg-success-solid' : 'bg-danger-solid'}`} />
              {i < attempts.length - 1 && <span className="w-px flex-1 bg-surface-strong" />}
            </div>
            <div className={`min-w-0 flex-1 ${i < attempts.length - 1 ? 'pb-5' : 'pb-0'}`}>
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span className="font-medium">{tf('retry.attempt', { n: a.attempt })}</span>
                <span className="break-all text-muted">
                  {fallback ? `${a.from_model} → ${a.to_model}` : `→ ${a.channel_name} · key #${a.key_id}`}
                </span>
                {!!a.status_code && <span className="text-xs text-subtle">HTTP {a.status_code}</span>}
                <span className="ml-auto text-xs text-subtle">{a.elapsed_ms}ms</span>
              </div>
              {ok ? (
                <div className="mt-0.5 text-xs text-success-text">{t('retry.success')}</div>
              ) : (
                <div className="mt-0.5 text-xs">
                  <Badge tone={fallback ? 'warning' : 'danger'}>{kindText(a.error_type)}</Badge>
                  {upstream.code && <Badge tone="neutral">{tf('retry.upstreamCode', { code: upstream.code })}</Badge>}
                  <span className="ml-2 break-all text-muted" title={a.error_message}>{upstream.text}</span>
                </div>
              )}
            </div>
          </div>
        )
      })}
    </div>
  )
}

import { ArrowRight, CornerDownRight } from 'lucide-react'
import type { RetryAttempt } from '../api/types'
import { describeUpstreamError, fmtMs, httpStatusTone, kindText } from '../lib/format'
import { t, tf } from '../lib/i18n'
import { Badge } from './ui'

/** 最后一次真实上游尝试（跳过模型降级边界）；失败请求的渠道 / key 以它为准。 */
export function lastAttempt(attempts: RetryAttempt[] | null | undefined): RetryAttempt | null {
  if (!attempts) return null
  for (let i = attempts.length - 1; i >= 0; i--) {
    if (!attempts[i].reason) return attempts[i]
  }
  return null
}

// 重试轨迹时间线：请求日志与 Playground 共用同一套命中链路展示。
export default function RetryTimeline({ attempts }: { attempts: RetryAttempt[] | null | undefined }) {
  if (!attempts || attempts.length === 0) {
    return <p className="text-xs text-subtle">{t('retry.none')}</p>
  }
  return (
    <ol className="space-y-0">
      {attempts.map((a, i) => (
        <li key={i} className="flex gap-3">
          <div className="flex flex-col items-center">
            <span className={`mt-1.5 h-2.5 w-2.5 shrink-0 rounded-full ${a.reason ? 'bg-warning-solid' : !a.error_type && !a.error_message ? 'bg-success-solid' : 'bg-danger-solid'}`} />
            {i < attempts.length - 1 && <span className="w-px flex-1 bg-line" />}
          </div>
          {a.reason ? <FallbackBoundary a={a} /> : <AttemptRow a={a} last={i === attempts.length - 1} />}
        </li>
      ))}
    </ol>
  )
}

/** 模型降级边界：不是一次上游尝试，用高亮块与原因徽标标出「从这里换了模型」。 */
function FallbackBoundary({ a }: { a: RetryAttempt }) {
  return (
    <div className="eg-expand-in mb-3 flex min-w-0 flex-1 items-start gap-2 rounded-md border border-warning-border bg-warning-subtle px-2.5 py-2">
      <CornerDownRight size={14} className="mt-0.5 shrink-0 text-warning-text" aria-hidden="true" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="font-semibold text-warning-text">{t('fallback.title')}</span>
          {/* 边界原因走分类字典本地化，不回显数据层写入的展示文案 */}
          <Badge tone="warning" compact>{kindText(a.reason ?? '')}</Badge>
          {/* 边界不是真实请求，耗时为 0 时不显示，避免出现一列「—」 */}
          {a.elapsed_ms > 0 && <span className="ml-auto tabular-nums text-subtle">{fmtMs(a.elapsed_ms)}</span>}
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-1 font-mono text-xs text-foreground-soft">
          <span className="text-muted">{a.from_model}</span>
          <ArrowRight size={12} className="shrink-0 text-subtle" aria-hidden="true" />
          <span className="font-medium">{a.to_model}</span>
        </div>
      </div>
    </div>
  )
}

function AttemptRow({ a, last }: { a: RetryAttempt; last: boolean }) {
  const ok = !a.error_type && !a.error_message
  const upstream = describeUpstreamError(a.error_message)
  return (
    <div className={`min-w-0 flex-1 ${last ? 'pb-0' : 'pb-4'}`}>
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
        <span className="font-semibold text-foreground-soft">{tf('retry.attempt', { n: a.attempt })}</span>
        <span className="font-medium text-foreground">{a.channel_name || '—'}</span>
        <span className="text-subtle">key #{a.key_id}</span>
        {!!a.status_code && <Badge tone={httpStatusTone(a.status_code)} compact>HTTP {a.status_code}</Badge>}
        <span className="ml-auto tabular-nums text-subtle">{fmtMs(a.elapsed_ms)}</span>
      </div>
      {ok ? (
        <div className="mt-0.5 text-xs text-success-text">{t('retry.success')}</div>
      ) : (
        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs">
          <Badge tone="danger" compact>{kindText(a.error_type)}</Badge>
          {upstream.code && <Badge tone="neutral" compact>{tf('retry.upstreamCode', { code: upstream.code })}</Badge>}
          <span className="line-clamp-2 min-w-0 break-all text-muted" data-upstream="true" title={upstream.text}>{upstream.text}</span>
        </div>
      )}
    </div>
  )
}

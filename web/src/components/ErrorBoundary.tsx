import type { ErrorInfo, ReactNode } from 'react'
import { ErrorBoundary as ReactErrorBoundary } from 'react-error-boundary'
import { CircleAlert } from 'lucide-react'
import { Button } from './ui/button'
import { t } from '../lib/i18n'

type Props = { children: ReactNode; scope: 'app' | 'route' }

/**
 * 渲染异常兜底（react-error-boundary）。scope=route 时只替换内容区，侧栏与其他页面照常可用；
 * scope=app 挂在根部，是整页白屏之前最后一道防线。
 */
export function ErrorBoundary({ children, scope }: Props) {
  const onError = (error: unknown, info: ErrorInfo) => console.error('evergate ui crashed', error, info.componentStack)
  return (
    <ReactErrorBoundary onError={onError} FallbackComponent={(props) => <CrashFallback {...props} scope={scope} />}>
      {children}
    </ReactErrorBoundary>
  )
}

/** 兜底页只依赖 Button 这一个叶子组件，不经过 barrel：崩溃的可能正是原语本身。 */
function CrashFallback({ error, scope, resetErrorBoundary }: { error: unknown; scope: Props['scope']; resetErrorBoundary: () => void }) {
  const message = error instanceof Error ? error.message : String(error)
  return (
    <div role="alert" className={scope === 'app' ? 'flex h-full items-center justify-center p-6' : 'rounded-xl border border-line bg-surface p-6'}>
      <div className="mx-auto w-full max-w-md">
        <div className="flex items-center gap-2 text-sm font-semibold text-danger-text">
          <CircleAlert size={16} aria-hidden="true" />
          {t(scope === 'app' ? 'crash.appTitle' : 'crash.routeTitle')}
        </div>
        <p className="mt-2 text-xs leading-5 text-muted-foreground">{t('crash.desc')}</p>
        {import.meta.env.DEV && (
          <pre className="mt-3 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-surface-soft p-3 font-mono text-[11px] text-foreground-soft">
            {message}
          </pre>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="outline" onClick={() => window.location.reload()}>{t('crash.reload')}</Button>
          <Button onClick={resetErrorBoundary}>{t('common.retry')}</Button>
        </div>
      </div>
    </div>
  )
}

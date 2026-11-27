import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Activity, AlertTriangle, ArrowRight, Download, KeyRound, Plug, Timer, TowerControl, Waypoints, Zap } from 'lucide-react'
import { useChannels, useLogsStats, useTokens, useUpdateStatus } from '../api/hooks'
import { Alert, Button, Card, ContentSwap, Empty, LoadError, PageHeader, PageSkeleton, StatusBadge, Td, Th } from '../components/ui'
import { UpdateDialog } from '../components/UpdateDialog'
import { channelStateTone, fmtInt, fmtMs, fmtTime, stateLabel, statusRank } from '../lib/format'
import { currentBasePath } from '../lib/runtime'
import { usePageTitle } from '../lib/usePageTitle'
import { t, tf } from '../lib/i18n'

export default function Dashboard() {
  usePageTitle(t('page.dashboard.title'))
  const channels = useChannels()
  const tokens = useTokens()
  const update = useUpdateStatus()
  const [updateOpen, setUpdateOpen] = useState(false)
  const todayStart = useMemo(() => {
    const d = new Date()
    d.setHours(0, 0, 0, 0)
    return d.toISOString()
  }, [])
  // 成功率/延迟来自后端全量聚合，而不是最新 100 条（T75 修复口径）。
  const stats = useLogsStats({ start: todayStart })

  const chs = useMemo(() => channels.data?.items ?? [], [channels.data?.items])
  const tokenItems = tokens.data?.items ?? []
  const s = stats.data?.stats

  const facts = useMemo(() => {
    const models = new Set<string>()
    chs.forEach((c) => (c.models || []).forEach((m) => models.add(m)))
    return {
      channels: chs.length,
      models: models.size,
      keys: chs.reduce((sum, c) => sum + c.key_count, 0),
    }
  }, [chs])

  // 额度预警：剩余 ≤20% 且设置了额度的令牌。
  const lowQuotaTokens = tokenItems.filter((tok) => tok.quota > 0 && tok.remaining_quota / tok.quota <= 0.2)
  const baseURL = `${window.location.origin}${currentBasePath() || ''}/v1`

  const updateBanner = update.data?.update_available ? (
    <Alert
      tone="info"
      title={tf('dashboard.update.available', { version: String(update.data.latest_version) })}
      action={<Button size="sm" onClick={() => setUpdateOpen(true)}><Download size={14} /> {t('dashboard.update.now')}</Button>}
    >
      <p>{t('dashboard.update.desc')}</p>
      {update.data.release_url && (
        <a href={update.data.release_url} target="_blank" rel="noreferrer" className="mt-1 inline-block font-medium underline-offset-2 hover:underline">
          {t('dashboard.update.notes')}
        </a>
      )}
    </Alert>
  ) : null

  const quotaBanner = lowQuotaTokens.length > 0 ? (
    <Alert tone="warning" action={<Link to="/tokens" className="font-medium underline underline-offset-2">{t('dashboard.lowQuota.adjust')}</Link>}>
      {tf('dashboard.lowQuota.title', { count: lowQuotaTokens.length })}
      {lowQuotaTokens.slice(0, 3).map((tok) => tf('dashboard.lowQuota.item', { name: tok.name, remaining: fmtInt(tok.remaining_quota) })).join(', ')}
      {lowQuotaTokens.length > 3 ? t('dashboard.lowQuota.more') : ''}
    </Alert>
  ) : null

  const pageHeader = (
    <PageHeader
      title={t('page.dashboard.title')}
      description={t('page.dashboard.desc')}
      actions={
        <span className="text-xs text-subtle">
          {update.isPending ? t('dashboard.version.checking') : update.data?.development ? t('dashboard.version.dev') : tf('dashboard.version.current', { version: String(update.data?.current_version ?? t('common.unknown')) })}
        </span>
      }
    />
  )

  // 引导状态机：零渠道 → 有渠道零 key → 零令牌 → 零请求；每一步保留上下文直到第一次成功请求。
  // 接口失败或仍在加载时不下结论：失败给可重试的错误态，避免把「没加载到」误报成「还没有」。
  const onboarding = channels.isError || tokens.isError ? (
    <LoadError
      error={channels.error ?? tokens.error}
      onRetry={() => { void channels.refetch(); void tokens.refetch() }}
    />
  ) : channels.isPending || tokens.isPending ? null : chs.length === 0 ? (
    <Card className="p-10">
      <Empty
        icon={<TowerControl size={40} />}
        title={t('dashboard.welcome.title')}
        hint={t('dashboard.welcome.hint')}
        action={<Link to="/channels" className="inline-flex items-center gap-1 text-sm font-medium text-brand-text hover:underline">{t('dashboard.openChannels')} <ArrowRight size={12} aria-hidden="true" /></Link>}
      />
      <ol className="mx-auto max-w-md space-y-3 text-sm">
        {[
          { n: 1, text: t('dashboard.onboarding.step1'), to: '/channels' },
          { n: 2, text: t('dashboard.onboarding.step2'), to: '/channels' },
          { n: 3, text: t('dashboard.onboarding.step3'), to: '/tokens' },
          { n: 4, text: tf('dashboard.onboarding.step4', { url: baseURL }), to: null },
        ].map((step) => (
          <li key={step.n} className="flex items-center gap-3 rounded-lg border border-line px-4 py-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-subtle text-xs font-bold text-brand-text">{step.n}</span>
            <span className="flex-1 text-muted">{step.text}</span>
            {step.to && (
              <Link to={step.to} className="text-xs font-medium text-brand-text hover:underline">{t('dashboard.onboarding.go')} <ArrowRight size={12} className="inline" aria-hidden="true" /></Link>
            )}
          </li>
        ))}
      </ol>
    </Card>
  ) : facts.keys === 0 ? (
    <Card className="p-8">
      <Empty
        icon={<Plug size={36} />}
        title={t('dashboard.noKey.title')}
        hint={t('dashboard.noKey.hint')}
        action={<Link to="/channels" className="text-sm font-medium text-brand-text hover:underline">{t('dashboard.openChannels')} <ArrowRight size={12} className="inline" aria-hidden="true" /></Link>}
      />
    </Card>
  ) : tokenItems.length === 0 ? (
    <Card className="p-8">
      <Empty
        icon={<KeyRound size={36} />}
        title={t('dashboard.noToken.title')}
        hint={t('dashboard.noToken.hint')}
        action={<Link to="/tokens" className="text-sm font-medium text-brand-text hover:underline">{t('dashboard.openTokens')} <ArrowRight size={12} className="inline" aria-hidden="true" /></Link>}
      />
    </Card>
  ) : s && s.total === 0 ? (
    <Card className="p-6">
      <div className="flex flex-wrap items-center gap-3 text-sm">
        <Waypoints size={18} className="shrink-0 text-brand-text" aria-hidden="true" />
        <span className="text-foreground-soft">
          {/* 占位 URL 以 <code> 呈现：按 {url} 切片后拼装，文案本身仍走字典 */}
          {t('dashboard.noRequests.text').split('{url}').map((part, index, parts) => (
            <span key={index}>{part}{index < parts.length - 1 && <code className="rounded bg-surface-strong px-1.5 py-0.5 font-mono text-xs">{baseURL}</code>}</span>
          ))}
        </span>
        <Link to="/tokens" className="font-medium text-brand-text hover:underline">{t('dashboard.noRequests.tokens')}</Link>
      </div>
    </Card>
  ) : null

  const abnormal = [...chs].sort((a, b) => {
    const rank = statusRank(a.status) - statusRank(b.status)
    if (rank !== 0) return rank
    if (a.priority !== b.priority) return a.priority - b.priority
    return a.id - b.id
  })

  return (
    <div className="eg-page-stack">
      {pageHeader}
      <ContentSwap loading={channels.isPending} skeleton={<PageSkeleton />}>
        <div className="eg-page-stack">
          {/* 横幅紧跟页头：先知道"这是哪一页"，再读"这一页有什么要注意的" */}
          {updateBanner}
          {quotaBanner}
          {onboarding}

      {/* 事实条：列数与下方 KPI 网格同步，分段线落在卡片间隙上；1px 网格间隙透出容器底色即分隔线 */}
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-line bg-line min-[1100px]:grid-cols-4">
        {[
          { value: facts.channels, label: t('dashboard.fact.channels') },
          { value: facts.models, label: t('dashboard.fact.models') },
          { value: facts.keys, label: t('dashboard.fact.keys') },
          { value: tokenItems.length, label: t('dashboard.fact.tokens') },
        ].map((item) => (
          <div key={item.label} className="flex min-w-0 items-baseline gap-2 bg-surface-soft px-4 py-3">
            <span className="text-lg font-semibold tabular-nums text-foreground">{item.value}</span>
            <span className="truncate text-sm text-muted">{item.label}</span>
          </div>
        ))}
      </div>

      {/* 今日卡片 */}
      {stats.isError && !stats.data && (
        <LoadError error={stats.error} onRetry={() => void stats.refetch()} title={t('loadFailed.stats')} />
      )}
      <div className="eg-grid eg-grid-kpi">
        {[
          { label: t('dashboard.kpi.today'), value: fmtInt(s?.total), icon: Activity },
          { label: t('dashboard.kpi.successRate'), value: s?.success_rate != null ? `${s.success_rate.toFixed(1)}%` : '—', icon: Zap },
          { label: t('dashboard.kpi.p95'), value: fmtMs(s?.p95_ms ?? undefined), icon: Timer },
          { label: t('dashboard.kpi.abnormal'), value: fmtInt(chs.filter((c) => c.status !== 'active').length), icon: AlertTriangle },
        ].map((c) => (
          <Card key={c.label} className="p-4">
            <div className="flex items-center gap-2 text-xs text-muted">
              <c.icon size={14} className="text-subtle" aria-hidden="true" />
              {c.label}
            </div>
            <div className="mt-1 text-2xl font-bold text-foreground">{c.value}</div>
          </Card>
        ))}
      </div>

      {/* 渠道健康列表（HealthSummaryStrip 模式） */}
      {chs.length > 0 && (
        <Card flush>
          <div className="eg-card-fluid flex items-center justify-between border-b border-line-soft">
            <h3 className="text-sm font-semibold text-foreground">{t('dashboard.health.title')}</h3>
            <Link to="/channels" className="text-xs text-brand-text hover:underline">{t('dashboard.health.manage')}</Link>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[620px] lg:min-w-0">
              <thead className="border-b border-line-soft bg-surface-soft">
                <tr>
          <Th>{t('table.channel')}</Th><Th>{t('table.status')}</Th><Th className="hidden min-[1120px]:table-cell">{t('table.type')}</Th>
          <Th className="hidden min-[1200px]:table-cell">{t('table.priority')}</Th><Th>{t('table.keys')}</Th><Th>{t('table.models')}</Th>
          <Th className="hidden min-[1280px]:table-cell">{t('table.lastProbe')}</Th><Th className="hidden min-[1180px]:table-cell">{t('table.errorSummary')}</Th>
          <Th className="hidden min-[1280px]:table-cell">{t('table.updatedAt')}</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {abnormal.map((c) => (
                  <tr key={c.id}>
                    <Td className="font-medium">{c.name}</Td>
                    <Td>
                      <StatusBadge tone={channelStateTone(c.status)}>
          <span title={c.last_probe_error || undefined}>{stateLabel(c.status)}</span>
                      </StatusBadge>
                    </Td>
                    <Td className="hidden text-muted min-[1120px]:table-cell">{c.type}</Td>
                    <Td className="hidden min-[1200px]:table-cell">{c.priority}</Td>
                    <Td>
                      <span className="font-medium">{c.available_key_count}</span>
                      <span className="text-subtle"> / {c.key_count}</span>
                    </Td>
                    <Td>{(c.models || []).length}</Td>
                    <Td className="hidden text-xs text-muted min-[1280px]:table-cell">
          {c.last_probe_at ? `${fmtTime(c.last_probe_at)} · ${fmtMs(c.last_probe_latency_ms)}` : t('common.notProbed')}
                    </Td>
                    <Td className="hidden max-w-56 min-[1180px]:table-cell">
                      <div className="truncate text-xs text-subtle" title={c.last_probe_error || undefined} data-upstream={c.last_probe_error ? 'true' : undefined}>
                        {c.last_probe_error || '—'}
                      </div>
                    </Td>
                    <Td className="hidden text-xs text-subtle min-[1280px]:table-cell">{fmtTime(c.updated_at)}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
        </div>
      </ContentSwap>
      <UpdateDialog open={updateOpen} onClose={() => setUpdateOpen(false)} status={update.data} />
    </div>
  )
}

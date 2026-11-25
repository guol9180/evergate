import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Activity, AlertTriangle, ArrowRight, Download, KeyRound, Plug, Timer, TowerControl, Waypoints, Zap } from 'lucide-react'
import { useChannels, useLogsStats, useTokens, useUpdateStatus } from '../api/hooks'
import { Button, Card, Empty, PageHeader, PageSkeleton, StatusBadge, Td, Th } from '../components/ui'
import { UpdateDialog } from '../components/UpdateDialog'
import { channelStateTone, fmtInt, fmtMs, fmtTime, stateLabel, statusRank } from '../lib/format'
import { currentBasePath } from '../lib/runtime'
import { usePageTitle } from '../lib/usePageTitle'
import { t } from '../lib/i18n'

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

  const chs = channels.data?.items ?? []
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
  const lowQuotaTokens = tokenItems.filter((t) => t.quota > 0 && t.remaining_quota / t.quota <= 0.2)
  const baseURL = `${window.location.origin}${currentBasePath() || ''}/v1`

  const updateBanner = update.data?.update_available ? (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-brand-border bg-brand-subtle px-4 py-3">
      <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-surface text-brand-text shadow-sm">
        <Download size={16} aria-hidden="true" />
      </span>
      <div className="min-w-56 flex-1">
        <div className="text-sm font-semibold text-foreground">EverGate {update.data.latest_version} 可更新</div>
        <p className="text-xs text-muted">下载并校验当前平台安装包，优雅退出后进入标准安装流程；数据目录保持不变。</p>
      </div>
      <Button size="sm" onClick={() => setUpdateOpen(true)}>
        <Download size={14} /> 立即更新
      </Button>
      {update.data.release_url && (
        <a href={update.data.release_url} target="_blank" rel="noreferrer" className="text-xs font-medium text-brand-text hover:underline">
          查看更新说明
        </a>
      )}
    </div>
  ) : null

  const quotaBanner = lowQuotaTokens.length > 0 ? (
    <div role="status" className="flex flex-wrap items-center gap-3 rounded-xl border border-warning-border bg-warning-subtle px-4 py-3 text-sm text-warning-text">
      <AlertTriangle size={16} className="shrink-0" aria-hidden="true" />
      <span>
        {lowQuotaTokens.length} 个令牌额度剩余不足 20%：
        {lowQuotaTokens.slice(0, 3).map((t) => `「${t.name}」剩余 ${fmtInt(t.remaining_quota)}`).join('、')}
        {lowQuotaTokens.length > 3 ? ' 等' : ''}
      </span>
      <Link to="/tokens" className="font-medium underline-offset-2 hover:underline">调整额度</Link>
    </div>
  ) : null

  if (channels.isPending) {
    return <PageSkeleton />
  }

  const pageHeader = (
    <PageHeader
      title={t('page.dashboard.title')}
      description={t('page.dashboard.desc')}
      actions={
        <span className="text-xs text-subtle">
          {update.isPending ? '版本检查中…' : update.data?.development ? '开发版本' : `版本 ${update.data?.current_version ?? '未知'}`}
        </span>
      }
    />
  )

  // 引导状态机：零渠道 → 有渠道零 key → 零令牌 → 零请求；每一步保留上下文直到第一次成功请求。
  const onboarding = chs.length === 0 ? (
    <Card className="p-10">
      <Empty icon={<TowerControl size={40} />} title={t('dashboard.welcome.title')} hint={t('dashboard.welcome.hint')} />
      <ol className="mx-auto max-w-md space-y-3 text-sm">
        {[
          { n: 1, text: '创建渠道：选预设、填入上游 base_url 与 API key（可多行）', to: '/channels' },
          { n: 2, text: '确认渠道已导入至少一把启用的 key，并「从上游拉取」模型', to: '/channels' },
          { n: 3, text: '创建下游令牌，复制生成的客户端配置片段', to: '/tokens' },
          { n: 4, text: `客户端 base_url 指向 ${baseURL}，并发起第一次请求`, to: null },
        ].map((step) => (
          <li key={step.n} className="flex items-center gap-3 rounded-lg border border-line px-4 py-3">
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-subtle text-xs font-bold text-brand-text">{step.n}</span>
            <span className="flex-1 text-muted">{step.text}</span>
            {step.to && (
              <Link to={step.to} className="text-xs font-medium text-brand-text hover:underline">前往 <ArrowRight size={12} className="inline" aria-hidden="true" /></Link>
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
        <span className="text-foreground-soft">今天还没有请求经过网关。确认客户端 base_url 为 <code className="rounded bg-surface-strong px-1.5 py-0.5 font-mono text-xs">{baseURL}</code> 并使用已创建的令牌。</span>
        <Link to="/tokens" className="font-medium text-brand-text hover:underline">查看令牌</Link>
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
      {updateBanner}
      {quotaBanner}
      {pageHeader}
      {onboarding}

      {/* 事实行（gpt-load HomeSummary 模式） */}
      <div className="flex flex-wrap gap-x-8 gap-y-3">
        {[
          { value: facts.channels, label: '渠道' },
          { value: facts.models, label: '去重模型' },
          { value: facts.keys, label: '上游 key' },
          { value: tokenItems.length, label: '下游令牌' },
        ].map((item) => (
          <div key={item.label} className="flex items-baseline gap-2">
            <span className="text-2xl font-bold tabular-nums text-foreground">{item.value}</span>
            <span className="text-sm text-muted">{item.label}</span>
          </div>
        ))}
      </div>

      {/* 今日卡片 */}
      <div className="eg-grid eg-grid-kpi">
        {[
          { label: '今日请求', value: fmtInt(s?.total), icon: Activity },
          { label: '成功率', value: s?.success_rate != null ? `${s.success_rate.toFixed(1)}%` : '—', icon: Zap },
          { label: 'P95 延迟', value: fmtMs(s?.p95_ms ?? undefined), icon: Timer },
          { label: '异常渠道', value: fmtInt(chs.filter((c) => c.status !== 'active').length), icon: AlertTriangle },
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
            <h3 className="text-sm font-semibold text-foreground">渠道健康</h3>
            <Link to="/channels" className="text-xs text-brand-text hover:underline">管理渠道</Link>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[620px] lg:min-w-0">
              <thead className="border-b border-line-soft bg-surface-soft">
                <tr>
                  <Th>渠道</Th><Th>状态</Th><Th className="hidden min-[1120px]:table-cell">类型</Th>
                  <Th className="hidden min-[1200px]:table-cell">优先级</Th><Th>key / 可用</Th><Th>模型数</Th>
                  <Th className="hidden min-[1280px]:table-cell">最近探测</Th><Th className="hidden min-[1180px]:table-cell">异常摘要</Th>
                  <Th className="hidden min-[1280px]:table-cell">更新时间</Th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line-soft">
                {abnormal.map((c) => (
                  <tr key={c.id}>
                    <Td className="font-medium">{c.name}</Td>
                    <Td>
                      <StatusBadge tone={channelStateTone(c.status)}>
                        <span title={c.last_probe_error || undefined}>{stateLabel[c.status] || c.status}</span>
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
                      {c.last_probe_at ? `${fmtTime(c.last_probe_at)} · ${fmtMs(c.last_probe_latency_ms)}` : '未探测'}
                    </Td>
                    <Td className="hidden max-w-56 min-[1180px]:table-cell">
                      <div className="truncate text-xs text-subtle" title={c.last_probe_error || undefined}>
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
      <UpdateDialog open={updateOpen} onClose={() => setUpdateOpen(false)} status={update.data} />
    </div>
  )
}

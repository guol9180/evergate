import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { Activity, AlertTriangle, ArrowRight, Download, Timer, TowerControl, Waypoints, Zap } from 'lucide-react'
import { useChannels, useLogs, useUpdateStatus } from '../api/hooks'
import { Badge, Card, Empty, Spinner, Td, Th } from '../components/ui'
import { fmtInt, fmtMs, fmtTime, stateLabel, statusRank, stateTone } from '../lib/format'

export default function Dashboard() {
  const channels = useChannels()
  const update = useUpdateStatus()
  const todayStart = useMemo(() => {
    const d = new Date()
    d.setHours(0, 0, 0, 0)
    return d.toISOString()
  }, [])
  const logs = useLogs({ start: todayStart, page_size: 100 })

  const chs = channels.data?.items ?? []
  const logItems = logs.data?.items ?? []

  const stats = useMemo(() => {
    const models = new Set<string>()
    chs.forEach((c) => (c.models || []).forEach((m) => models.add(m)))
    const total = logs.data?.total ?? 0
    const success = logItems.filter((l) => l.status_code >= 200 && l.status_code < 400).length
    const latencies = logItems.map((l) => l.total_ms).filter((v) => v > 0).sort((a, b) => a - b)
    const p95 = latencies.length ? latencies[Math.floor(latencies.length * 0.95)] : null
    return {
      channels: chs.length,
      models: models.size,
      keys: chs.reduce((s, c) => s + c.key_count, 0),
      requests: total,
      successRate: logItems.length ? Math.round((success / logItems.length) * 100) : null,
      p95,
    }
  }, [chs, logItems, logs.data])

  const windowsAsset = update.data?.assets?.find((asset) => asset.name.endsWith('windows-amd64.exe'))
  const updateBanner = update.data?.update_available ? (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-indigo-100 bg-indigo-50/70 px-4 py-3">
      <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-white text-indigo-600 shadow-sm">
        <Download size={16} />
      </span>
      <div className="min-w-56 flex-1">
        <div className="text-sm font-semibold text-indigo-900">EverGate {update.data.latest_version} 可下载</div>
        <p className="text-xs text-indigo-700/80">下载 Windows 版后替换旧 exe 并重启；数据目录保持不变。</p>
      </div>
      {windowsAsset && (
        <a href={windowsAsset.browser_download_url} className="inline-flex h-8 items-center rounded-md bg-indigo-600 px-3 text-xs font-medium text-white transition-colors hover:bg-indigo-700" target="_blank" rel="noreferrer">
          下载 Windows 版
        </a>
      )}
      <a href={update.data.release_url} target="_blank" rel="noreferrer" className="text-xs font-medium text-indigo-700 hover:underline">
        查看更新说明
      </a>
    </div>
  ) : null

  if (channels.isPending) {
    return <div className="flex justify-center py-20"><Spinner className="h-6 w-6" /></div>
  }

  // 空状态：四步引导（new-api 快速上手模式）
  if (chs.length === 0) {
    return (
      <div className="space-y-6">
        {updateBanner}
        <Card className="p-10">
          <Empty icon={<TowerControl size={40} />} title="欢迎使用 EverGate" hint="按以下四步开始使用你的 AI 网关" />
          <ol className="mx-auto max-w-md space-y-3 text-sm">
            {[
              { n: 1, text: '创建渠道：填入上游 base_url 与支持的模型', to: '/channels' },
              { n: 2, text: '批量导入上游 API key（支持多行粘贴）', to: '/channels' },
              { n: 3, text: '创建下游令牌，发给客户端使用', to: '/tokens' },
              { n: 4, text: '客户端 base_url 指向 http://localhost:8787/v1', to: null },
            ].map((s) => (
              <li key={s.n} className="flex items-center gap-3 rounded-lg border border-slate-200 px-4 py-3">
                <span className="flex h-6 w-6 items-center justify-center rounded-full bg-indigo-100 text-xs font-bold text-indigo-700">{s.n}</span>
                <span className="flex-1 text-slate-600">{s.text}</span>
                {s.to && (
                  <Link to={s.to} className="text-xs font-medium text-indigo-600 hover:underline">前往 <ArrowRight size={12} className="inline" /></Link>
                )}
              </li>
            ))}
          </ol>
        </Card>
      </div>
    )
  }

  const abnormal = [...chs].sort((a, b) => {
    const rank = statusRank(a.status) - statusRank(b.status)
    if (rank !== 0) return rank
    if (a.priority !== b.priority) return a.priority - b.priority
    return a.id - b.id
  })

  return (
    <div className="space-y-6">
      {updateBanner}
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-bold tracking-tight">运行概览</h1>
          <p className="mt-1 text-sm text-slate-500">本地网关、渠道健康与今日请求状态。</p>
        </div>
        <div className="text-xs text-slate-400">
          {update.isPending ? '版本检查中…' : update.data?.development ? '开发版本' : `版本 ${update.data?.current_version ?? '未知'}`}
        </div>
      </header>

      {/* 事实行（gpt-load HomeSummary 模式） */}
      <div className="flex flex-wrap items-baseline gap-x-6 gap-y-2">
        <span className="text-2xl font-bold">{stats.channels}</span><span className="text-sm text-slate-500">渠道</span>
        <span className="text-2xl font-bold">{stats.models}</span><span className="text-sm text-slate-500">去重模型</span>
        <span className="text-2xl font-bold">{stats.keys}</span><span className="text-sm text-slate-500">上游 key</span>
      </div>

      {/* 今日卡片 */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        {[
          { label: '今日请求', value: fmtInt(stats.requests), icon: Activity },
          { label: '成功率', value: stats.successRate != null ? `${stats.successRate}%` : '—', icon: Zap },
          { label: 'P95 延迟', value: fmtMs(stats.p95 ?? undefined), icon: Timer },
          { label: '异常渠道', value: fmtInt(chs.filter((c) => c.status !== 'active').length), icon: AlertTriangle },
        ].map((c) => (
          <Card key={c.label} className="p-4">
            <div className="flex items-center gap-2 text-xs text-slate-500">
              <c.icon size={14} className="text-slate-400" />
              {c.label}
            </div>
            <div className="mt-1 text-2xl font-bold text-slate-900">{c.value}</div>
          </Card>
        ))}
      </div>

      {/* 渠道健康列表（HealthSummaryStrip 模式） */}
      <Card>
        <div className="flex items-center justify-between border-b border-slate-100 px-4 py-3">
          <h3 className="text-sm font-semibold">渠道健康</h3>
          <Link to="/channels" className="text-xs text-indigo-600 hover:underline">管理渠道</Link>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead className="border-b border-slate-100 bg-slate-50/50">
              <tr><Th>渠道</Th><Th>状态</Th><Th>类型</Th><Th>优先级</Th><Th>key / 可用</Th><Th>模型数</Th><Th>最近探测</Th><Th>异常摘要</Th><Th>更新时间</Th></tr>
            </thead>
            <tbody className="divide-y divide-slate-50">
              {abnormal.map((c) => (
                <tr key={c.id}>
                  <Td className="font-medium">{c.name}</Td>
                  <Td>
                    <Badge tone={stateTone[c.status] || ''}>
                      <span title={c.last_probe_error || undefined}>{stateLabel[c.status] || c.status}</span>
                    </Badge>
                  </Td>
                  <Td className="text-slate-500">{c.type}</Td>
                  <Td>{c.priority}</Td>
                  <Td>
                    <span className="font-medium">{c.available_key_count}</span>
                    <span className="text-slate-400"> / {c.key_count}</span>
                  </Td>
                  <Td>{(c.models || []).length}</Td>
                  <Td className="text-xs text-slate-500">
                    {c.last_probe_at ? `${fmtTime(c.last_probe_at)} · ${fmtMs(c.last_probe_latency_ms)}` : '未探测'}
                  </Td>
                  <Td className="max-w-56">
                    <div className="truncate text-xs text-slate-400" title={c.last_probe_error || undefined}>
                      {c.last_probe_error || '—'}
                    </div>
                  </Td>
                  <Td className="text-xs text-slate-400">{fmtTime(c.updated_at)}</Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}

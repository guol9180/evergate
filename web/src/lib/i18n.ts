// 零依赖 i18n（T79）：模块级 locale + useSyncExternalStore，无需 Context 注入。
import { useSyncExternalStore } from 'react'

export type Locale = 'zh-CN' | 'en'

const STORAGE_KEY = 'eg-locale'

export const dict = {
  'zh-CN': {
    'app.name': 'EverGate',
    'app.tagline': 'AI 网关管理台',
    'nav.dashboard': '运行概览',
    'nav.channels': '渠道管理',
    'nav.tokens': '令牌管理',
    'nav.logs': '请求日志',
    'nav.playground': '调试台',
    'nav.usage': '用量成本',
    'nav.settings': '系统设置',
    'nav.main': '主导航',
    'nav.logout': '退出登录',
    'nav.theme': '主题',
    'theme.light': '浅色',
    'theme.dark': '深色',
    'theme.system': '跟随系统',
    'theme.toggle': '切换主题',
    'theme.current': '当前',
    'theme.next': '点击切换为',
    'locale.switch': 'Switch to English',
    'common.loading': '加载中…',
    'common.create': '新建',
    'common.save': '保存',
    'common.saving': '保存中…',
    'common.cancel': '取消',
    'common.refresh': '刷新',
    'common.delete': '删除',
    'common.edit': '编辑',
    'common.close': '关闭',
    'common.copy': '复制',
    'common.copied': '已复制',
    'common.export': '导出 CSV',
    'common.exporting': '导出中…',
    'common.testAll': '测试全部',
    'common.search': '变更即筛选',
    'common.send': '发送',
    'common.sending': '请求中…',
    'common.stop': '停止',
    'common.settingsMissing': '设置不存在',
    'page.dashboard.title': '运行概览',
    'page.dashboard.desc': '本地网关、渠道健康与今日请求状态。',
    'page.channels.title': '渠道管理',
    'page.channels.desc': '上游渠道、优先级权重与 key 池状态；展开行可管理凭据。',
    'page.channels.create': '新建渠道',
    'page.tokens.title': '令牌管理',
    'page.tokens.desc': '下游 API key、额度、有效期与访问白名单；明文 key 仅在创建时展示一次。',
    'page.tokens.create': '新建令牌',
    'page.logs.title': '请求日志',
    'page.logs.desc': '按模型、状态、渠道、令牌与时间范围筛选；点击行查看命中链路与重试轨迹。',
    'page.playground.title': '调试台',
    'page.playground.desc': '用管理台会话直接验证渠道与协议兼容性；支持 prompt、system、model、stream 与四种下游协议，工具调用等完整请求请使用真实客户端经 /v1 发起。',
    'page.usage.title': '用量成本',
    'page.usage.desc': '按 UTC 日聚合请求、token 与价格快照。',
    'page.settings.title': '系统设置',
    'page.settings.desc': 'SQLite 是运行时配置的唯一事实源；首次初始化来自 evergate.yml，保存后无需重启。',
    'page.login.title': '登录',
    'page.changePassword.title': '修改密码',
    'empty.channels.title': '还没有渠道',
    'empty.channels.hint': '新建一个上游渠道，填入 base_url 与支持的模型',
    'empty.tokens.title': '还没有下游令牌',
    'empty.tokens.hint': '创建令牌后发给客户端，作为调用网关的 API key',
    'empty.logs.title': '暂无日志',
    'empty.logs.hint': '有请求经过网关后，这里会显示每次调用的完整轨迹',
    'empty.usage.title': '所选范围暂无数据',
    'empty.playground.title': '还没有响应',
    'empty.playground.hint': '选择协议与模型，输入消息后发送；流式响应会逐段显示。',
    'empty.audit.title': '还没有审计记录',
    'empty.audit.hint': '在管理台进行写操作后会出现在这里',
    'dashboard.welcome.title': '欢迎使用 EverGate',
    'dashboard.welcome.hint': '按以下四步开始使用你的 AI 网关',
    'dashboard.noKey.title': '渠道还没有可用的上游 key',
    'dashboard.noKey.hint': '展开渠道行，粘贴一把或多把上游 API key；导入后可从上游拉取模型目录。',
    'dashboard.noToken.title': '还没有下游令牌',
    'dashboard.noToken.hint': '创建令牌后会生成客户端配置片段，可粘贴到 Codex、Claude Code、Cline 或 Cursor。',
    'dashboard.openChannels': '去渠道管理',
    'dashboard.openTokens': '去创建令牌',
    'playground.waiting': '等待首个 token…',
    'playground.noContent': '上游返回成功，但没有可显示的文本内容',
    'playground.aborted': '已中断本次请求，已输出的内容会保留。',
    'playground.stream': '流式输出',
    'playground.streamHint': '逐段渲染增量内容',
    'login.heading': 'EverGate 管理台',
    'login.username': '用户名',
    'login.password': '密码',
    'login.submit': '登录',
    'login.submitting': '登录中…',
    'changePassword.heading': '首次登录须修改密码',
    'changePassword.hint': '初始密码仅在首次启动时显示，请立即更换为自己保管的强密码',
    'changePassword.old': '旧密码',
    'changePassword.new': '新密码（≥8 位）',
    'changePassword.confirm': '确认新密码',
    'changePassword.submit': '修改密码',
    'changePassword.submitting': '提交中…',
    'changePassword.tooShort': '新密码至少 8 位',
    'changePassword.mismatch': '两次输入的新密码不一致',
  },
  en: {
    'app.name': 'EverGate',
    'app.tagline': 'AI Gateway Console',
    'nav.dashboard': 'Overview',
    'nav.channels': 'Channels',
    'nav.tokens': 'Tokens',
    'nav.logs': 'Request Logs',
    'nav.playground': 'Playground',
    'nav.usage': 'Usage & Cost',
    'nav.settings': 'Settings',
    'nav.main': 'Main navigation',
    'nav.logout': 'Sign out',
    'nav.theme': 'Theme',
    'theme.light': 'Light',
    'theme.dark': 'Dark',
    'theme.system': 'System',
    'theme.toggle': 'Toggle theme',
    'theme.current': 'current',
    'theme.next': 'click to switch to',
    'locale.switch': '切换到中文',
    'common.loading': 'Loading…',
    'common.create': 'New',
    'common.save': 'Save',
    'common.saving': 'Saving…',
    'common.cancel': 'Cancel',
    'common.refresh': 'Refresh',
    'common.delete': 'Delete',
    'common.edit': 'Edit',
    'common.close': 'Close',
    'common.copy': 'Copy',
    'common.copied': 'Copied',
    'common.export': 'Export CSV',
    'common.exporting': 'Exporting…',
    'common.testAll': 'Test all',
    'common.search': 'Filters apply on change',
    'common.send': 'Send',
    'common.sending': 'Sending…',
    'common.stop': 'Stop',
    'common.settingsMissing': 'Settings not available',
    'page.dashboard.title': 'Overview',
    'page.dashboard.desc': 'Local gateway, channel health and today\'s requests.',
    'page.channels.title': 'Channels',
    'page.channels.desc': 'Upstream channels, priority weights and key pool state; expand a row to manage credentials.',
    'page.channels.create': 'New channel',
    'page.tokens.title': 'Tokens',
    'page.tokens.desc': 'Downstream API keys, quotas, expiry and allow-lists; the plaintext key is shown once at creation.',
    'page.tokens.create': 'New token',
    'page.logs.title': 'Request Logs',
    'page.logs.desc': 'Filter by model, status, channel, token and time range; click a row for routing and retry trace.',
    'page.playground.title': 'Playground',
    'page.playground.desc': 'Validate channels and protocol compatibility from the admin session; supports prompt, system, model, stream and four downstream protocols. Use a real client via /v1 for tools and full request bodies.',
    'page.usage.title': 'Usage & Cost',
    'page.usage.desc': 'Requests, tokens and price snapshots aggregated by UTC day.',
    'page.settings.title': 'Settings',
    'page.settings.desc': 'SQLite is the single source of runtime configuration; seeded from evergate.yml, no restart needed after saving.',
    'page.login.title': 'Sign in',
    'page.changePassword.title': 'Change password',
    'empty.channels.title': 'No channels yet',
    'empty.channels.hint': 'Create an upstream channel with its base_url and supported models',
    'empty.tokens.title': 'No downstream tokens yet',
    'empty.tokens.hint': 'Create a token and hand it to clients as the gateway API key',
    'empty.logs.title': 'No logs yet',
    'empty.logs.hint': 'Once requests pass through the gateway, every call shows its full trace here',
    'empty.usage.title': 'No data in the selected range',
    'empty.playground.title': 'No response yet',
    'empty.playground.hint': 'Choose a protocol and model, enter a message and send; streaming output appears incrementally.',
    'empty.audit.title': 'No audit records yet',
    'empty.audit.hint': 'Admin write operations will appear here',
    'dashboard.welcome.title': 'Welcome to EverGate',
    'dashboard.welcome.hint': 'Follow these four steps to start your AI gateway',
    'dashboard.noKey.title': 'No active upstream key',
    'dashboard.noKey.hint': 'Expand a channel and paste one or more upstream API keys; then pull the model catalog.',
    'dashboard.noToken.title': 'No downstream token yet',
    'dashboard.noToken.hint': 'Create a token to get client configuration snippets for Codex, Claude Code, Cline or Cursor.',
    'dashboard.openChannels': 'Open channels',
    'dashboard.openTokens': 'Create a token',
    'playground.waiting': 'Waiting for the first token…',
    'playground.noContent': 'The upstream succeeded but returned no displayable text',
    'playground.aborted': 'Request interrupted; generated output is preserved.',
    'playground.stream': 'Streaming',
    'playground.streamHint': 'Render incremental output as it arrives',
    'login.heading': 'EverGate Console',
    'login.username': 'Username',
    'login.password': 'Password',
    'login.submit': 'Sign in',
    'login.submitting': 'Signing in…',
    'changePassword.heading': 'Change your password',
    'changePassword.hint': 'The initial password is shown only once. Replace it with a strong password you control.',
    'changePassword.old': 'Current password',
    'changePassword.new': 'New password (8+ characters)',
    'changePassword.confirm': 'Confirm new password',
    'changePassword.submit': 'Change password',
    'changePassword.submitting': 'Submitting…',
    'changePassword.tooShort': 'The new password must contain at least 8 characters',
    'changePassword.mismatch': 'The new passwords do not match',
  },
} as const

export type MessageKey = keyof typeof dict['zh-CN']

// 编译期保证 zh/en 键集合一致：en 缺键或多键都会在此处报类型错误。
type EnKeys = keyof typeof dict['en']
const _assertSameKeys: [Exclude<MessageKey, EnKeys>] extends [never] ? ([Exclude<EnKeys, MessageKey>] extends [never] ? true : never) : never = true
void _assertSameKeys

let current: Locale = detect()
const listeners = new Set<() => void>()

function detect(): Locale {
  if (typeof window === 'undefined') return 'zh-CN'
  return window.localStorage.getItem(STORAGE_KEY) === 'en' ? 'en' : 'zh-CN'
}

export function getLocale(): Locale {
  return current
}

export function setLocale(locale: Locale) {
  current = locale
  if (typeof window !== 'undefined') window.localStorage.setItem(STORAGE_KEY, locale)
  if (typeof document !== 'undefined') document.documentElement.lang = locale
  listeners.forEach((fn) => fn())
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

export function useLocale(): Locale {
  return useSyncExternalStore(subscribe, getLocale, getLocale)
}

export function t(key: MessageKey): string {
  return dict[current][key] ?? dict['zh-CN'][key]
}

/** 字典单测用：返回两种语言的键集合差异。 */
export function dictionaryDiff(): { missingInEn: string[]; missingInZh: string[] } {
  const zh = Object.keys(dict['zh-CN'])
  const en = Object.keys(dict.en)
  return {
    missingInEn: zh.filter((k) => !en.includes(k)),
    missingInZh: en.filter((k) => !zh.includes(k)),
  }
}

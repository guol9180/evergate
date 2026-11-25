import type { MessageKey } from './i18n'

export interface ChannelPreset {
  id: string
  labelKey: MessageKey
  baseUrl: string
  type: string
  supportsResponses: boolean
  supportsAnthropic: boolean
  supportsGemini?: boolean
  defaultName: string
  descKey: MessageKey
  warningKey?: MessageKey
  modelsPlaceholder: string
}

export const CUSTOM_CHANNEL_PRESET = 'custom'

// 厂商基址的唯一维护点：新增/调整厂商只改这里，表单与提示自动生效；文案键见 i18n.ts 的 preset.* 。
export const CHANNEL_PRESETS: ChannelPreset[] = [
  {
    id: 'openai',
    labelKey: 'preset.openai.label',
    baseUrl: 'https://api.openai.com/v1',
    type: 'openai',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'openai-main',
    descKey: 'preset.openai.desc',
    modelsPlaceholder: 'gpt-4o\ngpt-4o-mini',
  },
  {
    id: 'deepseek',
    labelKey: 'preset.deepseek.label',
    baseUrl: 'https://api.deepseek.com',
    type: 'deepseek',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'deepseek-main',
    descKey: 'preset.deepseek.desc',
    modelsPlaceholder: 'deepseek-chat\ndeepseek-reasoner',
  },
  {
    id: 'kimi',
    labelKey: 'preset.kimi.label',
    baseUrl: 'https://api.moonshot.cn/v1',
    type: 'openai-compatible',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'kimi-main',
    descKey: 'preset.kimi.desc',
    modelsPlaceholder: 'moonshot-v1-8k\nmoonshot-v1-32k',
  },
  {
    id: 'bigmodel',
    labelKey: 'preset.bigmodel.label',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    type: 'openai-compatible',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'bigmodel-main',
    descKey: 'preset.bigmodel.desc',
    modelsPlaceholder: 'glm-4.6\nglm-4.5-air',
  },
  {
    id: 'bigmodel-coding',
    labelKey: 'preset.bigmodel-coding.label',
    baseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4',
    type: 'openai-compatible',
    supportsResponses: false,
    supportsAnthropic: false,
    defaultName: 'bigmodel-coding',
    descKey: 'preset.bigmodel-coding.desc',
    warningKey: 'preset.bigmodel-coding.warning',
    modelsPlaceholder: 'glm-4.6',
  },
  {
    id: 'opencode-go',
    labelKey: 'preset.opencode-go.label',
    baseUrl: 'https://opencode.ai/inference/go/openai/v1',
    type: 'openai-compatible',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'opencode-go',
    descKey: 'preset.opencode-go.desc',
    modelsPlaceholder: 'glm-5.3-flash\ngpt-6-luna\nkimi-k3',
  },
  {
    id: 'zai-coding',
    labelKey: 'preset.zai-coding.label',
    baseUrl: 'https://api.z.ai/api/coding/paas/v4',
    type: 'openai-compatible',
    supportsResponses: false,
    supportsAnthropic: false,
    defaultName: 'zai-coding',
    descKey: 'preset.zai-coding.desc',
    warningKey: 'preset.zai-coding.warning',
    modelsPlaceholder: 'glm-4.6',
  },
  {
    id: 'anthropic',
    labelKey: 'preset.anthropic.label',
    baseUrl: 'https://api.anthropic.com',
    type: 'anthropic',
    supportsResponses: false,
    supportsAnthropic: true,
    defaultName: 'anthropic-main',
    descKey: 'preset.anthropic.desc',
    modelsPlaceholder: 'claude-sonnet-4-5\nclaude-opus-4-1',
  },
  {
    id: 'deepseek-anthropic',
    labelKey: 'preset.deepseek-anthropic.label',
    baseUrl: 'https://api.deepseek.com/anthropic',
    type: 'anthropic-compatible',
    supportsResponses: false,
    supportsAnthropic: true,
    defaultName: 'deepseek-anthropic',
    descKey: 'preset.deepseek-anthropic.desc',
    modelsPlaceholder: 'deepseek-chat\ndeepseek-reasoner',
  },
  {
    id: 'kimi-anthropic',
    labelKey: 'preset.kimi-anthropic.label',
    baseUrl: 'https://api.moonshot.ai/anthropic',
    type: 'anthropic-compatible',
    supportsResponses: false,
    supportsAnthropic: true,
    defaultName: 'kimi-anthropic',
    descKey: 'preset.kimi-anthropic.desc',
    modelsPlaceholder: 'kimi-k2-0905-preview',
  },
  {
    id: 'gemini',
    labelKey: 'preset.gemini.label',
    baseUrl: 'https://generativelanguage.googleapis.com',
    type: 'gemini',
    supportsResponses: false,
    supportsAnthropic: false,
    supportsGemini: true,
    defaultName: 'gemini-main',
    descKey: 'preset.gemini.desc',
    modelsPlaceholder: 'gemini-2.5-pro\ngemini-2.5-flash',
  },
]

export function findChannelPreset(id: string): ChannelPreset | undefined {
  return CHANNEL_PRESETS.find((preset) => preset.id === id)
}

// 预设渠道的名称自动生成：默认名被占用时依次尝试 -2 / -3 …（名称有唯一索引，重复创建同一预设时必须避开）。
export function uniquePresetName(base: string, taken: string[]): string {
  const used = new Set(taken)
  if (!used.has(base)) return base
  for (let i = 2; ; i++) {
    const candidate = `${base}-${i}`
    if (!used.has(candidate)) return candidate
  }
}

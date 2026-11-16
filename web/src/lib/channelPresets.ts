export interface ChannelPreset {
  id: string
  label: string
  baseUrl: string
  type: string
  supportsResponses: boolean
  supportsAnthropic: boolean
  supportsGemini?: boolean
  defaultName: string
  description: string
  warning?: string
  modelsPlaceholder: string
}

export const CUSTOM_CHANNEL_PRESET = 'custom'

// 厂商基址的唯一维护点：新增/调整厂商只改这里，表单与提示自动生效。
export const CHANNEL_PRESETS: ChannelPreset[] = [
  {
    id: 'openai',
    label: 'OpenAI 官方',
    baseUrl: 'https://api.openai.com/v1',
    type: 'openai',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'openai-main',
    description: '官方 API，Chat Completions 与 Responses 均原生支持。',
    modelsPlaceholder: 'gpt-4o\ngpt-4o-mini',
  },
  {
    id: 'deepseek',
    label: 'DeepSeek',
    baseUrl: 'https://api.deepseek.com',
    type: 'deepseek',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'deepseek-main',
    description: '官方兼容根地址（可省略 /v1），Chat Completions 与 Responses 均原生支持。',
    modelsPlaceholder: 'deepseek-chat\ndeepseek-reasoner',
  },
  {
    id: 'kimi',
    label: 'Kimi（Moonshot 开放平台）',
    baseUrl: 'https://api.moonshot.cn/v1',
    type: 'openai-compatible',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'kimi-main',
    description: 'Moonshot 开放平台 OpenAI 兼容端点，支持 Chat Completions 与 Responses。',
    modelsPlaceholder: 'moonshot-v1-8k\nmoonshot-v1-32k',
  },
  {
    id: 'bigmodel',
    label: '智谱 BigModel（常规）',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    type: 'openai-compatible',
    supportsResponses: true,
    supportsAnthropic: false,
    defaultName: 'bigmodel-main',
    description: '标准 API 套餐使用此地址，Chat Completions 与 Response API 均可用。',
    modelsPlaceholder: 'glm-4.6\nglm-4.5-air',
  },
  {
    id: 'bigmodel-coding',
    label: '智谱 BigModel Coding',
    baseUrl: 'https://open.bigmodel.cn/api/coding/paas/v4',
    type: 'openai-compatible',
    supportsResponses: false,
    supportsAnthropic: false,
    defaultName: 'bigmodel-coding',
    description: 'GLM Coding 套餐专用地址，与常规 API 地址不可混用。',
    warning: 'Coding 套餐必须使用 /api/coding/paas/v4；填成常规 /api/paas/v4 会报 503 或套餐不可用。',
    modelsPlaceholder: 'glm-4.6',
  },
  {
    id: 'zai-coding',
    label: 'Z.ai Coding（国际版）',
    baseUrl: 'https://api.z.ai/api/coding/paas/v4',
    type: 'openai-compatible',
    supportsResponses: false,
    supportsAnthropic: false,
    defaultName: 'zai-coding',
    description: 'Z.ai Coding 套餐专用地址；国际版与国内站账号不互通。',
    warning: 'Coding 套餐必须使用 /api/coding/paas/v4；填成常规 /api/paas/v4 会报 503 或套餐不可用。',
    modelsPlaceholder: 'glm-4.6',
  },
  {
    id: 'anthropic',
    label: 'Anthropic 官方（Messages）',
    baseUrl: 'https://api.anthropic.com',
    type: 'anthropic',
    supportsResponses: false,
    supportsAnthropic: true,
    defaultName: 'anthropic-main',
    description: '官方 Messages 原生端点，网关按 Base URL + /v1/messages 透传。',
    modelsPlaceholder: 'claude-sonnet-4-5\nclaude-opus-4-1',
  },
  {
    id: 'deepseek-anthropic',
    label: 'DeepSeek（Anthropic 兼容）',
    baseUrl: 'https://api.deepseek.com/anthropic',
    type: 'anthropic-compatible',
    supportsResponses: false,
    supportsAnthropic: true,
    defaultName: 'deepseek-anthropic',
    description: 'DeepSeek 的 Messages 兼容端点；与 OpenAI 兼容渠道需分开配置。',
    modelsPlaceholder: 'deepseek-chat\ndeepseek-reasoner',
  },
  {
    id: 'kimi-anthropic',
    label: 'Kimi（Anthropic 兼容）',
    baseUrl: 'https://api.moonshot.ai/anthropic',
    type: 'anthropic-compatible',
    supportsResponses: false,
    supportsAnthropic: true,
    defaultName: 'kimi-anthropic',
    description: 'Moonshot 国际站的 Messages 兼容端点；Base URL 必须包含 /anthropic。',
    modelsPlaceholder: 'kimi-k2-0905-preview',
  },
  {
    id: 'gemini',
    label: 'Google Gemini 官方',
    baseUrl: 'https://generativelanguage.googleapis.com',
    type: 'gemini',
    supportsResponses: false,
    supportsAnthropic: false,
    supportsGemini: true,
    defaultName: 'gemini-main',
    description: 'Gemini API 原生端点；网关按 Base URL + /v1beta/models/{model} 透传。',
    modelsPlaceholder: 'gemini-2.5-pro\ngemini-2.5-flash',
  },
]

export function findChannelPreset(id: string): ChannelPreset | undefined {
  return CHANNEL_PRESETS.find((preset) => preset.id === id)
}

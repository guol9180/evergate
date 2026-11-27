// 统一 API 客户端：处理 {ok, data, message} 信封与错误
import { withBasePath } from '../lib/runtime'

let unauthorizedHandler: (() => void) | null = null
let unauthorizedFired = false

/**
 * 注册 401 处理（main.tsx）：会话过期统一跳登录并带回跳地址，页面与 hook 不再各自判断。
 * 登录 / 首启接口自身的 401 是「凭据错误」，不触发；整页跳转后模块状态自然复位。
 */
export function setUnauthorizedHandler(handler: (() => void) | null) {
  unauthorizedHandler = handler
}

function isCredentialEndpoint(path: string): boolean {
  return /\/admin\/api\/auth\/(login|setup|setup-status)/.test(path)
}

export class ApiError extends Error {
  status: number
  code?: string
  constructor(message: string, status: number, code?: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(withBasePath(path), {
    credentials: 'same-origin',
    ...init,
    headers: {
      // FormData 由浏览器自带 boundary，不能手动指定 Content-Type
      ...(init?.body && !(init.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  })
  let json: { ok?: boolean; data?: T; message?: string; code?: string }
  try {
    json = await resp.json()
  } catch {
    throw new ApiError(`HTTP ${resp.status}`, resp.status)
  }
  if (!resp.ok || json.ok === false) {
    if (resp.status === 401 && !isCredentialEndpoint(path) && unauthorizedHandler && !unauthorizedFired) {
      unauthorizedFired = true
      unauthorizedHandler()
    }
    throw new ApiError(json.message || `HTTP ${resp.status}`, resp.status, json.code)
  }
  return json.data as T
}

export function jsonBody(data: unknown): RequestInit {
  return { method: 'POST', body: JSON.stringify(data) }
}

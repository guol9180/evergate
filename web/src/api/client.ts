// 统一 API 客户端：处理 {ok, data, message} 信封与错误
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
  const resp = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
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
    throw new ApiError(json.message || `HTTP ${resp.status}`, resp.status, json.code)
  }
  return json.data as T
}

export function jsonBody(data: unknown): RequestInit {
  return { method: 'POST', body: JSON.stringify(data) }
}

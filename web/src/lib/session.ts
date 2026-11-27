// 会话过期回跳：登录后回到原页。只接受站内相对路径，避免开放重定向。
import { currentBasePath, withBasePath } from './runtime'

export function safeNext(value: string | null | undefined): string {
  if (!value || !value.startsWith('/') || value.startsWith('//') || value.startsWith('/login') || value.startsWith('/setup')) return '/'
  return value
}

/** 路由器外（API 客户端）读取当前地址：去掉 base path，保留 query。 */
export function currentRoute(): string {
  const base = currentBasePath()
  const path = window.location.pathname
  const route = base && path.startsWith(base) ? path.slice(base.length) || '/' : path
  return `${route}${window.location.search}`
}

export function loginUrl(next: string): string {
  return `/login?next=${encodeURIComponent(next)}`
}

export function loginHref(next: string): string {
  return withBasePath(loginUrl(next))
}

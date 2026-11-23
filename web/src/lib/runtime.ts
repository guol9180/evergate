// Runtime base path is injected into the embedded index.html by the Go server.
// Keep URL joining in one place so API, router, and public assets share the
// same base_path contract.
export function pathnameFromHref(href: string, origin = 'http://evergate.local'): string {
  const path = new URL(href || '/', origin).pathname
  if (path === '/' || path === '') return ''
  return path.replace(/\/+$/, '')
}

export function currentBasePath(): string {
  if (typeof document === 'undefined') return ''
  const href = document.querySelector('base')?.getAttribute('href') || './'
  return pathnameFromHref(href, window.location.origin)
}

export function withBasePath(path: string, base = currentBasePath()): string {
  const suffix = path.startsWith('/') ? path : `/${path}`
  if (!base) return suffix
  return `${base}${suffix}`
}

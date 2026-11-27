import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider, useMutation } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import { Toaster } from '../../src/components/Toaster'
import { createMutationCache } from '../../src/lib/mutationCache'
import { safeNext } from '../../src/lib/session'
import { errorMessage } from '../../src/lib/errors'
import { ApiError } from '../../src/api/client'
import { setLocale } from '../../src/lib/i18n'

function harness(silent: boolean, message: string) {
  const client = new QueryClient({ mutationCache: createMutationCache() })
  let fire = () => {}
  function Probe() {
    const mutation = useMutation({
      mutationFn: async () => { throw new Error(message) },
      meta: silent ? { silentError: true } : undefined,
    })
    fire = () => void mutation.mutate()
    return null
  }
  render(<QueryClientProvider client={client}><Probe /><Toaster /></QueryClientProvider>)
  return () => fire()
}

describe('global mutation error feedback (T109)', () => {
  it('toasts mutation errors unless the mutation opts out', async () => {
    harness(false, '上游返回 500')()
    expect(await screen.findByText('操作失败')).toBeTruthy()
    expect(screen.getByText('上游返回 500')).toBeTruthy()
  })

  it('stays silent when the page renders the error inline', async () => {
    // sonner 的 store 是全局的，上一个用例的 Toast 会在新 Toaster 里复现；用本用例独有的文案断言「没弹」
    harness(true, '内联场景不应弹 Toast')()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(screen.queryByText('内联场景不应弹 Toast')).toBeNull()
  })
})

describe('safeNext', () => {
  it('accepts in-app paths only', () => {
    expect(safeNext('/logs?page=2')).toBe('/logs?page=2')
    expect(safeNext(null)).toBe('/')
    expect(safeNext('https://evil.example/')).toBe('/')
    expect(safeNext('//evil.example/')).toBe('/')
    expect(safeNext('/login')).toBe('/')
  })
})

describe('errorMessage (T109 ③)', () => {
  it('localizes by stable code, keeps dynamic details, falls back to the raw message', () => {
    setLocale('en')
    // 前缀按字典本地化，细节原样附带
    expect(errorMessage(new ApiError('参数错误：模型不能为空', 400, 'invalid_param'))).toBe('Invalid request: 模型不能为空')
    // 静态文案只给字典文本
    expect(errorMessage(new ApiError('查询失败', 500, 'query_failed'))).toBe('Query failed')
    // 未知 code / 无 code 回退原始 message
    expect(errorMessage(new ApiError('boom', 500, 'unknown_code'))).toBe('boom')
    expect(errorMessage(new Error('plain'))).toBe('plain')
    expect(errorMessage('text')).toBe('text')
    setLocale('zh-CN')
    expect(errorMessage(new ApiError('参数错误：模型不能为空', 400, 'invalid_param'))).toBe('参数错误：模型不能为空')
  })
})

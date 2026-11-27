import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider, useMutation } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import { Toaster } from '../../src/components/Toaster'
import { createMutationCache } from '../../src/lib/mutationCache'

/** 失败反馈只有一个出处：全局 MutationCache 默认弹一条；页面内联展示错误的 mutation 必须声明 silentError。 */
function harness(silent: boolean) {
  const client = new QueryClient({ mutationCache: createMutationCache() })
  const message = `失败详情 ${silent} ${Date.now()}`
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
  return { fire: () => fire(), message }
}

describe('mutation 失败反馈只出现一次', () => {
  it('默认 mutation 失败只弹一条 Toast（标题 + 详情各出现一次）', async () => {
    const { fire, message } = harness(false)
    fire()
    expect(await screen.findAllByText('操作失败')).toHaveLength(1)
    expect(screen.getAllByText(message)).toHaveLength(1)
  })

  it('声明 silentError 的 mutation 失败不弹 Toast（由页面内联展示）', async () => {
    // sonner 的 store 是全局的，上一个用例的 Toast 会在新 Toaster 里复现；用本用例独有的文案断言「没弹」
    const { fire, message } = harness(true)
    fire()
    await new Promise((resolve) => setTimeout(resolve, 50))
    expect(screen.queryByText(message)).toBeNull()
  })
})

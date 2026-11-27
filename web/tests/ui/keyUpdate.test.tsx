import { render, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useUpdateKey, type KeyUpdateInput } from '../../src/api/hooks'

afterEach(() => vi.unstubAllGlobals())

/** 渲染一个只触发 key 更新的探针，返回抓到的请求体列表。 */
function harness(input: KeyUpdateInput) {
  const bodies: Record<string, unknown>[] = []
  vi.stubGlobal('fetch', vi.fn(async (_url: string, init?: RequestInit) => {
    bodies.push(JSON.parse(String(init?.body)))
    return { ok: true, status: 200, json: async () => ({ ok: true, data: { updated: true } }) } as Response
  }))
  const client = new QueryClient()
  let fire = () => {}
  function Probe() {
    const update = useUpdateKey()
    fire = () => void update.mutate(input)
    return null
  }
  render(<QueryClientProvider client={client}><Probe /></QueryClientProvider>)
  return { fire: () => fire(), bodies }
}

describe('useUpdateKey 请求体', () => {
  it('rpm_limit 随 patch 一起发送（此前被静默丢弃）', async () => {
    const { fire, bodies } = harness({ id: 7, rpm_limit: 60 })
    fire()
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toEqual({ rpm_limit: 60 })
  })

  it('只发出现的字段：启停切换不带 weight / rpm_limit', async () => {
    const { fire, bodies } = harness({ id: 7, enabled: false })
    fire()
    await waitFor(() => expect(bodies).toHaveLength(1))
    expect(bodies[0]).toEqual({ enabled: false })
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import Layout from '../../src/components/Layout'
import { getLocale, setLocale } from '../../src/lib/i18n'

let updateData: Record<string, unknown> = { development: true }

vi.mock('../../src/api/hooks', () => ({
  useMe: () => ({ isPending: false, isError: false, data: { username: 'admin', must_change_password: false } }),
  useLogout: () => ({ mutate: vi.fn() }),
  useUpdateStatus: () => ({ isPending: false, isError: false, data: updateData }),
}))

const renderLayout = () =>
  render(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<div>dashboard</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  )

beforeEach(() => setLocale('zh-CN'))
afterEach(() => {
  cleanup()
  updateData = { development: true }
  setLocale('zh-CN')
  localStorage.clear()
})

describe('sidebar footer (T101)', () => {
  it('gives every action row an accessible name', () => {
    renderLayout()
    // jsdom 没有 matchMedia，侧栏按收起态渲染：文字标签不可见，名称只能来自 aria-label
    expect(screen.getByRole('button', { name: '退出登录' })).toBeTruthy()
    expect(screen.getByRole('button', { name: '语言：当前中文，点击切换为 English' })).toBeTruthy()
    expect(screen.getByRole('button', { name: '切换主题：当前跟随系统，点击切换为浅色' })).toBeTruthy()
  })

  it('shows only the username in the account block and keeps the upgrade link when available', () => {
    renderLayout()
    // 版本号由运行概览页头展示，侧栏账号块不重复
    expect(screen.getByText('admin').parentElement?.textContent).toBe('admin')
    expect(screen.queryByRole('link', { name: /获取/ })).toBeNull()

    cleanup()
    updateData = { update_available: true, latest_version: '9.9.9', release_url: 'https://example.com/release' }
    renderLayout()
    expect(screen.getByRole('link', { name: /获取 9\.9\.9/ })).toBeTruthy()
  })

  it('switches locale from the language row and shows the new current value', () => {
    renderLayout()
    fireEvent.click(screen.getByRole('button', { name: '语言：当前中文，点击切换为 English' }))
    expect(getLocale()).toBe('en')
    expect(screen.getByText('Language')).toBeTruthy()
    expect(screen.getByText('English')).toBeTruthy()
  })

  it('cycles theme from the theme row and shows the new current value', () => {
    renderLayout()
    const row = screen.getByRole('button', { name: '切换主题：当前跟随系统，点击切换为浅色' })
    fireEvent.click(row)
    expect(row.getAttribute('aria-label')).toBe('切换主题：当前浅色，点击切换为深色')
    expect(screen.getByText('浅色')).toBeTruthy()
  })
})

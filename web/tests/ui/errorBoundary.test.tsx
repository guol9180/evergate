import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ErrorBoundary } from '../../src/components/ErrorBoundary'
import { t } from '../../src/lib/i18n'

vi.spyOn(console, 'error').mockImplementation(() => {})

/** 一渲染就抛错的组件；armed 为 false 时正常渲染。 */
function Bomb({ armed }: { armed: boolean }) {
  if (armed) throw new Error('渲染炸弹')
  return <p>页面内容</p>
}

describe('渲染异常兜底', () => {
  it('路由级：只替换内容区，显示可恢复的错误页与原因，重试后恢复', () => {
    let setArmed: (armed: boolean) => void = () => {}
    function Probe() {
      const [armed, arm] = useState(true)
      setArmed = arm
      return <ErrorBoundary scope="route"><Bomb armed={armed} /></ErrorBoundary>
    }
    render(<Probe />)
    const alert = screen.getByRole('alert')
    expect(alert.textContent).toContain(t('crash.routeTitle'))
    expect(alert.textContent).toContain('渲染炸弹')
    expect(screen.queryByText('页面内容')).toBeNull()

    setArmed(false)
    fireEvent.click(screen.getByRole('button', { name: t('common.retry') }))
    expect(screen.getByText('页面内容')).toBeTruthy()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('根部：整页兜底使用独立标题', () => {
    render(<ErrorBoundary scope="app"><Bomb armed /></ErrorBoundary>)
    expect(screen.getByRole('alert').textContent).toContain(t('crash.appTitle'))
    expect(screen.getByRole('button', { name: t('crash.reload') })).toBeTruthy()
  })
})

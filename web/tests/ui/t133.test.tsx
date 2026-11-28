import { useState } from 'react'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import { useForm } from 'react-hook-form'
import { DatePicker, DateRangePicker, Field, NumberField, NumberFormField } from '../../src/components/ui'
import ModelPicker from '../../src/components/ModelPicker'
import Tokens from '../../src/pages/Tokens'
import { fmtDateTimeFull, fmtTime } from '../../src/lib/format'
import { t } from '../../src/lib/i18n'

vi.mock('../../src/api/hooks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../src/api/hooks')>()
  return {
    ...actual,
    useTokens: () => ({ data: { items: [] }, isPending: false }),
    useModelCatalog: () => ({ data: { items: [{ model: 'gpt-4o', channels: [{ channel_name: 'openai', upstream_model: 'gpt-4o' }] }] }, isPending: false, isError: false }),
    useCreateToken: () => ({ mutate: vi.fn(), isPending: false, isError: false }),
    useUpdateToken: () => ({ mutate: vi.fn(), isPending: false, isError: false }),
    useDeleteToken: () => ({ mutate: vi.fn(), isPending: false }),
  }
})

describe('NumberField（React Aria）', () => {
  function Harness({ minValue = 1, maxValue = 10, step = 1 }: { minValue?: number; maxValue?: number; step?: number }) {
    const [value, setValue] = useState<number | null>(null)
    return (
      <>
        <NumberField value={value} onChange={setValue} minValue={minValue} maxValue={maxValue} step={step} aria-label="n" />
        <output data-testid="value">{value ?? 'null'}</output>
      </>
    )
  }

  it('输入在失焦时提交为 number，清空提交为 null，不再是字符串换算', () => {
    render(<Harness />)
    const input = screen.getByLabelText('n')
    fireEvent.change(input, { target: { value: '7' } })
    fireEvent.blur(input)
    expect(screen.getByTestId('value').textContent).toBe('7')
    fireEvent.change(input, { target: { value: '' } })
    fireEvent.blur(input)
    expect(screen.getByTestId('value').textContent).toBe('null')
  })

  it('步进按钮按 step 增减并在上下限处禁用；空值点加落到下限，越界值失焦夹回范围', () => {
    render(<Harness />)
    const input = screen.getByLabelText('n')
    const inc = screen.getByRole('button', { name: t('numberField.increment') })
    const dec = screen.getByRole('button', { name: t('numberField.decrement') })
    fireEvent.click(inc)
    expect(screen.getByTestId('value').textContent).toBe('1')
    expect(dec.hasAttribute('disabled')).toBe(true)
    fireEvent.click(inc)
    fireEvent.click(inc)
    expect((input as HTMLInputElement).value).toBe('3')
    fireEvent.change(input, { target: { value: '99' } })
    fireEvent.blur(input)
    expect(screen.getByTestId('value').textContent).toBe('10')
    expect(inc.hasAttribute('disabled')).toBe(true)
    fireEvent.click(inc)
    expect(screen.getByTestId('value').textContent).toBe('10')
  })

  it('整数步长：小数点在真实浏览器里被 onBeforeInput 拦下；绕过拦截塞进来的小数失焦后被丢弃', () => {
    render(<Harness />)
    const input = screen.getByLabelText('n')
    fireEvent.change(input, { target: { value: '2.5' } })
    expect(screen.getByTestId('value').textContent).toBe('null')
    fireEvent.blur(input)
    expect(screen.getByTestId('value').textContent).toBe('null')
  })

  it('NumberFormField 把表单值接进 NumberField，setFocus 能落到输入框', async () => {
    let focus!: (name: 'n') => void
    function Form() {
      const form = useForm<{ n: number | null }>({ defaultValues: { n: 4 } })
      focus = form.setFocus
      return (
        <form>
          <NumberFormField control={form.control} name="n" label="数量" minValue={0} maxValue={100} step={1} />
        </form>
      )
    }
    render(<Form />)
    const input = screen.getByLabelText('数量')
    expect((input as HTMLInputElement).value).toBe('4')
    focus('n')
    await waitFor(() => expect(document.activeElement).toBe(input))
    fireEvent.change(input, { target: { value: '12' } })
    fireEvent.blur(input)
    expect((input as HTMLInputElement).value).toBe('12')
  })

  it('Field 的错误态映射到 aria-invalid', () => {
    render(
      <Field label="数量" error="必须是整数">
        <NumberField value={null} onChange={() => {}} aria-label="数量" />
      </Field>,
    )
    expect(screen.getByLabelText('数量').getAttribute('aria-invalid')).toBe('true')
  })
})

describe('ModelPicker（cmdk 多选）', () => {
  const options = ['alpha', 'beta', 'gamma'].map((model) => ({ publicModel: model, upstreamModel: model }))
  const picker = (props: Partial<React.ComponentProps<typeof ModelPicker>> = {}) => (
    <ModelPicker
      title="上游模型"
      trigger={<button type="button">open</button>}
      open
      onOpenChange={() => {}}
      options={options}
      initialModels={['beta']}
      onApply={() => {}}
      {...props}
    />
  )

  it('目录晚于打开到达时按 initialModels 重算预选', () => {
    const { rerender } = render(picker({ options: [], loading: true }))
    expect(screen.getByText(t('picker.loading'))).toBeTruthy()
    rerender(picker())
    expect(screen.getByRole('option', { name: /beta/ }).getAttribute('aria-checked')).toBe('true')
    expect(screen.getByRole('option', { name: /alpha/ }).getAttribute('aria-checked')).toBe('false')
  })

  it('全选 / 反选作用于当前列表，确认时按排序回传', async () => {
    const onApply = vi.fn()
    render(picker({ onApply }))
    expect(screen.getByText(t('picker.selected').replace('{n}', '1'))).toBeTruthy()
    // cmdk 的过滤结果异步调度，全选要等可见项就位
    await waitFor(() => expect(screen.getByRole('checkbox', { name: t('picker.selectAll') }).hasAttribute('disabled')).toBe(false))
    fireEvent.click(screen.getByText(t('picker.selectAll')))
    expect(screen.getByText(t('picker.selected').replace('{n}', '3'))).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: t('picker.invert') }))
    expect(screen.getByText(t('picker.selected').replace('{n}', '0'))).toBeTruthy()
    expect(screen.getByRole('button', { name: t('picker.confirm') }).hasAttribute('disabled')).toBe(true)
    fireEvent.click(screen.getByText(t('picker.selectAll')))
    fireEvent.click(screen.getByRole('button', { name: t('picker.confirm') }))
    expect(onApply).toHaveBeenCalledWith(['alpha', 'beta', 'gamma'], 'append', false)
  })

  it('搜索框里 Enter 切换当前高亮项（键盘导航由 cmdk 负责），搜索词过滤列表', async () => {
    render(picker())
    const search = screen.getByRole('combobox')
    fireEvent.keyDown(search, { key: 'Enter' })
    expect(screen.getByRole('option', { name: /alpha/ }).getAttribute('aria-checked')).toBe('true')
    fireEvent.change(search, { target: { value: 'gam' } })
    await waitFor(() => expect(screen.queryByRole('option', { name: /alpha/ })).toBeNull())
    expect(screen.getByRole('option', { name: /gamma/ })).toBeTruthy()
  })

  it('令牌弹窗：「从目录选择」展开选择器，再点收起；过期时间走日期选择器', async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter><Tokens /></MemoryRouter>
      </QueryClientProvider>,
    )
    fireEvent.click(screen.getAllByRole('button', { name: t('page.tokens.create') })[0])
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByRole('button', { name: t('tokens.form.expiresAt') })).toBeTruthy()
    const toggle = within(dialog).getByRole('button', { name: t('tokens.form.catalogOpen') })
    fireEvent.click(toggle)
    expect(await screen.findByRole('combobox')).toBeTruthy()
    expect(within(dialog).getByRole('button', { name: t('tokens.form.catalogOpen') }).getAttribute('aria-expanded')).toBe('true')
    fireEvent.click(within(dialog).getByRole('button', { name: t('tokens.form.catalogOpen') }))
    await waitFor(() => expect(screen.queryByRole('combobox')).toBeNull())
  })
})

describe('日期选择器', () => {
  it('DateRangePicker：点选得到正序区间，单日不收起，多日收起，触发器按本地日期显示', () => {
    const onChange = vi.fn()
    function Harness() {
      const [value, setValue] = useState<{ from?: Date; to?: Date } | undefined>()
      return <DateRangePicker value={value} onChange={(next) => { setValue(next); onChange(next) }} />
    }
    render(<Harness />)
    const trigger = screen.getByRole('button', { name: t('datePicker.rangePlaceholder') })
    fireEvent.click(trigger)
    const popover = screen.getByRole('dialog')
    const day = (n: number) => popover.querySelector(`[data-day="${new Date(new Date().getFullYear(), new Date().getMonth(), n).toLocaleDateString('zh-CN')}"]`) as HTMLButtonElement
    fireEvent.click(day(5))
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ from: expect.any(Date), to: expect.any(Date) }))
    const [from, to] = [onChange.mock.lastCall![0].from, onChange.mock.lastCall![0].to] as [Date, Date]
    expect(from.getDate()).toBe(5)
    expect(to.getDate()).toBe(5)
    expect(screen.getByRole('dialog')).toBeTruthy()
    fireEvent.click(day(3))
    const range = onChange.mock.lastCall![0]
    expect(range.from.getDate()).toBe(3)
    expect(range.to.getDate()).toBe(5)
    expect(screen.queryByRole('dialog')).toBeNull()
    const now = new Date()
    const pad = (n: number) => String(n).padStart(2, '0')
    expect(trigger.textContent).toContain(`${pad(now.getMonth() + 1)}-03 – ${pad(now.getMonth() + 1)}-05`)
  })

  it('DatePicker（带时间）：选日保留时刻，时间输入改时刻，清除回 undefined', () => {
    const onChange = vi.fn()
    function Harness() {
      const [value, setValue] = useState<Date | undefined>()
      return <DatePicker value={value} onChange={(next) => { setValue(next); onChange(next) }} withTime />
    }
    render(<Harness />)
    fireEvent.click(screen.getByRole('button', { name: t('datePicker.placeholder') }))
    const popover = screen.getByRole('dialog')
    const day = (n: number) => popover.querySelector(`[data-day="${new Date(new Date().getFullYear(), new Date().getMonth(), n).toLocaleDateString('zh-CN')}"]`) as HTMLButtonElement
    fireEvent.click(day(15))
    const picked = onChange.mock.lastCall![0] as Date
    expect(picked.getDate()).toBe(15)
    fireEvent.change(within(popover).getByLabelText(t('datePicker.time')), { target: { value: '13:45' } })
    const timed = onChange.mock.lastCall![0] as Date
    expect([timed.getDate(), timed.getHours(), timed.getMinutes()]).toEqual([15, 13, 45])
    fireEvent.click(day(16))
    const moved = onChange.mock.lastCall![0] as Date
    expect([moved.getDate(), moved.getHours(), moved.getMinutes()]).toEqual([16, 13, 45])
    fireEvent.click(within(popover).getByRole('button', { name: new RegExp(t('datePicker.clear')) }))
    expect(onChange).toHaveBeenLastCalledWith(undefined)
  })
})

describe('时间列格式（date-fns）', () => {
  it('今天只显时刻，同年显月日，跨年带年份；完整时间给 title', () => {
    const now = new Date()
    expect(fmtTime(now.toISOString())).toMatch(/^\d{2}:\d{2}$/)
    const lastWeek = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 7, 8, 5)
    const sameYear = lastWeek.getFullYear() === now.getFullYear()
    expect(fmtTime(lastWeek.toISOString()).length).toBe(sameYear ? 5 : 10)
    const lastYear = new Date(now.getFullYear() - 1, 0, 2, 8, 5)
    expect(fmtTime(lastYear.toISOString())).toBe(`${now.getFullYear() - 1}-01-02`)
    expect(fmtDateTimeFull(lastYear.toISOString())).toBe(`${now.getFullYear() - 1}-01-02 08:05:00`)
    expect(fmtTime('not-a-date')).toBe('—')
  })
})

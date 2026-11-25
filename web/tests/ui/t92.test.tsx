import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import PriceTable, { fmtPriceMicros } from '../../src/components/PriceTable'
import ModelPicker from '../../src/components/ModelPicker'

describe('PriceTable', () => {
  it('formats USD per million tokens and labels the price source', () => {
    render(<PriceTable prices={[
      { model: 'gpt-4o-mini', input_micros_per_million: 150000, output_micros_per_million: 600000, source: 'catalog' },
      { model: 'unknown-model', input_micros_per_million: 0, output_micros_per_million: 0, source: '' },
    ]} />)
    expect(screen.getByText('gpt-4o-mini')).toBeTruthy()
    expect(screen.getByText('$0.15')).toBeTruthy()
    expect(screen.getByText('$0.60')).toBeTruthy()
    expect(screen.getByText('预设价格')).toBeTruthy()
    expect(screen.getByText('未计价')).toBeTruthy()
  })

  it('keeps two decimals for normal prices and expands only below one cent', () => {
    expect(fmtPriceMicros(2_500_000)).toBe('$2.50')
    expect(fmtPriceMicros(3_000)).toBe('$0.003')
    expect(fmtPriceMicros(0)).toBe('$0.00')
  })
})

describe('ModelPicker prices', () => {
  it('shows upstream prices and writes them as overrides by default', () => {
    const onApply = vi.fn()
    render(<ModelPicker
      title="上游模型"
      options={[{
        publicModel: 'mock-alpha',
        upstreamModel: 'mock-alpha',
        price: { input_micros_per_million: 1_500_000, output_micros_per_million: 6_000_000 },
      }]}
      initialModels={[]}
      onApply={onApply}
      onCancel={() => {}}
    />)
    expect(screen.getByText(/\$1\.50 \/ \$6\.00/)).toBeTruthy()
    fireEvent.click(screen.getByText('mock-alpha'))
    fireEvent.click(screen.getByRole('button', { name: '确认写入' }))
    expect(onApply).toHaveBeenCalledWith(['mock-alpha'], 'append', true)
  })
})

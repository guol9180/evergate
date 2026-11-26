import type { EffectivePrice } from '../api/types'
import { Badge, Td, Th } from './ui'
import { t, tf } from '../lib/i18n'

/** 微美元 / 百万 token → 美元展示；常规价格固定两位小数，低于 $0.01 才展开到 4 位。 */
export function fmtPriceMicros(micros: number): string {
  const usd = micros / 1_000_000
  if (usd <= 0) return '$0.00'
  if (usd >= 0.01) return `$${usd.toFixed(2)}`
  return `$${usd.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}`
}

const sourceTone = { override: 'brand', default: 'neutral', catalog: 'success', '': 'neutral' } as const

/** 价格来源徽标：手工覆盖 / 全局默认 / 预设价格 / 未计价。 */
export function PriceSourceBadge({ source }: { source: EffectivePrice['source'] }) {
  const label = source === 'override' ? t('usage.price.source.override')
    : source === 'default' ? t('usage.price.source.default')
      : source === 'catalog' ? t('usage.price.source.catalog')
        : t('usage.price.source.none')
  return <Badge tone={sourceTone[source]}>{label}</Badge>
}

/** 某渠道各模型生效价格表（管理台价格卡与渠道表单共用）。 */
export default function PriceTable({ prices }: { prices: EffectivePrice[] }) {
  return (
    <div className="overflow-x-auto rounded-lg border border-line">
      <table className="w-full min-w-[240px] text-xs">
        <thead className="bg-surface-soft">
          <tr>
            <Th>{t('usage.price.model')}</Th>
            <Th>{t('usage.price.inputCol')}</Th>
            <Th>{t('usage.price.outputCol')}</Th>
          </tr>
        </thead>
        <tbody className="divide-y divide-line-soft">
          {prices.map((price) => (
            <tr key={price.model}>
              <Td className="font-medium">
                <span className="block whitespace-nowrap">{price.model}</span>
                {/* 来源贴在模型名下，窄卡片里也不必再挤一列 */}
                <span className="mt-1 block" title={price.source_url || undefined}>
                  <PriceSourceBadge source={price.source} />
                </span>
              </Td>
              {/* 缓存价贴在输入价下方：设了才显示，未设的一侧标注「按输入价」 */}
              <Td className="tabular-nums text-muted">
                <span className="block">{price.source ? fmtPriceMicros(price.input_micros_per_million) : '—'}</span>
                {price.source && (price.cache_read_micros_per_million > 0 || price.cache_write_micros_per_million > 0) && (
                  <span className="mt-1 block text-[11px] text-subtle">
                    {t('usage.price.cacheShort')} {cacheSide(price.cache_read_micros_per_million)} · {cacheSide(price.cache_write_micros_per_million)}
                  </span>
                )}
              </Td>
              <Td className="tabular-nums text-muted">{price.source ? fmtPriceMicros(price.output_micros_per_million) : '—'}</Td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** 缓存价一侧的展示：未设置（0）时如实标注按输入价计，避免被读成免费。 */
function cacheSide(micros: number): string {
  return micros > 0 ? fmtPriceMicros(micros) : t('usage.price.cacheFollowInput')
}

/** 选择器里的一行价格说明（输入 / 输出 USD 每百万 token）。 */
export function priceHint(inputMicros: number, outputMicros: number): string {
  return tf('picker.price', { input: fmtPriceMicros(inputMicros), output: fmtPriceMicros(outputMicros) })
}

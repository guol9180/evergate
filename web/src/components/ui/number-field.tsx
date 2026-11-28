import * as React from "react"
import {
  Button as AriaButton,
  Group,
  Input as AriaInput,
  NumberField as AriaNumberField,
  type NumberFieldProps as AriaNumberFieldProps,
} from "react-aria-components"
import { Minus, Plus } from "lucide-react"
import { cn } from "cn"
import { t } from "@/lib/i18n"

export interface NumberFieldProps
  extends Omit<
    AriaNumberFieldProps,
    "value" | "defaultValue" | "onChange" | "children" | "className" | "style" | "isDisabled" | "placeholder"
  > {
  /** 受控值：null 表示空（React Aria 内部用 NaN 表示空，这里不往外泄漏） */
  value: number | null
  onChange: (value: number | null) => void
  disabled?: boolean
  placeholder?: string
  /** 整个字段容器（输入框 + 步进按钮）的类名 */
  className?: string
  size?: "sm" | "md"
  /** 隐藏步进按钮（表格内的紧凑输入）；↑/↓ 仍可步进 */
  hideStepper?: boolean
  /** 由 Field 注入：出错态红边 + 红色聚焦环 */
  "aria-invalid"?: boolean
  ref?: React.Ref<HTMLInputElement>
}

/** 默认整数步长按整数格式化（输入框不接受小数点）；小数步长保留 6 位，覆盖价格 / 秒级超时的精度 */
function defaultFormatOptions(step?: number): Intl.NumberFormatOptions {
  return { useGrouping: false, maximumFractionDigits: step != null && Number.isInteger(step) ? 0 : 6 }
}

/**
 * 数字输入（React Aria NumberField）：步进按钮、↑/↓ 键、上下限夹取与失焦提交都由 React Aria 负责；
 * 表单拿到的是 number / null，不再做字符串换算。视觉与 Input 同规格。
 */
export function NumberField({
  value,
  onChange,
  className,
  size = "md",
  hideStepper = false,
  formatOptions,
  step,
  isInvalid,
  disabled,
  placeholder,
  "aria-invalid": ariaInvalid,
  ref,
  ...props
}: NumberFieldProps) {
  const compact = size === "sm"
  return (
    <AriaNumberField
      {...props}
      step={step}
      isDisabled={disabled}
      // 关掉原生校验：表单校验由 react-hook-form + zod 负责，这里只要 aria 状态
      validationBehavior="aria"
      isWheelDisabled
      value={value ?? Number.NaN}
      onChange={(next) => onChange(Number.isNaN(next) ? null : next)}
      formatOptions={formatOptions ?? defaultFormatOptions(step)}
      isInvalid={ariaInvalid || isInvalid}
      incrementAriaLabel={t("numberField.increment")}
      decrementAriaLabel={t("numberField.decrement")}
      className={cn("group/number-field", className)}
    >
      <Group
        className={cn(
          "flex w-full items-center overflow-hidden rounded-md border border-line-strong bg-surface transition-colors data-disabled:bg-surface-strong data-disabled:text-subtle focus-within:border-brand focus-within:ring-2 focus-within:ring-brand-subtle data-invalid:border-danger-solid data-invalid:focus-within:border-danger-solid data-invalid:focus-within:ring-danger-subtle",
          compact ? "h-7" : "h-9"
        )}
      >
        <AriaInput
          ref={ref}
          placeholder={placeholder}
          className={cn(
            "h-full min-w-0 flex-1 bg-transparent px-3 text-foreground outline-none placeholder:text-subtle",
            compact ? "text-xs" : "text-sm"
          )}
        />
        {!hideStepper && (
          <div className="flex h-full shrink-0 items-stretch border-l border-line-soft">
            <AriaButton
              slot="decrement"
              className="flex w-7 items-center justify-center text-muted-foreground transition-colors outline-none hover:bg-surface-soft hover:text-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand disabled:pointer-events-none disabled:opacity-30 [&_svg]:size-3.5"
            >
              <Minus aria-hidden="true" />
            </AriaButton>
            <AriaButton
              slot="increment"
              className="flex w-7 items-center justify-center text-muted-foreground transition-colors outline-none hover:bg-surface-soft hover:text-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand disabled:pointer-events-none disabled:opacity-30 [&_svg]:size-3.5"
            >
              <Plus aria-hidden="true" />
            </AriaButton>
          </div>
        )}
      </Group>
    </AriaNumberField>
  )
}

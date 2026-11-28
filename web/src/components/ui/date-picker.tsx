import * as React from "react"
import { format, isSameDay, isSameYear } from "date-fns"
import type { DateRange, Labels, Matcher } from "react-day-picker"
import { CalendarDays, Clock, X } from "lucide-react"
import { cn } from "cn"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import { Input } from "@/components/ui/input"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { dateFnsLocale } from "@/lib/format"
import { t } from "@/lib/i18n"
import { useMediaQuery } from "@/lib/useMediaQuery"

// 弹层与 Select 同层级（z-[60]，浮在 Dialog 之上）；不设固定宽度，由日历撑开。
// 矮视口（两侧都放不下整月）时按 Radix 给出的可用高度限高并内部滚动，不越过视口
const contentClass = "z-[60] w-auto max-h-[var(--radix-popover-content-available-height)] overflow-y-auto p-0"
// 触发器与 Input 同规格（h-9 / lg 尺寸），左对齐；空值弱化色
const triggerClass = "h-9 w-full justify-start px-3 font-normal"

interface PickerAriaProps {
  id?: string
  className?: string
  disabled?: boolean
  "aria-label"?: string
  "aria-labelledby"?: string
  "aria-describedby"?: string
  "aria-invalid"?: boolean
}

/** 日历读屏文案跟随界面语言（react-day-picker 默认是英文） */
const labels: Partial<Labels> = {
  labelPrevious: () => t("datePicker.prevMonth"),
  labelNext: () => t("datePicker.nextMonth"),
}

function dayDisabled(min?: Date, max?: Date): Matcher[] | undefined {
  const matchers: Matcher[] = []
  if (min) matchers.push({ before: min })
  if (max) matchers.push({ after: max })
  return matchers.length ? matchers : undefined
}

export interface DatePickerProps extends PickerAriaProps {
  value: Date | undefined
  onChange: (value: Date | undefined) => void
  placeholder?: string
  /** 带时间（分钟精度）：日历下方显示时间输入与清除；不带时间则选中即收起 */
  withTime?: boolean
  min?: Date
  max?: Date
}

/** 单日选择（Popover + Calendar）；带时间时换日保留已选时刻。 */
export function DatePicker({
  value,
  onChange,
  placeholder = t("datePicker.placeholder"),
  withTime = false,
  min,
  max,
  id,
  className,
  disabled,
  "aria-label": ariaLabel,
  "aria-labelledby": ariaLabelledBy,
  "aria-describedby": ariaDescribedBy,
  "aria-invalid": ariaInvalid,
}: DatePickerProps) {
  const [open, setOpen] = React.useState(false)
  const timeText = value ? format(value, "HH:mm") : ""
  const pickDay = (day: Date | undefined) => {
    if (!day) {
      onChange(undefined)
      return
    }
    const next = value
      ? new Date(day.getFullYear(), day.getMonth(), day.getDate(), value.getHours(), value.getMinutes())
      : day
    onChange(next)
    if (!withTime) setOpen(false)
  }
  const pickTime = (text: string) => {
    const match = /^(\d{2}):(\d{2})$/.exec(text)
    if (!value || !match) return
    const next = new Date(value)
    next.setHours(Number(match[1]), Number(match[2]), 0, 0)
    onChange(next)
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="lg"
          id={id}
          disabled={disabled}
          aria-label={ariaLabel}
          aria-labelledby={ariaLabelledBy}
          aria-describedby={ariaDescribedBy}
          aria-invalid={ariaInvalid || undefined}
          className={cn(triggerClass, !value && "text-muted-foreground", className)}
        >
          <CalendarDays aria-hidden="true" />
          {value ? (withTime ? format(value, "yyyy-MM-dd HH:mm") : format(value, "yyyy-MM-dd")) : placeholder}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" sideOffset={6} collisionPadding={8} className={contentClass}>
        <Calendar
          mode="single"
          selected={value}
          onSelect={pickDay}
          disabled={dayDisabled(min, max)}
          defaultMonth={value}
          autoFocus
          locale={dateFnsLocale()}
          labels={labels}
        />
        {withTime && (
          <div className="flex items-center gap-2 border-t border-line px-3 py-2">
            <Clock size={14} className="shrink-0 text-subtle" aria-hidden="true" />
            <Input
              type="time"
              step={60}
              value={timeText}
              disabled={!value || disabled}
              onChange={(event) => pickTime(event.target.value)}
              aria-label={t("datePicker.time")}
              className="h-8 w-32"
            />
            {value && (
              <Button type="button" size="sm" variant="ghost" className="ml-auto" onClick={() => onChange(undefined)}>
                <X /> {t("datePicker.clear")}
              </Button>
            )}
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

export interface DateRangePickerProps extends PickerAriaProps {
  value: DateRange | undefined
  onChange: (value: DateRange | undefined) => void
  placeholder?: string
  min?: Date
  max?: Date
  /** 触发器下方的清除入口（有值时显示） */
  clearable?: boolean
}

/**
 * 日期范围（Popover + range Calendar）：起点晚于终点在选择器结构上不可能——点在起点之前的日子会重新起算。
 * 首次点选即得到单日区间并回写；再点一天扩展成区间后收起。
 */
export function DateRangePicker({
  value,
  onChange,
  placeholder = t("datePicker.rangePlaceholder"),
  min,
  max,
  id,
  className,
  disabled,
  clearable = false,
  "aria-label": ariaLabel,
  "aria-labelledby": ariaLabelledBy,
  "aria-describedby": ariaDescribedBy,
  "aria-invalid": ariaInvalid,
}: DateRangePickerProps) {
  const [open, setOpen] = React.useState(false)
  const twoMonths = useMediaQuery("(min-width: 900px)")
  const pick = (range: DateRange | undefined) => {
    onChange(range)
    // 单日区间保持展开，方便再点一天扩展；多日区间成型即收起
    if (range?.from && range.to && !isSameDay(range.from, range.to)) setOpen(false)
  }
  const dayText = (day: Date) => (value?.from && isSameYear(value.from, day) ? format(day, "MM-dd") : format(day, "yyyy-MM-dd"))
  const text = value?.from ? (value.to && !isSameDay(value.from, value.to) ? `${dayText(value.from)} – ${dayText(value.to)}` : dayText(value.from)) : ""
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="lg"
          id={id}
          disabled={disabled}
          aria-label={ariaLabel}
          aria-labelledby={ariaLabelledBy}
          aria-describedby={ariaDescribedBy}
          aria-invalid={ariaInvalid || undefined}
          className={cn(triggerClass, !text && "text-muted-foreground", className)}
        >
          <CalendarDays aria-hidden="true" />
          {text || placeholder}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" sideOffset={6} collisionPadding={8} className={contentClass}>
        <Calendar
          mode="range"
          selected={value}
          onSelect={pick}
          numberOfMonths={twoMonths ? 2 : 1}
          disabled={dayDisabled(min, max)}
          defaultMonth={value?.from}
          autoFocus
          locale={dateFnsLocale()}
          labels={labels}
        />
        {clearable && value && (
          <div className="flex justify-end border-t border-line px-3 py-2">
            <Button type="button" size="sm" variant="ghost" onClick={() => pick(undefined)}>
              <X /> {t("datePicker.clear")}
            </Button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

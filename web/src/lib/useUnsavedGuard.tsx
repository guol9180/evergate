import { useRef, useState, type ReactNode } from 'react'
import { ConfirmDialog } from '../components/ui'
import { t } from './i18n'

/**
 * 未保存更改保护：把关闭动作（Esc / 点遮罩 / 关闭按钮）包一层——有脏数据时先确认，确认后才真正关闭。
 * 用法：const guard = useUnsavedGuard(formState.isDirty)；<Dialog onClose={guard(close)}>{guard.dialog}</Dialog>
 */
export function useUnsavedGuard(dirty: boolean) {
  const [open, setOpen] = useState(false)
  const pending = useRef<(() => void) | null>(null)
  const confirm = () => {
    setOpen(false)
    const close = pending.current
    pending.current = null
    close?.()
  }
  return {
    guard: (close: () => void) => () => {
      if (!dirty) {
        close()
        return
      }
      pending.current = close
      setOpen(true)
    },
    dialog: (
      <ConfirmDialog
        open={open}
        title={t('unsaved.title')}
        message={t('unsaved.message')}
        danger
        confirmText={t('unsaved.discard')}
        cancelText={t('common.cancel')}
        onConfirm={confirm}
        onCancel={() => setOpen(false)}
      />
    ) as ReactNode,
  }
}
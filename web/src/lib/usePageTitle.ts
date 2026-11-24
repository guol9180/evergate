import { useEffect } from 'react'
import { useLocale } from './i18n'

/** 设置 document.title 为「<页面名> · EverGate」，多标签页可区分；订阅语言切换以便标题同步。 */
export function usePageTitle(title: string) {
  useLocale()
  useEffect(() => {
    document.title = `${title} · EverGate`
  }, [title])
}

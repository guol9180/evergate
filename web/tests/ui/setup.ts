import { afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// 用例按中文字典断言；jsdom 的 navigator.language 是 en-US，先固定界面语言，避免首访检测把字典切成英文
window.localStorage.setItem('eg-locale', 'zh-CN')

// jsdom 缺失的浏览器 API：Radix Select / Popper 打开弹层时会调用，没有桩会直接抛错
if (!Element.prototype.scrollIntoView) {
  Element.prototype.scrollIntoView = () => {}
}
if (!Element.prototype.hasPointerCapture) {
  Element.prototype.hasPointerCapture = () => false
  Element.prototype.releasePointerCapture = () => {}
}
if (typeof globalThis.ResizeObserver === 'undefined') {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  } as unknown as typeof ResizeObserver
}

afterEach(() => cleanup())

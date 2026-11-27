#!/usr/bin/env node
import { spawn } from 'node:child_process'
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { once } from 'node:events'
import http from 'node:http'
import os from 'node:os'
import path from 'node:path'
import process from 'node:process'

const args = new Map()
for (let i = 2; i < process.argv.length;) {
  const key = process.argv[i]
  if (key === '--seed') {
    args.set(key, true)
    i++
  } else {
    args.set(key, process.argv[i + 1])
    i += 2
  }
}

const baseURL = (args.get('--url') || 'http://127.0.0.1:8787').replace(/\/$/, '')
const chrome = args.get('--chrome') || 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'
const screenshotDir = args.get('--screenshots')
const seed = args.has('--seed')
const widthArg = args.get('--widths')
const themeArg = args.get('--theme') || 'system'
const localeArg = args.get('--locale') || 'zh'
const username = process.env.EG_AUDIT_USER || 'admin'
// 初始密码自 T75 起为随机生成：审计前请以 EVERGATE_INITIAL_ADMIN_PASSWORD 启动被测服务，或在此处指定 EG_AUDIT_PASSWORD。
let password = process.env.EG_AUDIT_PASSWORD || 'admin123'
const nextPassword = process.env.EG_AUDIT_NEW_PASSWORD
const widths = widthArg
  ? widthArg.split(',').map((value) => Number(value.trim())).filter((value) => Number.isFinite(value) && value > 0)
  : [360, 390, 768, 1024, 1280, 1440, 1600, 1920, 2560]
const screenshotWidths = new Set([390, 768, 907, 1280, 1440, 1920])
const pages = ['/', '/channels', '/tokens', '/logs', '/playground', '/usage', '/settings']

let mockServer
let chromeProcess
let profileDir
let cdp

try {
  const sessionCookie = await login()
  if (seed) {
    await seedData(sessionCookie)
    await new Promise((resolve) => setTimeout(resolve, 1200))
  }

  const chromePort = await freePort()
  profileDir = await mkdtemp(path.join(os.tmpdir(), 'evergate-layout-audit-'))
  chromeProcess = spawn(chrome, [
    '--headless=new',
    `--remote-debugging-port=${chromePort}`,
    `--user-data-dir=${profileDir}`,
    '--no-first-run',
    '--disable-gpu',
    '--no-sandbox',
    'about:blank',
  ], { stdio: 'ignore', windowsHide: true })

  const target = await waitForTarget(chromePort)
  cdp = await connectCDP(target.webSocketDebuggerUrl)
  await cdp.send('Page.enable')
  await cdp.send('Runtime.enable')
  await cdp.send('Network.enable')
  await cdp.send('Network.setCookie', {
    name: 'evergate_session',
    value: sessionCookie.split('=').slice(1).join('='),
    url: baseURL,
    path: '/',
  })
  // 主题 / 语言存在 localStorage：在文档脚本执行前写入，保证首帧就是目标变体
  await cdp.send('Page.addScriptToEvaluateOnNewDocument', {
    source: `try { localStorage.setItem('eg-theme', ${JSON.stringify(themeArg)}); localStorage.setItem('eg-locale', ${JSON.stringify(localeArg === 'en' ? 'en' : 'zh-CN')}) } catch (error) {}`,
  })

  if (screenshotDir) await mkdir(screenshotDir, { recursive: true })
  const results = []
  const failures = []
  for (const width of widths) {
    await cdp.send('Emulation.setDeviceMetricsOverride', {
      width, height: 900, deviceScaleFactor: 1, mobile: width < 768,
    })
    for (const route of pages) {
      const loaded = cdp.waitEvent('Page.loadEventFired')
      await cdp.send('Page.navigate', { url: baseURL + route })
      await loaded
      // 懒加载路由（Usage）与数据请求需要时间：轮询 main 有内容后再测，避免把骨架屏当成通过
      for (let attempt = 0; attempt < 12; attempt++) {
        const ready = await evaluate(`(() => { const main = document.querySelector('main'); return !!main && main.innerText.trim().length > 30 })()`)
        if (ready) break
        await new Promise((resolve) => setTimeout(resolve, 200))
      }
      await new Promise((resolve) => setTimeout(resolve, 300))
      const metrics = await evaluate(`(() => {
        const root = document.documentElement;
        const tables = [...document.querySelectorAll('table')].map((table) => {
          const container = table.closest('.overflow-x-auto');
          return {
            minWidth: getComputedStyle(table).minWidth,
            containerOverflow: container ? container.scrollWidth - container.clientWidth : 0,
            tableOverflow: container ? 0 : table.scrollWidth - table.clientWidth,
          };
        });
        const sidebar = document.querySelector('aside');
        // 上游返回的原始错误文本按数据展示，不参与"英文界面无中文"判定
        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
        let bodyText = '';
        while (walker.nextNode()) {
          const node = walker.currentNode;
          if (node.parentElement?.closest('[data-upstream]')) continue;
          bodyText += (node.nodeValue || '') + '\\n';
        }
        const cjk = (bodyText.match(/[\u4e00-\u9fff]/g) || []).length;
        const cjkSamples = (bodyText.match(/.{0,10}[\u4e00-\u9fff]+.{0,10}/g) || []).slice(0, 4);
        return {
          innerWidth,
          docClientWidth: root.clientWidth,
          docScrollWidth: root.scrollWidth,
          bodyScrollWidth: document.body.scrollWidth,
          sidebarWidth: sidebar ? Math.round(sidebar.getBoundingClientRect().width * 10) / 10 : 0,
          cjk,
          cjkSamples,
          tables,
        };
      })()`)
      const row = { width, route, ...metrics }
      results.push(row)
      if (row.docScrollWidth > row.docClientWidth + 1 || row.bodyScrollWidth > row.docClientWidth + 1) {
        failures.push(`${width}px ${route}: page horizontal overflow ${row.docScrollWidth}/${row.docClientWidth}`)
      }
      if (width >= 900 && row.tables.some((table) => table.containerOverflow > 1 || table.tableOverflow > 1)) {
        failures.push(`${width}px ${route}: table overflow ${JSON.stringify(row.tables)}`)
      }
      if (localeArg === 'en' && row.cjk > 0) {
        failures.push(`${width}px ${route}: ${row.cjk} CJK characters left in English mode ${JSON.stringify(row.cjkSamples)}`)
      }
      if (screenshotDir && screenshotWidths.has(width)) {
        const shot = await cdp.send('Page.captureScreenshot', { format: 'png', fromSurface: true })
        const name = `${width}-${route === '/' ? 'dashboard' : route.slice(1)}-${themeArg}-${localeArg}.png`
        await writeFile(path.join(screenshotDir, name), Buffer.from(shot.data, 'base64'))
      }
    }
  }

  for (let i = 1; i < results.length; i++) {
    const prev = results[i - 1]
    const current = results[i]
    if (prev.route === current.route && current.width > prev.width && current.sidebarWidth < prev.sidebarWidth) {
      failures.push(`${current.route}: sidebar width regressed ${prev.width}->${current.width}`)
    }
  }
  if (failures.length) {
    console.error(failures.join('\n'))
    process.exitCode = 1
  } else {
    console.log(`layout-audit: ${widths.length} widths x ${pages.length} pages passed`)
  }
} finally {
  cdp?.close()
  chromeProcess?.kill()
  mockServer?.close()
  if (chromeProcess && chromeProcess.exitCode === null) {
    await Promise.race([
      once(chromeProcess, 'exit'),
      new Promise((resolve) => setTimeout(resolve, 2000)),
    ])
  }
  if (profileDir) {
    for (let i = 0; i < 10; i++) {
      try {
        await rm(profileDir, { recursive: true, force: true })
        break
      } catch (error) {
        // Chrome 拼写词典 (.bdic) 在高负载下可能延迟释放：审计结论已出，临时目录留给系统清理，不让清理失败盖掉结果
        if (error?.code !== 'EBUSY' || i === 9) {
          if (error?.code === 'EBUSY') console.warn(`layout-audit: temp profile left behind (${profileDir})`)
          else throw error
          break
        }
        await new Promise((resolve) => setTimeout(resolve, 500))
      }
    }
  }
}

async function login() {
  const response = await fetch(`${baseURL}/admin/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const text = await response.text()
  if (!response.ok) throw new Error(`login failed: ${response.status} ${text}`)
  const payload = JSON.parse(text)
  const cookie = cookieValue(response.headers)
  if (payload?.data?.must_change_password) {
    if (!nextPassword) throw new Error('first login requires EG_AUDIT_NEW_PASSWORD')
    const changed = await fetch(`${baseURL}/admin/api/auth/password`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Cookie: cookie },
      body: JSON.stringify({ old_password: password, new_password: nextPassword }),
    })
    if (!changed.ok) throw new Error(`password change failed: ${changed.status} ${await changed.text()}`)
    password = nextPassword
    return login()
  }
  return cookie
}

async function seedData(cookie) {
  mockServer = http.createServer((req, res) => {
    req.resume()
    res.writeHead(200, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({
      id: 'chatcmpl-layout-audit',
      object: 'chat.completion',
      choices: [{ index: 0, message: { role: 'assistant', content: 'layout audit' }, finish_reason: 'stop' }],
      usage: { prompt_tokens: 3, completion_tokens: 2, total_tokens: 5 },
    }))
  })
  await new Promise((resolve) => mockServer.listen(0, '127.0.0.1', resolve))
  const mockURL = `http://127.0.0.1:${mockServer.address().port}`
  const suffix = Date.now().toString(36)
  const channel = await adminRequest(cookie, '/admin/api/channels', 'POST', {
    name: `layout-audit-${suffix}`,
    type: 'openai-compatible',
    base_url: mockURL,
    models: ['layout-audit-model'],
    priority: 0,
    weight: 1,
  })
  await adminRequest(cookie, `/admin/api/channels/${channel.id}/keys`, 'POST', { keys: `sk-layout-${suffix}` })
  const token = await adminRequest(cookie, '/admin/api/tokens', 'POST', { name: `layout-audit-${suffix}` })
  for (let i = 0; i < 3; i++) {
    const response = await fetch(`${baseURL}/v1/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token.key}` },
      body: JSON.stringify({ model: 'layout-audit-model', messages: [{ role: 'user', content: 'hello' }] }),
    })
    if (!response.ok) throw new Error(`seed chat failed: ${response.status} ${await response.text()}`)
  }
}

async function adminRequest(cookie, route, method, body) {
  const response = await fetch(baseURL + route, {
    method,
    headers: { 'Content-Type': 'application/json', Cookie: cookie },
    body: body ? JSON.stringify(body) : undefined,
  })
  const payload = await response.json()
  if (!response.ok || payload.ok === false) throw new Error(`${route}: ${response.status} ${JSON.stringify(payload)}`)
  return payload.data
}

function cookieValue(headers) {
  const raw = headers.getSetCookie?.()[0] || headers.get('set-cookie') || ''
  const pair = raw.split(';', 1)[0]
  if (!pair.startsWith('evergate_session=')) throw new Error('login did not return evergate_session cookie')
  return pair
}

async function freePort() {
  const server = http.createServer()
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve))
  const port = server.address().port
  await new Promise((resolve) => server.close(resolve))
  return port
}

async function waitForTarget(port) {
  const deadline = Date.now() + 15_000
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, { method: 'PUT' })
      if (response.ok) return response.json()
    } catch {}
    await new Promise((resolve) => setTimeout(resolve, 200))
  }
  throw new Error('Chrome DevTools endpoint did not become ready')
}

async function connectCDP(wsURL) {
  const socket = new WebSocket(wsURL)
  const pending = new Map()
  const waiters = new Map()
  let nextId = 1
  await new Promise((resolve, reject) => {
    socket.addEventListener('open', resolve, { once: true })
    socket.addEventListener('error', reject, { once: true })
  })
  socket.addEventListener('message', (event) => {
    const message = JSON.parse(event.data)
    if (message.id) {
      const waiter = pending.get(message.id)
      if (!waiter) return
      pending.delete(message.id)
      if (message.error) waiter.reject(new Error(message.error.message))
      else waiter.resolve(message.result)
      return
    }
    const queue = waiters.get(message.method)
    if (queue?.length) queue.shift()(message.params)
  })
  return {
    send(method, params = {}) {
      const id = nextId++
      socket.send(JSON.stringify({ id, method, params }))
      return new Promise((resolve, reject) => pending.set(id, { resolve, reject }))
    },
    waitEvent(method) {
      return new Promise((resolve) => {
        const queue = waiters.get(method) || []
        queue.push(resolve)
        waiters.set(method, queue)
      })
    },
    close() {
      socket.close()
    },
  }
}

async function evaluate(expression) {
  const result = await cdp.send('Runtime.evaluate', {
    expression,
    returnByValue: true,
    awaitPromise: true,
  })
  if (result.exceptionDetails) {
    throw new Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text || 'page evaluation failed')
  }
  return result.result.value
}

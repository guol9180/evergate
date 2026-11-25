/** 复制文本到剪贴板；Clipboard API 不可用或失败时回退 execCommand，返回是否成功。 */
export async function copyText(text: string): Promise<boolean> {
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 非安全上下文（http://局域网 IP）或权限被拒时走回退
    }
  }
  try {
    const area = document.createElement('textarea')
    area.value = text
    area.setAttribute('readonly', '')
    area.style.position = 'fixed'
    area.style.opacity = '0'
    document.body.appendChild(area)
    area.select()
    const ok = document.execCommand('copy')
    area.remove()
    return ok
  } catch {
    return false
  }
}

/** 生成常见客户端的环境变量片段；baseURL 为空时用当前页面 origin 推断。 */
export function clientEnvSnippet(key: string, baseURL: string): string {
  const root = baseURL.replace(/\/v1\/?$/, '')
  return [
    `OPENAI_BASE_URL=${baseURL}`,
    `OPENAI_API_KEY=${key}`,
    '',
    `ANTHROPIC_BASE_URL=${root}`,
    `ANTHROPIC_AUTH_TOKEN=${key}`,
    '',
    `export EVERGATE_BASE_URL="${baseURL}" EVERGATE_API_KEY="${key}"`,
  ].join('\n')
}

/** 三协议「一键获取模型列表」的 curl 示例；复用上面的 EVERGATE_* 变量，避免重复暴露明文 key。 */
export function modelListSnippets(baseURL: string): string {
  const root = baseURL.replace(/\/v1\/?$/, '')
  return [
    '# OpenAI / OpenAI-compatible',
    'curl "$EVERGATE_BASE_URL/models" -H "Authorization: Bearer $EVERGATE_API_KEY"',
    '',
    '# Anthropic Messages',
    'curl "$EVERGATE_BASE_URL/models" -H "x-api-key: $EVERGATE_API_KEY" -H "anthropic-version: 2023-06-01"',
    '',
    '# Gemini API',
    `curl "${root}/v1beta/models" -H "x-goog-api-key: $EVERGATE_API_KEY"`,
  ].join('\n')
}

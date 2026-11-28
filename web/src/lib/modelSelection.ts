export type ModelSelectionMode = 'append' | 'replace'

/**
 * 把选择器确认的模型合并进当前列表。
 * replace 只替换「目录里有的」模型：手填的、目录没返回的模型保留，不被一次替换清空。
 * available 为目录可选项；缺省时退化为整体替换。
 */
export function applyModelSelection(current: string[], selected: string[], mode: ModelSelectionMode, available?: readonly string[]): string[] {
  if (mode === 'replace') {
    if (!available) return [...new Set(selected)]
    const catalog = new Set(available)
    return [...new Set([...current.filter((model) => !catalog.has(model)), ...selected])]
  }
  return [...new Set([...current, ...selected])]
}

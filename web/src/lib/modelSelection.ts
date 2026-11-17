export type ModelSelectionMode = 'append' | 'replace'

export function applyModelSelection(current: string[], selected: string[], mode: ModelSelectionMode): string[] {
  if (mode === 'replace') return [...new Set(selected)]
  return [...new Set([...current, ...selected])]
}

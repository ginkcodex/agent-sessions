export type TabCloseAction = 'close' | 'others' | 'right' | 'left';

export const TAB_CLOSE_ACTIONS: { action: TabCloseAction; label: string }[] = [
  { action: 'close', label: 'Close' },
  { action: 'others', label: 'Close Others' },
  { action: 'right', label: 'Close Right' },
  { action: 'left', label: 'Close Left' },
];

export function tabsToClose(ids: string[], target: string, action: TabCloseAction): string[] {
  const index = ids.indexOf(target);
  if (index < 0) return [];
  if (action === 'close') return [target];
  if (action === 'others') return ids.filter((id) => id !== target);
  return action === 'right' ? ids.slice(index + 1) : ids.slice(0, index);
}

/**
 * The file tab a key selects, counting from 1: Cmd+1–9 on macOS, Ctrl+1–9
 * elsewhere. The key's position is read, so layouts that type other
 * characters on the digit row work too. Null for any other key.
 */
export function tabShortcut(
  e: Pick<KeyboardEvent, 'code' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>,
  mac: boolean,
): number | null {
  const chord = mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
  if (!chord || e.altKey || e.shiftKey) return null;
  const match = /^(?:Digit|Numpad)([1-9])$/.exec(e.code);
  return match ? Number(match[1]) : null;
}

/**
 * Where Ctrl+Shift+PageUp or PageDown moves the shown tab, as in VS Code on
 * every platform: -1 left, 1 right. Null for any other key.
 */
export function tabMoveShortcut(e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>): -1 | 1 | null {
  if (!e.ctrlKey || !e.shiftKey || e.metaKey || e.altKey) return null;
  return e.key === 'PageUp' ? -1 : e.key === 'PageDown' ? 1 : null;
}

/**
 * The gap a pointer at x points to, between tabs whose horizontal extents
 * are given in order: 0 before the first, tabs.length after the last.
 */
export function gapAt(x: number, tabs: { left: number; width: number }[]): number {
  const gap = tabs.findIndex((t) => x < t.left + t.width / 2);
  return gap < 0 ? tabs.length : gap;
}

/** The position a tab at from takes when dropped in gap; null when it stays put. */
export function dropIndex(from: number, gap: number): number | null {
  const to = gap > from ? gap - 1 : gap;
  return to === from ? null : to;
}

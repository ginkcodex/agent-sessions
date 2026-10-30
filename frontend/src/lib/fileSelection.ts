export interface FileLineSelection {
  anchor: number;
  head: number;
}

export function selectFileLine(current: FileLineSelection | undefined, line: number, extend: boolean): FileLineSelection {
  return { anchor: extend && current ? current.anchor : line, head: line };
}

/** Git counts newline-terminated lines, not the editor's extra empty row at EOF. */
export function isBlameableLine(lines: readonly string[], line: number): boolean {
  const count = lines.length - (lines.at(-1) === '' ? 1 : 0);
  return Number.isInteger(line) && line > 0 && line <= count;
}

export function includesFileLine(selection: FileLineSelection | undefined, line: number): boolean {
  return !!selection && line >= Math.min(selection.anchor, selection.head) && line <= Math.max(selection.anchor, selection.head);
}

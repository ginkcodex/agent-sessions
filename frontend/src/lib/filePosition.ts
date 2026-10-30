/** A source line and its offset from the top of the viewport, independent of wrapping. */
export interface FilePosition {
  line: number;
  offset: number;
  left: number;
}

/** Source lines are in document order, including when long lines wrap. */
export function firstVisibleSourceLine(lines: ArrayLike<{ getBoundingClientRect(): { bottom: number } }>, top: number): number {
  let lo = 0, hi = lines.length;
  while (lo < hi) {
    const mid = (lo + hi) >>> 1;
    if (lines[mid].getBoundingClientRect().bottom <= top) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}

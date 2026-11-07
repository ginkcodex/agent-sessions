// Where a scrolled view is, by one of its rows (a message, a source line, a
// Markdown block), so it can be put back after its text changes size
// (Settings → Appearance). WebKit has no CSS scroll anchoring: without this
// the view keeps its pixel offset and other text slides under it.

export interface RowAnchor {
  /** The row's top, in px from the top of the view; negative when it starts above. */
  offset: number;
  /** Its height when taken. */
  height: number;
}

/** Where the row is now, relative to the top of the view. */
export function rowAnchor(container: HTMLElement, row: HTMLElement): RowAnchor {
  const rect = row.getBoundingClientRect();
  return { offset: rect.top - container.getBoundingClientRect().top, height: rect.height };
}

/** The first row whose bottom is below the top of the view. */
export function firstVisibleRow(container: HTMLElement, rows: Iterable<HTMLElement>): HTMLElement | null {
  const top = container.getBoundingClientRect().top;
  for (const row of rows) if (row.getBoundingClientRect().bottom > top) return row;
  return null;
}

/**
 * Scrolls the row back to where the anchor was. A row that started in view
 * keeps its top in place; one that started above keeps the same share of it
 * above the view, as its text grew or shrank with the rest.
 */
export function restoreRow(container: HTMLElement, row: HTMLElement, anchor: RowAnchor): void {
  const rect = row.getBoundingClientRect();
  const offset = anchor.offset >= 0 || anchor.height <= 0 ? anchor.offset : anchor.offset * (rect.height / anchor.height);
  container.scrollTop += rect.top - container.getBoundingClientRect().top - offset;
}

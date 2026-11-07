// Where a scrolled transcript is, by message, so it can be put back after
// its text changes size (Settings → Appearance). Messages are found again by
// index, not by element.
import { firstVisibleRow, restoreRow, rowAnchor, type RowAnchor } from './scrollAnchor';

export interface MessageAnchor extends RowAnchor {
  /** The message's `data-message-index`. */
  index: number;
}

/** The first message whose bottom is below the top of the view. */
export function firstVisibleMessage(container: HTMLElement): MessageAnchor | null {
  const row = firstVisibleRow(container, container.querySelectorAll<HTMLElement>('[data-message-index]'));
  return row ? { index: Number(row.dataset.messageIndex), ...rowAnchor(container, row) } : null;
}

/** Scrolls the message back to where it was (see `restoreRow`). Returns false when it is no longer shown. */
export function restoreMessage(container: HTMLElement, anchor: MessageAnchor): boolean {
  const row = container.querySelector<HTMLElement>(`[data-message-index="${anchor.index}"]`);
  if (!row) return false;
  restoreRow(container, row, anchor);
  return true;
}

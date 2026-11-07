// Where a scrolled transcript is, by message, so it can be put back after
// its text changes size (Settings → Appearance).

export interface MessageAnchor {
  /** The message's `data-message-index`. */
  index: number;
  /** Its top, in px from the top of the view; negative when it starts above. */
  offset: number;
  /** Its height when taken. */
  height: number;
}

/** The first message whose bottom is below the top of the view. */
export function firstVisibleMessage(container: HTMLElement): MessageAnchor | null {
  const top = container.getBoundingClientRect().top;
  for (const row of container.querySelectorAll<HTMLElement>('[data-message-index]')) {
    const rect = row.getBoundingClientRect();
    if (rect.bottom > top) return { index: Number(row.dataset.messageIndex), offset: rect.top - top, height: rect.height };
  }
  return null;
}

/**
 * Scrolls the message back to where it was. A message that started in view
 * keeps its top in place; one that started above keeps the same share of it
 * above the view, as its text grew or shrank with the rest. Returns false
 * when the message is no longer shown.
 */
export function restoreMessage(container: HTMLElement, anchor: MessageAnchor): boolean {
  const row = container.querySelector<HTMLElement>(`[data-message-index="${anchor.index}"]`);
  if (!row) return false;
  const rect = row.getBoundingClientRect();
  const offset = anchor.offset >= 0 || anchor.height <= 0 ? anchor.offset : anchor.offset * (rect.height / anchor.height);
  container.scrollTop += rect.top - container.getBoundingClientRect().top - offset;
  return true;
}

import type { Message, MessagesPage, PageCap } from './types';

/** The viewer's page: at most PAGE_SIZE messages and, where the host caps pages, about PAGE_BYTES. */
export const PAGE_SIZE = 50;
export const PAGE_BYTES = 256 * 1024;

/** The viewer's cap for a page that must keep the message at anchor (-1: the page's last). */
export function pageCap(anchor: number): PageCap {
  return { maxBytes: PAGE_BYTES, anchor };
}

/**
 * The page of the engine's capPage (internal/engine/service.go): the
 * contiguous messages around cap.anchor whose sizeOf fits cap.maxBytes,
 * growing one before and one after in turn; the anchor is always kept.
 * The mock backend uses it; the real hosts cap on the server.
 */
export function capPage(page: MessagesPage, cap: PageCap, sizeOf: (m: Message) => number): MessagesPage {
  const list = page.messages || [];
  const n = list.length;
  if (cap.maxBytes <= 0 || n === 0) return page;
  const start = Math.min(page.offset, page.totalCount);
  const anchor = cap.anchor < 0 ? n - 1 : Math.min(Math.max(cap.anchor - start, 0), n - 1);
  let lo = anchor;
  let hi = anchor + 1;
  let used = sizeOf(list[anchor]);
  let before = lo > 0;
  let after = hi < n;
  while (before || after) {
    if (before) {
      const b = sizeOf(list[lo - 1]);
      if (used + b <= cap.maxBytes) {
        used += b;
        lo--;
        before = lo > 0;
      } else before = false;
    }
    if (after) {
      const b = sizeOf(list[hi]);
      if (used + b <= cap.maxBytes) {
        used += b;
        hi++;
        after = hi < n;
      } else after = false;
    }
  }
  if (lo === 0 && hi === n) return page;
  return {
    ...page,
    messages: list.slice(lo, hi),
    offset: start + lo,
    limit: hi - lo,
    hasMore: start + hi < page.totalCount,
    capped: true,
  };
}

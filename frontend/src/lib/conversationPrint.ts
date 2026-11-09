import type { Message, MessagesPage, PageCap, Part, SessionRef } from './types';
import { visibleTranscriptParts, withTranslation } from './transcript';
import { manualBashMessage, manualBashMarkdown } from './manualBash';
import { PAGE_SIZE, pageCap } from './pageCap';
import { StaleReplyError } from './link';

/** Only already displayed translations are supplied; printing never translates. */
export type PrintTranslation = (message: Message, index: number) => string | undefined;

/** Chat-level turns with role headings, sharing one image-attachment namespace. */
export function conversationPrintParts(
  ref: SessionRef,
  messages: Message[],
  offset = 0,
  translation?: PrintTranslation,
): Part[] {
  const parts: Part[] = [];
  messages.forEach((message, index) => {
    let shown = visibleTranscriptParts(message, 'chat');
    if (!shown.length) return;
    const bash = manualBashMessage(message, ref);
    const translated = translation?.(message, offset + index);
    if (bash) shown = [{ kind: 'text', text: manualBashMarkdown(bash) }];
    else if (translated !== undefined) shown = withTranslation(shown, translated);
    parts.push({ kind: 'text', text: `${parts.length ? '---\n\n' : ''}## ${message.role === 'user' ? 'User' : 'Assistant'}` }, ...shown);
  });
  return parts;
}

export type ConversationPageLoader = (
  ref: SessionRef, offset: number, limit: number, cap?: PageCap,
) => Promise<MessagesPage>;

/**
 * Fetch all Chat content without touching the viewer's loaded/search window.
 * Reject changing snapshots and incomplete pages rather than print partial or
 * mixed history. The caller's operation-wide guard also covers host switches
 * between requests, which individual API reply guards cannot detect.
 */
export async function loadConversationPrintParts(
  ref: SessionRef,
  getMessages: ConversationPageLoader,
  valid: () => boolean,
  translation?: PrintTranslation,
): Promise<Part[]> {
  const parts: Part[] = [];
  let offset = 0;
  let total: number | undefined;
  let revision: string | undefined;
  do {
    if (!valid()) throw new StaleReplyError();
    const page = await getMessages(ref, offset, PAGE_SIZE, pageCap(offset));
    if (!valid()) throw new StaleReplyError();
    if (total === undefined) {
      total = page.totalCount;
      revision = page.revision;
    } else if (total !== page.totalCount || revision !== page.revision) {
      throw new Error('The conversation changed while preparing it. Please try again.');
    }
    const end = page.offset + page.messages.length;
    if (page.offset !== offset || end > total || (end === offset && offset < total)
      || (!page.hasMore && end < total)) {
      throw new Error('The complete conversation could not be loaded. Please try again.');
    }
    const next = conversationPrintParts(ref, page.messages, offset, translation);
    if (parts.length && next.length) next.unshift({ kind: 'text', text: '---' });
    parts.push(...next);
    offset = end;
  } while (offset < total);
  return parts;
}

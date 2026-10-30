import type { Message, MessagesDelta, MessagesPage, MessagesSinceReply, MessagesSinceRequest, SessionRef } from './types';

/**
 * The messages a transcript view holds: [offset, offset + messages.length) of
 * the transcript at `revision`. Pages from hosts without revisions leave it
 * undefined, and such windows always refresh with full pages.
 */
export interface TranscriptWindow {
  messages: Message[];
  offset: number;
  hasMore: boolean;
  revision?: string;
}

export interface TranscriptPager {
  getMessages(ref: SessionRef, offset: number, limit: number): Promise<MessagesPage>;
  getMessagesSince(req: MessagesSinceRequest): Promise<MessagesSinceReply>;
}

export function windowOf(page: MessagesPage): TranscriptWindow {
  return { messages: page.messages || [], offset: page.offset, hasMore: page.hasMore, revision: page.revision };
}

/**
 * The page a delta stands for: the window's reused messages, kept as the same
 * objects so their rows stay as they are, then the delta's. Null when the
 * reused range is not inside the window.
 */
export function applyDelta(win: TranscriptWindow, delta: MessagesDelta): TranscriptWindow | null {
  const reused = delta.reused || 0;
  const at = delta.offset - win.offset;
  if (reused < 0 || reused > 0 && (at < 0 || at + reused > win.messages.length)) return null;
  const fresh = delta.messages || [];
  return {
    messages: reused > 0 ? win.messages.slice(at, at + reused).concat(fresh) : fresh,
    offset: delta.offset,
    hasMore: delta.hasMore,
    revision: delta.revision,
  };
}

/**
 * A page spliced into a window came from another revision, so the window may
 * mix two snapshots and needs a refresh. The window keeps its older revision:
 * a delta from it resends whatever changed since, including in the new page.
 */
export function mixesRevisions(win: TranscriptWindow, page: MessagesPage): boolean {
  return Boolean(win.revision && page.revision && win.revision !== page.revision);
}

/**
 * Refreshes transcript windows by delta where the connection serving them
 * supports it. An unsupported reply switches that connection, identified by
 * the data epoch, which moves with every (re)connect, to full pages.
 */
export class TranscriptRefresher {
  private readonly api: TranscriptPager;
  private readonly epoch: () => number;
  private unsupportedEpoch: number | null = null;

  constructor(api: TranscriptPager, epoch: () => number) {
    this.api = api;
    this.epoch = epoch;
  }

  /** The page at offset and limit, as getMessages returns it, reusing win's unchanged messages. */
  async refresh(ref: SessionRef, win: TranscriptWindow, offset: number, limit: number): Promise<TranscriptWindow> {
    const started = this.epoch();
    if (win.revision && this.unsupportedEpoch !== started) {
      const reply = await this.api.getMessagesSince({
        ref,
        baseRevision: win.revision,
        haveOffset: win.offset,
        haveCount: win.messages.length,
        offset,
        limit,
      });
      if (!reply.supported) this.unsupportedEpoch = started;
      const next = reply.supported && reply.delta ? applyDelta(win, reply.delta) : null;
      if (next) return next;
    }
    return windowOf(await this.api.getMessages(ref, offset, limit));
  }
}

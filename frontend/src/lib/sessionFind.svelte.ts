import type { MessageJump, SessionRef, SessionSearchHit, SessionSearchReply, SessionSearchRequest } from './types';
import { LatestRequestGate, searchTerms } from './search';
import { findCount } from './fileFind';
import { searchSeed } from './contentSearch';
import { errorText } from './manage';

export const SESSION_FIND_DEBOUNCE_MS = 150;

interface FindHost {
  search: (req: SessionSearchRequest) => Promise<SessionSearchReply>;
  /** The open session. */
  ref: () => SessionRef | null;
  /** Meta and system messages are searched where the display level shows them. */
  includeHidden: () => boolean;
  /** The topmost message on screen, or -1. */
  topIndex: () => number;
  /** Shows a match: scrolls to it and marks it until the next one. */
  jump: (jump: MessageJump) => void;
  /** Drops the marks. */
  clear: () => void;
  isStale: (err: unknown) => boolean;
}

/**
 * Find in the open session: the host searches its whole transcript, not
 * only the loaded page, and each step jumps to a matching message.
 */
export class SessionFind {
  open = $state(false);
  query = $state('');
  hits = $state<SessionSearchHit[]>([]);
  current = $state(-1);
  loading = $state(false);
  error = $state<string | null>(null);
  truncated = $state(false);
  unsupported = $state(false);
  /** Bumped when the input should take focus. */
  focusRequest = $state(0);

  private gate = new LatestRequestGate(SESSION_FIND_DEBOUNCE_MS);
  private jumpID = 0;
  // A typed search waits for the debounce.
  private typed = false;

  constructor(private host: FindHost) {}

  get count(): string {
    if (!this.searchable || this.unsupported || this.error) return '';
    return findCount({ matches: this.hits, capped: this.truncated }, this.current);
  }

  /** What the host and the marks search for: a quote still being typed is closed. */
  get search(): string {
    const query = this.query.trim();
    return (query.match(/"/g)?.length ?? 0) % 2 ? `${query}"` : query;
  }

  private get searchable(): boolean {
    return searchTerms(this.search).length > 0;
  }

  /** Opens the bar, searching the selected text when there is one. */
  show(selection = ''): void {
    const seed = findSeed(selection);
    const wasOpen = this.open;
    this.open = true;
    this.focusRequest++;
    if (seed && seed !== this.query) this.setQuery(seed);
    else if (!wasOpen && this.searchable) void this.runNow();
  }

  close(): void {
    this.open = false;
    this.gate.cancel();
    this.typed = false;
    this.clearResults();
    this.host.clear();
  }

  /** Another session: nothing of this one's find carries over. */
  reset(): void {
    this.close();
    this.query = '';
  }

  setQuery(query: string): void {
    this.query = query;
    if (!this.searchable) {
      this.gate.cancel();
      this.typed = false;
      this.clearResults();
      this.host.clear();
      return;
    }
    this.loading = true;
    this.typed = true;
    this.gate.schedule(() => void this.runNow());
  }

  /** The transcript or the display level changed: count again, keeping the place. */
  refresh(): void {
    if (this.open && this.searchable && !this.typed) void this.runNow(true);
  }

  /**
   * Searches now. A new search starts at the first match from the top of
   * the screen; a refresh keeps the current match's message.
   */
  async runNow(keep = false): Promise<void> {
    const generation = this.gate.begin();
    this.typed = false;
    const ref = this.host.ref();
    const query = this.search;
    if (!ref || !this.searchable) return;
    const kept = this.hits[this.current]?.messageIndex;
    this.loading = true;
    try {
      const reply = await this.host.search({ ref, query, includeHidden: this.host.includeHidden() });
      if (!this.gate.isCurrent(generation)) return;
      this.error = null;
      this.unsupported = !reply.supported;
      this.hits = reply.result?.hits ?? [];
      this.truncated = Boolean(reply.result?.truncated);
      this.current = this.hits.length ? nearest(this.hits, keep && kept !== undefined ? kept : this.host.topIndex()) : -1;
      // A refresh never moves the view: it only recounts.
      if (keep) return;
      if (this.current >= 0) this.jump(ref, query);
      else this.host.clear();
    } catch (err) {
      if (!this.gate.isCurrent(generation) || this.host.isStale(err)) return;
      this.hits = [];
      this.current = -1;
      this.truncated = false;
      this.error = errorText(err, 'Search failed');
    } finally {
      if (this.gate.isCurrent(generation)) this.loading = false;
    }
  }

  /** The next match, or with -1 the previous one, wrapping at the ends. */
  step(delta: number): void {
    const ref = this.host.ref();
    if (!ref || !this.hits.length) return;
    const from = this.current < 0 ? (delta > 0 ? -1 : 0) : this.current;
    this.current = (from + delta + this.hits.length) % this.hits.length;
    this.jump(ref, this.search);
  }

  handleKey(e: KeyboardEvent): void {
    if (e.key === 'Enter') {
      e.preventDefault();
      // Enter before the typed search was sent sends it now.
      if (this.typed) void this.runNow();
      else this.step(e.shiftKey ? -1 : 1);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      this.close();
    }
  }

  private jump(ref: SessionRef, query: string): void {
    const hit = this.hits[this.current];
    this.host.jump({
      id: ++this.jumpID,
      ref: { ...ref },
      messageIndex: hit.messageIndex,
      kind: 'text',
      query,
      parts: hit.parts,
      sticky: true,
    });
  }

  private clearResults(): void {
    this.hits = [];
    this.current = -1;
    this.loading = false;
    this.error = null;
    this.truncated = false;
    this.unsupported = false;
  }
}

/** The first match at or after the message, else the last one before it. */
function nearest(hits: SessionSearchHit[], messageIndex: number): number {
  const at = hits.findIndex((hit) => hit.messageIndex >= messageIndex);
  return at >= 0 ? at : hits.length - 1;
}

/** Selected words are found together, as a phrase. */
export function findSeed(selection: string): string | undefined {
  const seed = searchSeed(selection.trim());
  return seed && /\s/.test(seed) && !seed.includes('"') ? `"${seed}"` : seed;
}

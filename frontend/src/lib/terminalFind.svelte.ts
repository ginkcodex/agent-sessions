import type { SearchAddon, ISearchResultChangeEvent } from '@xterm/addon-search';
import { searchSeed } from './contentSearch';
import { terminalFindColors } from './terminal';

export const TERMINAL_FIND_LIMIT = 1000;

interface FindHost {
  addon: () => SearchAddon | null;
  selection: () => string;
  focus: () => void;
  theme: () => 'light' | 'dark';
}

/** One find session per terminal view, retained when its tab is hidden. */
export class TerminalFind {
  open = $state(false);
  query = $state('');
  matchCase = $state(false);
  resultIndex = $state(-1);
  resultCount = $state(0);
  found = $state(false);

  constructor(private host: FindHost) {}

  get count(): string {
    if (!this.query) return '';
    if (!this.found) return 'No results';
    const total = `${this.resultCount}${this.resultCount >= TERMINAL_FIND_LIMIT ? '+' : ''}`;
    return this.resultIndex < 0 ? `${total} matches` : `${this.resultIndex + 1} of ${total}`;
  }

  openFind(): void {
    const seed = searchSeed(this.host.selection());
    if (seed) this.query = seed;
    this.open = true;
    this.search();
  }

  closeFind(): void {
    this.open = false;
    this.host.addon()?.clearDecorations();
    this.host.focus();
  }

  setQuery(query: string): void {
    this.query = query;
    this.search();
  }

  toggleCase(): void {
    this.matchCase = !this.matchCase;
    this.refresh();
  }

  refresh(): void {
    // The addon caches highlights by query; invalidate them for option or
    // theme changes too, even when the query itself is unchanged.
    this.host.addon()?.clearDecorations();
    this.search();
  }

  setResults(result: ISearchResultChangeEvent): void {
    this.resultIndex = result.resultIndex;
    this.resultCount = result.resultCount;
    this.found = result.resultCount > 0;
  }

  search(direction: 'next' | 'previous' = 'next', incremental = true): void {
    const addon = this.host.addon();
    if (!addon) return;
    if (!this.query) {
      addon.clearDecorations();
      this.resultIndex = -1;
      this.resultCount = 0;
      this.found = false;
      return;
    }
    const options = {
      incremental,
      caseSensitive: this.matchCase,
      decorations: terminalFindColors(this.host.theme()),
    };
    this.found = direction === 'previous'
      ? addon.findPrevious(this.query, options)
      : addon.findNext(this.query, options);
  }
}

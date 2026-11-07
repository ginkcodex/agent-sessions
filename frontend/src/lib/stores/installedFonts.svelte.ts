import { api } from '../api';

/**
 * Monospace fonts installed on this computer, shared by the terminal and
 * editor font pickers: listed once per Settings opening, and again on Refresh.
 */
export class InstalledFonts {
  fonts = $state.raw<string[]>([]);
  // True until a listing finishes, so a chosen font is not called
  // unavailable before the list is known.
  loading = $state(true);
  error = $state('');
  private stale = true;
  private pending: Promise<void> | null = null;

  constructor(private readonly list: () => Promise<string[]> = () => api.terminalFonts()) {}

  /** Lists the fonts unless this Settings opening already has. */
  ensure(): Promise<void> {
    return this.stale ? this.refresh() : (this.pending ?? Promise.resolve());
  }

  /** Lists the fonts again; a listing in progress is shared. */
  refresh(): Promise<void> {
    if (this.pending) return this.pending;
    this.stale = false;
    this.loading = true;
    this.error = '';
    this.pending = (async () => {
      try {
        this.fonts = await this.list();
      } catch (err) {
        this.error = err instanceof Error ? err.message : String(err);
        this.stale = true;
      } finally {
        this.loading = false;
        this.pending = null;
      }
    })();
    return this.pending;
  }

  /** Settings closed: the next opening lists the fonts again. */
  markStale(): void {
    this.stale = true;
  }
}

export const installedFonts = new InstalledFonts();

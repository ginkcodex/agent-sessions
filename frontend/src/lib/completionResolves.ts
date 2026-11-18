import type { CodeCompletionDetails } from './codeDiagnostics';

/** How a completion answer's items are resolved on the host. */
export interface ResolveSource {
  /** A new request ID, for cancel. */
  newId(): string;
  /** Resolves item index at version under requestId; rejects for an answer that is not this one. */
  ask(requestId: string, index: number, version: number): Promise<CodeCompletionDetails>;
  /** Cancels requestId on the host; never throws. */
  cancel(requestId: string): void;
}

// A resolve in flight: its request ID, to cancel it, and how many callers wait for it.
interface Resolving { promise: Promise<CodeCompletionDetails>; requestId: string; waiters: number; done: boolean }

/**
 * The resolves of one completion answer, by `index@version`, asked once and
 * shared. Only version 0 and the latest version asked are kept: the editor
 * reads no other version again.
 */
export class CompletionResolves {
  private readonly resolving = new Map<string, Resolving>();
  private readonly done = new Map<string, CodeCompletionDetails>();
  private latest = 0;
  private readonly source: ResolveSource;

  constructor(source: ResolveSource) { this.source = source; }

  /** What item index resolved to at version, when it already did. */
  settled(index: number, version: number): CodeCompletionDetails | undefined {
    return this.done.get(`${index}@${version}`);
  }

  /** The keys held, in flight and settled: for tests. */
  keys(): { resolving: string[]; settled: string[] } {
    return { resolving: [...this.resolving.keys()].sort(), settled: [...this.done.keys()].sort() };
  }

  /**
   * Item index resolved at version. Each caller waits until its signal
   * aborts, when its promise rejects, or, without a signal, until the resolve
   * settles. Once no caller waits, the request is cancelled on the host, and
   * the next ask starts another. A failure is asked again next time.
   */
  at(index: number, version: number, signal?: AbortSignal): Promise<CodeCompletionDetails> {
    if (signal?.aborted) return Promise.reject(new Error('aborted'));
    if (version > 0 && version !== this.latest) {
      this.latest = version;
      this.dropOtherVersions(version);
    }
    const key = `${index}@${version}`;
    let entry = this.resolving.get(key);
    if (!entry) {
      const requestId = this.source.newId();
      const asked: Resolving = {
        requestId, waiters: 0, done: false,
        promise: this.source.ask(requestId, index, version).then(d => {
          if (this.resolving.get(key) === asked) this.done.set(key, d);
          return d;
        }),
      };
      asked.promise.then(() => { asked.done = true; }, () => {
        asked.done = true;
        if (this.resolving.get(key) === asked) this.resolving.delete(key);
      });
      this.resolving.set(key, entry = asked);
    }
    const waited = entry;
    waited.waiters++;
    if (!signal) return waited.promise;
    return new Promise((resolve, reject) => {
      const leave = () => {
        reject(new Error('aborted'));
        if (--waited.waiters > 0 || waited.done) return;
        if (this.resolving.get(key) === waited) this.resolving.delete(key);
        this.source.cancel(waited.requestId);
      };
      signal.addEventListener('abort', leave, { once: true });
      const stop = () => signal.removeEventListener('abort', leave);
      waited.promise.then(d => { stop(); resolve(d); }, e => { stop(); reject(e); });
    });
  }

  private dropOtherVersions(version: number): void {
    const old = (key: string) => !key.endsWith('@0') && !key.endsWith(`@${version}`);
    for (const key of [...this.resolving.keys()]) if (old(key)) this.resolving.delete(key);
    for (const key of [...this.done.keys()]) if (old(key)) this.done.delete(key);
  }
}

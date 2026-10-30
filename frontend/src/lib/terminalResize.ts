interface TerminalSize {
  cols: number;
  rows: number;
}

const sameSize = (a: TerminalSize | undefined, b: TerminalSize) => a?.cols === b.cols && a.rows === b.rows;

/** Coalesce redraws, but never let an older PTY resize overtake a newer one. */
export class TerminalResize {
  private wanted: TerminalSize | undefined;
  private sent: TerminalSize | undefined;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private sending = false;
  private disposed = false;

  private send: (cols: number, rows: number) => Promise<void>;
  private delay: number;
  private retryDelay: number;

  constructor(send: (cols: number, rows: number) => Promise<void>, delay = 80, retryDelay = 250) {
    this.send = send;
    this.delay = delay;
    this.retryDelay = retryDelay;
  }

  request(cols: number, rows: number): void {
    if (this.disposed) return;
    this.wanted = { cols, rows };
    this.schedule(this.delay);
  }

  dispose(): void {
    this.disposed = true;
    clearTimeout(this.timer);
  }

  private schedule(delay: number): void {
    clearTimeout(this.timer);
    this.timer = setTimeout(() => { void this.flush(); }, delay);
  }

  private async flush(): Promise<void> {
    if (this.disposed || this.sending || !this.wanted || sameSize(this.sent, this.wanted)) return;
    const size = this.wanted;
    this.sending = true;
    let failed = false;
    try {
      await this.send(size.cols, size.rows);
      this.sent = size;
    } catch {
      // A rejected call did not establish the size. Retry even if the box
      // stays unchanged: a stale PTY width breaks shell line editing.
      failed = true;
    } finally {
      this.sending = false;
      if (!this.disposed && this.wanted && !sameSize(this.sent, this.wanted)) {
        this.schedule(failed ? this.retryDelay : 0);
      }
    }
  }
}

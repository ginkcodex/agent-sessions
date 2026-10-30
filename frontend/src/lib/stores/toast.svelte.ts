import type { ToastMessage } from '../guidance';

const SHOW_MS = 8000;
type PauseReason = 'pointer' | 'focus';

// One toast at a time. Old controls and timers may only dismiss their own ID.
export class ToastStore {
  current = $state<(ToastMessage & { id: number }) | null>(null);

  private seq = 0;
  private timer: ReturnType<typeof setTimeout> | null = null;
  private paused = new Set<PauseReason>();

  show(message: ToastMessage): number {
    const previous = this.current;
    const id = ++this.seq;
    this.clearTimer();
    this.paused.clear();
    this.current = { ...message, id };
    this.startTimer();
    // Publish the replacement first so a dismissal callback cannot dismiss
    // the old toast again (and can safely show a new message itself).
    previous?.onDismiss?.();
    return id;
  }

  dismiss(id?: number, reason: 'dismiss' | 'select' = 'dismiss'): void {
    if (!this.current || (id !== undefined && this.current.id !== id)) return;
    const message = this.current;
    this.clearTimer();
    this.paused.clear();
    this.current = null;
    message.onDismiss?.(reason);
  }

  invoke(id: number, index: number): void {
    if (this.current?.id !== id || !Number.isInteger(index) || index < 0) return;
    const action = this.current.actions?.[index];
    if (!action) return;
    this.dismiss(id, 'select');
    action.onSelect();
  }

  pause(reason: PauseReason = 'pointer'): void {
    this.paused.add(reason);
    this.clearTimer();
  }

  resume(reason: PauseReason = 'pointer'): void {
    this.paused.delete(reason);
    this.startTimer();
  }

  private clearTimer(): void {
    if (this.timer !== null) clearTimeout(this.timer);
    this.timer = null;
  }

  private startTimer(): void {
    this.clearTimer();
    const message = this.current;
    if (message && !message.persistent && this.paused.size === 0) {
      this.timer = setTimeout(() => this.dismiss(message.id), SHOW_MS);
    }
  }
}

export const toast = new ToastStore();

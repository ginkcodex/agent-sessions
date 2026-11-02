import { api } from '../api';
import { epoch } from '../link';
import type { SessionMeta, SessionRef } from '../types';
import { appState } from './appState.svelte';
import { link } from './link.svelte';

export class SessionRename {
  target = $state<{ ref: SessionRef; title: string } | null>(null);
  title = $state('');
  busy = $state(false);
  error = $state('');
  private owner = 0;
  private generation = 0;

  open(session: SessionMeta): void {
    if (this.busy || link.stale || link.locked) return;
    this.generation++;
    this.owner = epoch();
    this.target = { ref: { ...session.ref }, title: session.title || session.firstPrompt || '(untitled)' };
    this.title = session.title || '';
    this.error = '';
  }

  dismiss(): void {
    this.generation++;
    this.target = null;
    this.error = '';
    this.busy = false;
  }

  reconcileHost(): void {
    if (this.target && (this.owner !== epoch() || link.stale || link.locked)) this.dismiss();
  }

  get valid(): boolean {
    return Boolean(this.title.trim()) && !/[\u0000-\u001f\u007f]/.test(this.title);
  }

  async save(): Promise<void> {
    if (!this.target || this.busy || !this.valid || link.stale || link.locked || this.owner !== epoch()) return;
    const id = this.generation;
    const owner = this.owner;
    const ref = { ...this.target.ref };
    this.busy = true;
    this.error = '';
    try {
      await api.renameSession(ref, this.title.trim());
      if (id !== this.generation || owner !== epoch()) return;
      this.dismiss();
      await appState.refresh();
    } catch (err) {
      if (id === this.generation && owner === epoch()) this.error = err instanceof Error ? err.message : String(err);
    } finally {
      if (id === this.generation) this.busy = false;
    }
  }
}

export const sessionRename = new SessionRename();

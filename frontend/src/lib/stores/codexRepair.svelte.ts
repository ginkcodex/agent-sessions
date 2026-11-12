import { api } from '../api';
import { epoch } from '../link';
import { needsCodexRepair, type CodexRepairResult } from '../codexRepair';
import type { SessionMeta, SessionRef } from '../types';
import { appState } from './appState.svelte';
import { link } from './link.svelte';
import { toast } from './toast.svelte';

export class CodexRepair {
  busy = $state(false);
  error = $state('');
  result = $state<CodexRepairResult | null>(null);
  private target: SessionRef | null = null;
  private key = '';
  private generation = 0;
  private owner = 0;

  setSession(meta: SessionMeta): void {
    const key = `${epoch()}:${meta.ref.agent}:${meta.ref.id}:${meta.sourcePath ?? ''}`;
    if (key === this.key) return;
    this.key = key;
    this.generation++;
    this.owner = epoch();
    this.target = needsCodexRepair(meta) ? { ...meta.ref } : null;
    this.busy = false;
    this.error = '';
    this.result = null;
  }

  async run(): Promise<void> {
    if (!this.target || this.busy || this.result || link.stale || link.locked || this.owner !== epoch()) return;
    const ref = { ...this.target };
    const generation = this.generation;
    const owner = this.owner;
    const current = () => generation === this.generation && owner === epoch();
    this.busy = true;
    this.error = '';
    try {
      const result = await api.repairCodexSession(ref);
      if (!current()) return;
      this.result = result;
      toast.show({ title: 'Session repaired', body: 'You can resume it now. A backup was saved.', code: result.backupPath });
      try { await appState.refresh(); }
      catch (err) { if (current()) this.error = `Session repaired. Refresh the session list: ${err instanceof Error ? err.message : String(err)}`; }
    } catch (err) {
      if (current()) this.error = err instanceof Error ? err.message : String(err);
    } finally {
      if (current()) this.busy = false;
    }
  }
}

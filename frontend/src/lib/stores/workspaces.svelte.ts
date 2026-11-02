import { api } from '../api';
import { epoch } from '../link';
import { link } from './link.svelte';
import { appState } from './appState.svelte';
import { directoryNodeKey } from '../tree';

/** Explicit workspaces are persisted and authorized on the host that owns them. */
export class WorkspacesStore {
  open = $state(false);
  busy = $state(false);
  error = $state('');
  directories = $state<string[]>([]);
  private owner = 0;
  private generation = 0;

  show(): void {
    if (link.stale || link.locked) return;
    this.owner = epoch();
    this.error = '';
    this.open = true;
  }

  dismiss(): void { this.open = false; }

  async refresh(): Promise<void> {
    const id = ++this.generation;
    const owner = epoch();
    this.directories = [];
    try {
      const dirs = await api.listWorkspaces();
      if (id === this.generation && owner === epoch()) this.directories = dirs;
    } catch { /* Older hosts can still browse their discovered sessions. */ }
  }

  async add(path: string): Promise<boolean> {
    if (this.busy || !path.trim() || this.owner !== epoch() || link.stale || link.locked) return false;
    this.busy = true;
    this.error = '';
    const owner = this.owner;
    try {
      const dir = await api.openWorkspace(path.trim());
      if (owner !== epoch()) return false;
      await this.refresh();
      // Opening a workspace is explicit navigation, not a search within the old filters.
      appState.filter = {};
      await appState.setGroupMode('dir-agent');
      if (owner !== epoch()) return false;
      const key = directoryNodeKey(appState.groups, dir);
      if (key) await appState.selectGroup(key, 'push');
      if (owner !== epoch()) return false;
      await appState.selectSession(null);
      this.open = false;
      return true;
    } catch (err) {
      if (owner === epoch()) this.error = err instanceof Error ? err.message : String(err);
      return false;
    } finally { this.busy = false; }
  }

  async remove(dir: string): Promise<void> {
    if (link.stale || link.locked) return;
    const owner = epoch();
    try {
      await api.removeWorkspace(dir);
      if (owner !== epoch()) return;
      await this.refresh();
      await appState.reload();
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
    }
  }
}

export const workspaces = new WorkspacesStore();

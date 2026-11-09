import { link } from './link.svelte';
import { filterStarredWorkspaces, loadWorkspaceStars, workspaceStarKey, WORKSPACE_STARS_KEY } from '../workspaceStars';
import type { GroupNode } from '../types';

/** Client-side favorites stay separate from host-owned workspace registrations. */
export class WorkspaceStarsStore {
  private keys = $state<string[]>(loadWorkspaceStars());
  onlyStarred = $state(false);

  isStarred(cwd: string): boolean {
    return this.keys.includes(workspaceStarKey(cwd, link.dataHost));
  }

  toggle(cwd: string): void {
    if (!cwd || link.stale || link.locked) return;
    const key = workspaceStarKey(cwd, link.dataHost);
    this.keys = this.keys.includes(key) ? this.keys.filter(saved => saved !== key) : [...this.keys, key];
    try { localStorage.setItem(WORKSPACE_STARS_KEY, JSON.stringify(this.keys)); }
    catch { /* Keep in-memory stars usable when storage is unavailable. */ }
  }

  visibleGroups(groups: GroupNode[]): GroupNode[] {
    return this.onlyStarred ? filterStarredWorkspaces(groups, cwd => this.isStarred(cwd)) : groups;
  }
}

export const workspaceStars = new WorkspaceStarsStore();

import type { GroupNode } from './types';
import { nodeKind } from './tree';

export const WORKSPACE_STARS_KEY = 'agent-sessions:workspace-stars';

/** Match Windows paths independently of case and slash style; preserve POSIX case. */
export function workspacePath(path: string): string {
  const windows = /^[a-z]:[\\/]/i.test(path) || path.startsWith('\\\\') || path.startsWith('//');
  const normalized = windows ? path.replace(/\\/g, '/').toLowerCase() : path;
  return normalized.replace(/\/+$/, '') || '/';
}

/** Use the data owner, not the connecting host or a presentation/grouping key. */
export function workspaceStarKey(path: string, host?: string): string {
  return JSON.stringify([host ?? '', workspacePath(path)]);
}

/** Ignore malformed preferences without preventing the workspace list from opening. */
export function loadWorkspaceStars(): string[] {
  try {
    const saved: unknown = JSON.parse(localStorage.getItem(WORKSPACE_STARS_KEY) ?? '[]');
    if (!Array.isArray(saved)) return [];
    const valid: string[] = [];
    for (const key of saved) {
      if (typeof key !== 'string') continue;
      try {
        const pair: unknown = JSON.parse(key);
        if (Array.isArray(pair) && pair.length === 2 && typeof pair[0] === 'string'
          && typeof pair[1] === 'string' && pair[1]) {
          valid.push(workspaceStarKey(pair[1], pair[0]));
        }
      } catch { /* Ignore only the invalid entry. */ }
    }
    return [...new Set(valid)];
  } catch { return []; }
}

/** Keep workspace subtrees and their context ancestors without changing the source tree. */
export function filterStarredWorkspaces(groups: GroupNode[], matches: (cwd: string) => boolean): GroupNode[] {
  return groups.flatMap(node => {
    const kind = nodeKind(node);
    // A directory owns its whole grouped subtree. Flat session families behave
    // the same way, even when a subagent's cwd differs from its parent's.
    if (kind === 'directory') return node.cwd && matches(node.cwd) ? [node] : [];
    if (kind === 'session' && node.cwd && matches(node.cwd)) return [node];
    const children = filterStarredWorkspaces(node.children ?? [], matches);
    if (!children.length) return [];
    const sessions = [...new Map(children.flatMap(child => child.sessions ?? [])
      .map(ref => [JSON.stringify([ref.agent, ref.id]), ref])).values()];
    return [{ ...node, children, sessions, sessionCount: sessions.length }];
  });
}

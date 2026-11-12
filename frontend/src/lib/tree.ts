import type { GroupNode, GroupNodeKind, SessionMeta, SessionRef } from './types';
import { refsEqual } from './manage';

/**
 * Returns the node's kind, inferring it from fields when the backend did not
 * send one (older cache or mock data) so the UI never renders blank.
 */
export function nodeKind(node: GroupNode): GroupNodeKind {
  if (node.kind) return node.kind;
  if (node.agent && !node.cwd) return 'agent';
  if (node.cwd) return 'directory';
  return 'session';
}

/** The workspace of a tree row, inherited by agents and topics under a directory. */
export function nodeDirectory(groups: GroupNode[], key: string): string | undefined {
  function walk(nodes: GroupNode[], inherited?: string): string | undefined {
    for (const node of nodes) {
      const cwd = node.cwdMissing ? undefined : node.cwd || inherited;
      if (node.key === key) return cwd;
      const found = walk(node.children ?? [], cwd);
      if (found !== undefined) return found;
    }
    return undefined;
  }
  return walk(groups);
}

/**
 * The last name in a directory label such as ~/Workspaces/app or
 * C:\Users\me\app, for showing above the whole path. Labels without a
 * name of their own (~, /, C:\, (unknown directory)) come back as they are.
 */
export function directoryName(label: string): string {
  const trimmed = label.replace(/[\\/]+$/, '');
  const name = trimmed.slice(Math.max(trimmed.lastIndexOf('/'), trimmed.lastIndexOf('\\')) + 1);
  return name && !/^[A-Za-z]:$/.test(name) ? name : label;
}

/**
 * Computes the set of keys that should start collapsed. The top-level
 * workspaces start closed, while top-level agent groups stay open.
 * Nested groups and sessions with subagents also start closed.
 */
export function defaultCollapsedKeys(groups: GroupNode[]): Set<string> {
  const out = new Set<string>();
  const walk = (nodes: GroupNode[], nested: boolean) => {
    for (const node of nodes) {
      const children = node.children ?? [];
      if (children.length > 0 && (nested || nodeKind(node) === 'session' || nodeKind(node) === 'directory')) {
        out.add(node.key);
      }
      walk(children, true);
    }
  };
  walk(groups, false);
  return out;
}

/**
 * True when the node is collapsed. A key in `manual` records a user toggle
 * and flips that node's default state.
 */
export function isCollapsed(
  key: string,
  manual: Set<string>,
  defaults: Set<string>
): boolean {
  return manual.has(key) !== defaults.has(key);
}

/** Returns the subset of `keys` that still exist somewhere in the tree. */
export function pruneKeys(keys: Set<string>, groups: GroupNode[]): Set<string> {
  const live = new Set<string>();
  const walk = (nodes: GroupNode[]) => {
    for (const node of nodes) {
      if (keys.has(node.key)) live.add(node.key);
      if (node.children) walk(node.children);
    }
  };
  walk(groups);
  return live;
}

/**
 * Picks the session to select after a tree node is clicked: a session node
 * selects itself when visible, otherwise the first visible session in the
 * list. Returns null when the list is empty.
 */
export function sessionToSelect(
  node: GroupNode,
  visible: { ref: SessionRef }[]
): SessionRef | null {
  const own = nodeKind(node) === 'session' ? node.sessions?.[0] : undefined;
  if (own && visible.some((s) => refsEqual(s.ref, own))) return own;
  return visible[0]?.ref ?? null;
}

/**
 * What the terminal panel should show once a tree row opens session ref: the
 * session's own directory and identity, as clicking its row in the list
 * passes, so its own tab wins. The row's directory stands in when the session's
 * is unknown, and alone when no session opens. Null without any directory.
 */
export function workspaceTerminalTarget<T extends Pick<SessionMeta, 'ref' | 'cwd' | 'cwdMissing'>>(
  visible: T[],
  ref: SessionRef | null,
  rowDirectory: string | undefined
): { dir: string; session?: T } | null {
  const session = ref ? visible.find((s) => refsEqual(s.ref, ref)) : undefined;
  const dir = (session && !session.cwdMissing && session.cwd) || rowDirectory;
  if (!dir) return null;
  return session ? { dir, session } : { dir };
}

/**
 * The keys of the nodes that hold a session: the group it is in, the groups
 * around that one, and its own node where the tree shows sessions (flat
 * mode, subagents). Empty when the tree does not have it, e.g. a filter
 * hides it.
 */
export function nodesHolding(groups: GroupNode[], ref: SessionRef | null): Set<string> {
  const keys = new Set<string>();
  if (!ref) return keys;
  const walk = (nodes: GroupNode[]) => {
    for (const node of nodes) {
      // A node lists every session under it, so only one that lists the
      // session can hold it.
      if (!node.sessions?.some((s) => refsEqual(s, ref))) continue;
      keys.add(node.key);
      if (node.children) walk(node.children);
    }
  };
  walk(groups);
  return keys;
}

/**
 * The row to show as selected. The selected group stays when it holds the
 * open session, or when the tree does not have the session. Otherwise it is
 * the innermost row holding the session that shows: groups inside a closed
 * one are hidden, so the closed one stands for them.
 */
export function nodeToHighlight(
  groups: GroupNode[],
  ref: SessionRef | null,
  selectedKey: string | null,
  manual: Set<string>,
  defaults: Set<string>
): string | null {
  const holding = nodesHolding(groups, ref);
  if (holding.size === 0 || (selectedKey && holding.has(selectedKey))) return selectedKey;
  let innermost: string | null = null;
  const walk = (nodes: GroupNode[]) => {
    const node = nodes.find((n) => holding.has(n.key));
    if (!node) return;
    innermost = node.key;
    if (node.children && !isCollapsed(node.key, manual, defaults)) walk(node.children);
  };
  walk(groups);
  return innermost;
}

/** What a click in the group tree may land on and still be empty space. */
const TREE_CONTROLS = '.node-row, button, a, input, select, textarea, [role="menu"]';

/**
 * Whether a click in the group tree landed on its empty space — below the
 * rows, between them, or on a message — rather than on a row, a control or
 * an open menu. Such a click releases the group filter.
 */
export function isEmptyTreeClick(target: EventTarget | null): boolean {
  const el = target as { closest?: (selector: string) => unknown } | null;
  return typeof el?.closest === 'function' && !el.closest(TREE_CONTROLS);
}

/**
 * Lists the distinct directories in the tree that still exist on disk,
 * sorted, as candidates to import a bundle into.
 */
export function knownDirectories(groups: GroupNode[]): string[] {
  const dirs = new Set<string>();
  const walk = (nodes: GroupNode[]) => {
    for (const node of nodes) {
      if (nodeKind(node) === 'directory' && node.cwd && !node.cwdMissing) dirs.add(node.cwd);
      if (node.children) walk(node.children);
    }
  };
  walk(groups);
  return [...dirs].sort((a, b) => a.localeCompare(b));
}

/**
 * Finds the directory node for cwd, the group to show after a session was
 * imported there. When directories sit under agent groups, the one under
 * agent wins. Returns null when the tree has no such directory.
 */
export function directoryNodeKey(groups: GroupNode[], cwd: string, agent?: string): string | null {
  let fallback: string | null = null;
  const walk = (nodes: GroupNode[], parentAgent?: string): string | null => {
    for (const node of nodes) {
      const kind = nodeKind(node);
      if (kind === 'directory' && node.cwd === cwd) {
        if (!parentAgent || parentAgent === agent) return node.key;
        fallback ??= node.key;
      }
      if (node.children) {
        const found = walk(node.children, kind === 'agent' ? node.agent : parentAgent);
        if (found) return found;
      }
    }
    return null;
  };
  return walk(groups) ?? fallback;
}

/**
 * Hashes a remote host alias deterministically to an 8-character hex string using FNV-1a.
 */
export function hashHost(host: string): string {
  let hash = 0x811c9dc5;
  for (let i = 0; i < host.length; i++) {
    hash ^= host.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(16).padStart(8, '0');
}

/**
 * Returns the storage namespace identifier for a target host.
 * Empty, null, or 'local' identifies the local machine.
 */
export function targetKey(host?: string): string {
  if (!host || host === 'local') {
    return 'local';
  }
  return hashHost(host);
}

export const BASE_COLLAPSED_STORAGE_KEY = 'agent-sessions:tree-toggled';

// Toggles saved by earlier versions. Each one flips a default that has
// since changed, so kept they would open and close the wrong groups.
const LEGACY_COLLAPSED_STORAGE_KEY = 'agent-sessions:tree-collapsed';

/**
 * Returns the namespaced storage key for the given host, and drops that
 * host's toggles saved under the legacy key.
 */
export function storageKeyForCollapsed(host?: string): string {
  const target = targetKey(host);
  if (typeof localStorage !== 'undefined') {
    try {
      localStorage.removeItem(`${LEGACY_COLLAPSED_STORAGE_KEY}:${target}`);
      if (target === 'local') localStorage.removeItem(LEGACY_COLLAPSED_STORAGE_KEY);
    } catch {
      // Storage access disabled or restricted
    }
  }
  return `${BASE_COLLAPSED_STORAGE_KEY}:${target}`;
}

export function loadCollapsedKeys(host?: string): Set<string> {
  if (typeof localStorage === 'undefined') return new Set();
  try {
    const key = storageKeyForCollapsed(host);
    const parsed = JSON.parse(localStorage.getItem(key) || '[]');
    if (Array.isArray(parsed)) {
      return new Set(parsed.filter((k): k is string => typeof k === 'string'));
    }
  } catch {
    // Ignore JSON parse errors and start from defaults
  }
  return new Set();
}

export function saveCollapsedKeys(keys: Set<string>, host?: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    const key = storageKeyForCollapsed(host);
    localStorage.setItem(key, JSON.stringify([...keys]));
  } catch {
    // Storage quota exceeded or disabled
  }
}


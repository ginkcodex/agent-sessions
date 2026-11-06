// The session list as a tree: each subagent session under the session that
// spawned it. Pure functions, so the list component only holds state.
import type { SessionMeta, SessionRef } from './types';
import { refKey } from './manage';

export interface SessionTreeNode {
  key: string;
  session: SessionMeta;
  parentKey: string | null;
  children: SessionTreeNode[];
  /**
   * Shown only to keep a matching subagent with its family: the session
   * itself does not match the current filters.
   */
  context: boolean;
}

export interface SessionTree {
  roots: SessionTreeNode[];
  byKey: Map<string, SessionTreeNode>;
}

export interface SessionTreeRow {
  node: SessionTreeNode;
  depth: number;
  expanded: boolean;
}

export function parentRef(session: SessionMeta): SessionRef | null {
  return session.parentId ? { agent: session.ref.agent, id: session.parentId } : null;
}

/**
 * Builds the tree from the sessions that match the filters, in the order the
 * backend sorted them, plus the ancestors fetched to give them context. A
 * family sits where its first matching member would have been; children are
 * ordered by when they were started. Context sessions that no longer lead to
 * a match are dropped.
 */
export function buildSessionTree(matches: SessionMeta[], context: SessionMeta[] = []): SessionTree {
  const byKey = new Map<string, SessionTreeNode>();
  const order = new Map<string, number>();
  const add = (session: SessionMeta, isContext: boolean) => {
    const key = refKey(session.ref);
    if (byKey.has(key)) return;
    byKey.set(key, { key, session, parentKey: null, children: [], context: isContext });
    if (!isContext) order.set(key, order.size);
  };
  for (const s of matches) add(s, false);
  for (const s of context) add(s, true);

  const parents = new Map<string, string>();
  for (const node of byKey.values()) {
    const ref = parentRef(node.session);
    const parentKey = ref ? refKey(ref) : '';
    if (parentKey && parentKey !== node.key && byKey.has(parentKey)) parents.set(node.key, parentKey);
  }
  // Cut every link inside a cycle; sessions pointing into one stay attached.
  const done = new Set<string>();
  for (const start of byKey.keys()) {
    const chain: string[] = [];
    const at = new Map<string, number>();
    for (let key: string | undefined = start; key && !done.has(key); key = parents.get(key)) {
      const seen = at.get(key);
      if (seen !== undefined) {
        for (const member of chain.slice(seen)) parents.delete(member);
        break;
      }
      at.set(key, chain.length);
      chain.push(key);
    }
    for (const key of chain) done.add(key);
  }

  const roots: SessionTreeNode[] = [];
  for (const node of byKey.values()) {
    const parentKey = parents.get(node.key);
    const parent = parentKey ? byKey.get(parentKey) : undefined;
    if (parent) {
      node.parentKey = parent.key;
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }

  // Rank: the position of the family's first matching member, if any.
  const rank = new Map<string, number>();
  const rankOf = (node: SessionTreeNode): number => {
    let best = order.get(node.key) ?? Infinity;
    for (const child of node.children) best = Math.min(best, rankOf(child));
    rank.set(node.key, best);
    return best;
  };
  for (const root of roots) rankOf(root);

  const prune = (nodes: SessionTreeNode[]): SessionTreeNode[] => {
    const kept: SessionTreeNode[] = [];
    for (const n of nodes) {
      if (rank.get(n.key) === Infinity) {
        forget(n);
        continue;
      }
      n.children = prune(n.children);
      n.children.sort(byStart);
      kept.push(n);
    }
    return kept;
  };
  const forget = (node: SessionTreeNode) => {
    byKey.delete(node.key);
    for (const child of node.children) forget(child);
  };
  const kept = prune(roots);
  kept.sort((a, b) => rank.get(a.key)! - rank.get(b.key)!);
  return { roots: kept, byKey };
}

function byStart(a: SessionTreeNode, b: SessionTreeNode): number {
  const at = Date.parse(a.session.createdAt) || 0;
  const bt = Date.parse(b.session.createdAt) || 0;
  return at - bt || (a.key < b.key ? -1 : a.key > b.key ? 1 : 0);
}

/**
 * Where a family's open or closed choice is kept. A session listed as context
 * keeps its own: closing it as an ordinary family earlier must not hide the
 * matches a later filter lists it for.
 */
export function expansionKey(node: SessionTreeNode): string {
  return node.context ? `context:${node.key}` : node.key;
}

/**
 * Whether a family is open. Families start closed; a context session starts
 * open, since it is only listed to show the subagents under it.
 */
export function isExpanded(node: SessionTreeNode, overrides: ReadonlyMap<string, boolean>): boolean {
  return node.children.length > 0 && (overrides.get(expansionKey(node)) ?? node.context);
}

/** The rows on screen: every root, and the children of each open family. */
export function flattenSessionTree(tree: SessionTree, overrides: ReadonlyMap<string, boolean>): SessionTreeRow[] {
  const rows: SessionTreeRow[] = [];
  const walk = (nodes: SessionTreeNode[], depth: number) => {
    for (const node of nodes) {
      const expanded = isExpanded(node, overrides);
      rows.push({ node, depth, expanded });
      if (expanded) walk(node.children, depth + 1);
    }
  };
  walk(tree.roots, 0);
  return rows;
}

/** The keys of a session's ancestors in the tree, nearest first. */
export function ancestorKeys(tree: SessionTree, key: string): string[] {
  const out: string[] = [];
  for (let at = tree.byKey.get(key)?.parentKey; at; at = tree.byKey.get(at)?.parentKey) out.push(at);
  return out;
}

/** Whether an ancestor is picked: deleting it deletes this session too. */
export function hasPickedAncestor(tree: SessionTree, key: string, picked: ReadonlySet<string>): boolean {
  return ancestorKeys(tree, key).some((k) => picked.has(k));
}

/**
 * The picked sessions to delete, parents before children. A session whose
 * ancestor is picked is left out: every agent's delete already takes a
 * session's subagents with it, and naming both blocks the child.
 */
export function pickedForDelete(tree: SessionTree, picked: ReadonlySet<string>): SessionRef[] {
  const out: SessionRef[] = [];
  const walk = (nodes: SessionTreeNode[]) => {
    for (const node of nodes) {
      if (picked.has(node.key)) out.push(node.session.ref);
      else walk(node.children);
    }
  };
  walk(tree.roots);
  return out;
}

/**
 * Resolves the ancestors of matching subagents that are not matches
 * themselves. `known` returns a session, null when it is known to be gone, or
 * undefined when it has not been looked up; the latter come back in
 * `missing` for the caller to fetch.
 */
export function collectContext(
  matches: SessionMeta[],
  known: (key: string) => SessionMeta | null | undefined,
): { context: SessionMeta[]; missing: SessionRef[] } {
  const listed = new Set(matches.map((s) => refKey(s.ref)));
  const context: SessionMeta[] = [];
  const missing = new Map<string, SessionRef>();
  const seen = new Set<string>();
  let pending = matches;
  while (pending.length > 0) {
    const next: SessionMeta[] = [];
    for (const s of pending) {
      const ref = parentRef(s);
      if (!ref) continue;
      const key = refKey(ref);
      if (listed.has(key) || seen.has(key)) continue;
      seen.add(key);
      const meta = known(key);
      if (meta) {
        context.push(meta);
        next.push(meta);
      } else if (meta === undefined) {
        missing.set(key, ref);
      }
    }
    pending = next;
  }
  return { context, missing: [...missing.values()] };
}

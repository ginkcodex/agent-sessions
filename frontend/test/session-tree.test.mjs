// The session list's tree: subagent sessions under the session that spawned
// them, context parents for filtered matches, and what a bulk delete sends.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  ancestorKeys,
  buildSessionTree,
  collectContext,
  expansionKey,
  flattenSessionTree,
  hasPickedAncestor,
  pickedForDelete,
} from '../src/lib/sessionTree.ts';

const s = (id, { parent, created = '2026-11-01T00:00:00Z', agent = 'claude-code' } = {}) => ({
  ref: { agent, id }, parentId: parent, title: id, createdAt: created, updatedAt: created,
  cwd: '/w', counts: { user: 0, assistant: 0 }, tokens: {},
});
const key = (id, agent = 'claude-code') => `${agent}:${id}`;
const shown = (rows) => rows.map((r) => `${'  '.repeat(r.depth)}${r.node.session.ref.id}${r.node.context ? '*' : ''}`);

test('subagents nest under their parent, started first listed first, and start closed', () => {
  const matches = [
    s('b-late', { parent: 'b', created: '2026-11-01T03:00:00Z' }),
    s('a'),
    s('b'),
    s('b-early', { parent: 'b', created: '2026-11-01T01:00:00Z' }),
    s('c'),
  ];
  const tree = buildSessionTree(matches);
  // b's family sits where its first member was listed: before a.
  assert.deepEqual(shown(flattenSessionTree(tree, new Map())), ['b', 'a', 'c']);
  assert.deepEqual(shown(flattenSessionTree(tree, new Map([[key('b'), true]]))), ['b', '  b-early', '  b-late', 'a', 'c']);
  assert.deepEqual(flattenSessionTree(tree, new Map([[key('a'), true]])).map((r) => r.expanded), [false, false, false], 'a childless row never opens');
});

test('parents only link within one agent, and a subagent without a listed parent stands alone', () => {
  const tree = buildSessionTree([s('p'), s('x', { parent: 'p', agent: 'codex' }), s('orphan', { parent: 'gone' })]);
  assert.deepEqual(shown(flattenSessionTree(tree, new Map([[key('p'), true]]))), ['p', 'x', 'orphan']);
});

test('nesting goes as deep as the agents do, and every row knows its ancestors', () => {
  const tree = buildSessionTree([s('root'), s('kid', { parent: 'root' }), s('grandkid', { parent: 'kid' })]);
  const open = new Map([[key('root'), true], [key('kid'), true]]);
  assert.deepEqual(flattenSessionTree(tree, open).map((r) => r.depth), [0, 1, 2]);
  assert.deepEqual(ancestorKeys(tree, key('grandkid')), [key('kid'), key('root')]);
  assert.deepEqual(ancestorKeys(tree, key('root')), []);
});

test('a parent cycle is cut instead of hanging the list', () => {
  const tree = buildSessionTree([s('a', { parent: 'b' }), s('b', { parent: 'a' }), s('c', { parent: 'a' })]);
  const rows = flattenSessionTree(tree, new Map([[key('a'), true]]));
  assert.deepEqual(shown(rows), ['a', '  c', 'b']);
});

test('a matching subagent brings its non-matching ancestors along, open and marked as context', () => {
  const kid = s('kid', { parent: 'mid' });
  const store = new Map([[key('mid'), s('mid', { parent: 'top' })], [key('top'), s('top')], [key('lost'), null]]);
  const first = collectContext([kid, s('orphan', { parent: 'lost' })], (k) => store.get(k));
  assert.deepEqual(first.context.map((m) => m.ref.id), ['mid', 'top']);
  assert.deepEqual(first.missing, [], 'a parent known to be gone is not asked for again');

  const unknown = collectContext([kid], () => undefined);
  assert.deepEqual(unknown.missing, [{ agent: 'claude-code', id: 'mid' }]);

  const tree = buildSessionTree([kid], first.context);
  assert.deepEqual(shown(flattenSessionTree(tree, new Map())), ['top*', '  mid*', '    kid']);
  const top = tree.byKey.get(key('top'));
  assert.deepEqual(shown(flattenSessionTree(tree, new Map([[expansionKey(top), false]]))), ['top*']);
  // Closing top as an ordinary family earlier must not hide what a filter
  // now lists it for.
  assert.deepEqual(shown(flattenSessionTree(tree, new Map([[key('top'), false]]))), ['top*', '  mid*', '    kid']);
  assert.notEqual(expansionKey(top), expansionKey(buildSessionTree([s('top')]).byKey.get(key('top'))));
});

test('a context session that no longer leads to a match is dropped', () => {
  const tree = buildSessionTree([s('a')], [s('stale')]);
  assert.deepEqual(shown(flattenSessionTree(tree, new Map())), ['a']);
  assert.equal(tree.byKey.has(key('stale')), false);
});

test('picking a parent covers its subagents, and the delete names only the parent', () => {
  const tree = buildSessionTree([s('p'), s('kid', { parent: 'p' }), s('grandkid', { parent: 'kid' }), s('q')]);
  const picked = new Set([key('grandkid'), key('p'), key('q')]);
  assert.deepEqual(pickedForDelete(tree, picked).map((r) => r.id), ['p', 'q']);
  assert.equal(hasPickedAncestor(tree, key('grandkid'), picked), true);
  assert.equal(hasPickedAncestor(tree, key('p'), picked), false);
  assert.deepEqual(pickedForDelete(tree, new Set([key('kid')])).map((r) => r.id), ['kid']);
});

test('the list renders the tree, keeps search keys in the field, and deletes the deduplicated pick', () => {
  const list = readFileSync(new URL('../src/lib/components/sessionlist/SessionList.svelte', import.meta.url), 'utf8');
  assert.match(list, /<VirtualList bind:this=\{virtualList\} items=\{rows\}[^>]*role="tree"/);
  assert.match(list, /e\.key === 'ArrowRight' \|\| e\.key === 'ArrowLeft'\) && fromRows\(e\.target\)/);
  assert.match(list, /manage\.requestDelete\(toDelete\)/);
  assert.match(list, /covered=\{hasPickedAncestor\(tree, row\.node\.key, picked\)\}/);
  // Revealing a selection once must not undo a later collapse or scroll on reloads.
  assert.match(list, /if \(key === revealedKey\) return;/);
  // Every open or closed choice is stored under the node's expansion key.
  assert.doesNotMatch(list, /\.set\(\w+(\.node)?\.key, /);
  assert.match(list, /expansion = new Map\(expansion\)\.set\(expansionKey\(node\), open\)/);
});

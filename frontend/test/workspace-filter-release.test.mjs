// The group tree's empty space releases the workspace filter: a click below
// or between the rows, or Escape in the tree, clears the selected group.
// The row that filters the list shows as selected; the row that only holds
// the open session shows as current, quieter.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import { compile } from 'svelte/compiler';
import { isEmptyTreeClick } from '../src/lib/tree.ts';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
// jsdom lays nothing out: the tree's reveal has nowhere to scroll.
window.Element.prototype.scrollIntoView = () => {};
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });

const client = (path) => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const lib = (path) => JSON.stringify(new URL(`../src/lib/${path}.ts`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
const { proxy } = await import(new URL('../node_modules/svelte/src/internal/client/index.js', import.meta.url).href);

test('isEmptyTreeClick: empty space, not rows, controls or menus', () => {
  const doc = new JSDOM(`<div id="tree" role="tree">
    <p id="message">No sessions</p>
    <ul id="list"><li id="item"><div class="node-row" id="row"><span id="label">api</span><button id="star">*</button></div>
      <div role="menu" id="menu"><button role="menuitem" id="menuitem">Go to File…</button></div></li></ul>
    <a id="link" href="#">x</a><input id="input"></div>`).window.document;
  const at = (id) => doc.getElementById(id);
  for (const id of ['tree', 'list', 'item', 'message']) assert.equal(isEmptyTreeClick(at(id)), true, id);
  for (const id of ['row', 'label', 'star', 'menu', 'menuitem', 'link', 'input']) assert.equal(isEmptyTreeClick(at(id)), false, id);
  assert.equal(isEmptyTreeClick(null), false);
  assert.equal(isEmptyTreeClick(doc.createTextNode('x')), false, 'a target without closest is not counted');
});

function compiled(name, replacements) {
  const url = new URL(`../src/lib/components/${name}.svelte`, import.meta.url);
  let code = compile(readFileSync(url, 'utf8'), { generate: 'client', filename: fileURLToPath(url), css: 'external' }).js.code;
  code = code
    .replaceAll("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replaceAll("'svelte/internal/client'", client('internal/client/index.js'))
    .replaceAll("'svelte/internal/flags/legacy'", client('internal/flags/legacy.js'))
    .replaceAll("from 'svelte';", `from ${client('index-client.js')};`);
  for (const [from, to] of replacements) {
    const next = typeof from === 'string' ? code.replace(from, to) : code.replace(from, to);
    assert.notEqual(next, code, `replaced ${from}`);
    code = next;
  }
  return import(`data:text/javascript,${encodeURIComponent(code)}`).then((m) => m.default);
}

const fx = (globalThis.__wfr = {});
const store = (name) => `const ${name} = new Proxy({}, { get: (_, k) => globalThis.__wfr.${name}[k], set: (_, k, v) => { globalThis.__wfr.${name}[k] = v; return true; } });`;
const Row = await compiled('sidebar/GroupNodeItem', [
  ["import Icon from '../common/Icon.svelte';", 'const Icon = () => {};'],
  ["import GroupNodeItem from './GroupNodeItem.svelte';", 'const GroupNodeItem = (...a) => globalThis.__wfr.Row(...a);'],
  ["import AgentIcon from '../common/AgentIcon.svelte';", 'const AgentIcon = () => {};'],
  [/import ContextMenu.*?from '..\/common\/ContextMenu.svelte';/, 'const ContextMenu = () => {};'],
  ["'../../tree'", lib('tree')],
  ["import { importer } from '../../stores/importer.svelte';", 'const importer = {};'],
  ["import { files } from '../../stores/files.svelte';", 'const files = {};'],
  ["import { newSession } from '../../stores/newSession.svelte';", 'const newSession = { available: false };'],
  ["'../../newSession'", lib('newSession')],
  ["import { workspaces } from '../../stores/workspaces.svelte';", 'const workspaces = { directories: [] };'],
  ["import { workspaceStars } from '../../stores/workspaceStars.svelte';", 'const workspaceStars = { isStarred: () => false, toggle() {} };'],
]);
fx.Row = Row;
const Tree = await compiled('sidebar/GroupTree', [
  ["import Icon from '../common/Icon.svelte';", 'const Icon = () => {};'],
  ["import { appState } from '../../stores/appState.svelte';", store('appState')],
  ["import { discovery } from '../../stores/discovery.svelte';", 'const discovery = { outcome: "unavailable", status: null };'],
  ["'../../discovery'", lib('discovery')],
  ["import { manage } from '../../stores/manage.svelte';", 'const manage = { ageFilter: "any" };'],
  ["'../../manage'", lib('manage')],
  ["'../../tree'", lib('tree')],
  ["import { terminals } from '../../stores/terminals.svelte';", 'const terminals = { selectDirectory() {} };'],
  ["import { link } from '../../stores/link.svelte';", 'const link = {};'],
  ["import GroupNodeItem from './GroupNodeItem.svelte';", 'const GroupNodeItem = globalThis.__wfr.Row;'],
  ["import { workspaceStars } from '../../stores/workspaceStars.svelte';", 'const workspaceStars = { onlyStarred: false, visibleGroups: (g) => g };'],
]);

const dir = (cwd, count = 1) => ({ key: `dir:${cwd}`, label: cwd, cwd, sessionCount: count, sessions: [{ agent: 'codex', id: cwd.slice(1) }] });

function openTree({ selectedGroupKey = 'dir:/api', open = { agent: 'codex', id: 'api' } } = {}) {
  const calls = [];
  fx.appState = proxy({
    groups: [dir('/api'), dir('/web')],
    filter: {},
    loadingGroups: false,
    sessions: [],
    selectedGroupKey,
    selectedSessionRef: open,
    collapsedKeys: new Set(),
    defaultCollapsed: new Set(),
    async selectGroup(key, record) {
      calls.push(['selectGroup', key, record]);
      fx.appState.selectedGroupKey = key;
    },
    async selectSession(ref) {
      calls.push(['selectSession', ref]);
    },
  });
  const target = document.createElement('div');
  document.body.appendChild(target);
  const component = mount(Tree, { target, props: {} });
  flushSync();
  const row = (cwd) => [...target.querySelectorAll('.node-row')].find((r) => r.textContent.includes(cwd.slice(1)));
  const look = (cwd) => (row(cwd).classList.contains('selected') ? 'selected' : row(cwd).classList.contains('current') ? 'current' : 'plain');
  const tree = target.querySelector('[role="tree"]');
  return {
    calls, tree, row, look,
    click(el) { el.dispatchEvent(new window.MouseEvent('click', { bubbles: true })); flushSync(); },
    key(el, key) { el.dispatchEvent(new window.KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })); flushSync(); },
    close() { unmount(component); target.remove(); },
  };
}

test('a click on empty space releases the workspace filter and keeps the open session', async () => {
  const t = openTree();
  try {
    assert.equal(t.look('/api'), 'selected', 'the filtering row');
    t.click(t.tree);
    assert.deepEqual(t.calls, [['selectGroup', null, 'push']], 'released as a navigation step; no session change');
    assert.equal(t.look('/api'), 'current', 'still marks the open session, quieter');
    assert.equal(t.look('/web'), 'plain');

    t.click(t.tree);
    assert.equal(t.calls.length, 1, 'nothing to release any more');
  } finally { t.close(); }
});

test('a gap between rows releases the filter; rows and their buttons do not', async () => {
  const t = openTree();
  try {
    t.click(t.row('/api').querySelector('.star-btn'));
    assert.deepEqual(t.calls, [], 'the star keeps its own click');
    t.click(t.row('/api'));
    await new Promise((r) => setImmediate(r));
    assert.deepEqual(t.calls[0], ['selectGroup', 'dir:/api', 'push'], 'a row click selects as before');
    t.calls.length = 0;
    t.click(t.tree.querySelector('.group-node-container'));
    assert.deepEqual(t.calls, [['selectGroup', null, 'push']], 'the item around a row, outside it, is empty space');
  } finally { t.close(); }
});

test('Escape in the tree releases the filter', () => {
  const t = openTree();
  try {
    t.key(t.row('/web'), 'Enter');
    t.calls.length = 0;
    fx.appState.selectedGroupKey = 'dir:/web';
    flushSync();
    t.key(t.row('/web'), 'Escape');
    assert.deepEqual(t.calls, [['selectGroup', null, 'push']]);
    t.key(t.tree, 'Escape');
    assert.equal(t.calls.length, 1, 'no filter, nothing to do');
  } finally { t.close(); }
});

test('the open session\'s group is current, not selected, when another group filters', () => {
  const t = openTree({ selectedGroupKey: 'dir:/web' });
  try {
    // The open session lives in /api: the highlight follows it, quietly.
    assert.equal(t.look('/api'), 'current');
    assert.equal(t.look('/web'), 'plain');
  } finally { t.close(); }
  const none = openTree({ selectedGroupKey: null, open: null });
  try {
    assert.equal(none.look('/api'), 'plain');
    none.click(none.tree);
    assert.deepEqual(none.calls, [], 'no filter to release');
  } finally { none.close(); }
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import { compile, compileModule } from 'svelte/compiler';
import ts from 'typescript';
import { filterStarredWorkspaces, loadWorkspaceStars, workspacePath, workspaceStarKey, WORKSPACE_STARS_KEY } from '../src/lib/workspaceStars.ts';
import { loadServerComponent, fixtureStore, renderWithFixture } from './helpers.mjs';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
const client = path => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const lib = path => JSON.stringify(new URL(`../src/lib/${path}.ts`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
function clientImports(code) {
  return code.replaceAll("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replaceAll("'svelte/internal/client'", client('internal/client/index.js'))
    .replaceAll("'svelte/internal/flags/legacy'", client('internal/flags/legacy.js'));
}
const link = globalThis.__starLink = { dataHost: undefined, stale: false, locked: false };
const storeURL = new URL('../src/lib/stores/workspaceStars.svelte.ts', import.meta.url);
const storeJS = ts.transpileModule(readFileSync(storeURL, 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
}).outputText;
let storeCode = clientImports(compileModule(storeJS, { generate: 'client', filename: 'workspaceStars.svelte.js' }).js.code)
  .replace("import { link } from './link.svelte';", 'const link = globalThis.__starLink;')
  .replace("'../workspaceStars'", lib('workspaceStars'));
const { WorkspaceStarsStore } = await import(`data:text/javascript,${encodeURIComponent(storeCode)}`);

function storage(initial = {}) {
  const data = new Map(Object.entries(initial));
  globalThis.localStorage = { getItem: key => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) };
  link.dataHost = undefined; link.stale = false; link.locked = false;
  return data;
}
const ref = id => ({ agent: 'test', id });
const session = (id, cwd, children = []) => ({ key: id, kind: 'session', label: id, cwd, children, sessions: [ref(id)], sessionCount: 1 });
const directory = (cwd, children = []) => ({ key: `dir:${cwd}`, kind: 'directory', label: cwd, cwd, children, sessions: children.flatMap(n => n.sessions), sessionCount: children.length });

test('stars persist by data host and canonical workspace path, never by group key', () => {
  storage();
  const store = new WorkspaceStarsStore();
  store.toggle('/work/Important');
  assert.equal(store.isStarred('/work/Important/'), true);
  assert.equal(store.isStarred('/work/important'), false, 'POSIX case remains significant');
  assert.equal(new WorkspaceStarsStore().isStarred('/work/Important'), true);
  link.dataHost = 'wsl:Ubuntu';
  assert.equal(store.isStarred('/work/Important'), false);
  store.toggle('/work/Important');
  link.dataHost = 'ssh:prod';
  assert.equal(store.isStarred('/work/Important'), false);
  link.dataHost = undefined;
  store.toggle('/work/Important');
  assert.equal(store.isStarred('/work/Important'), false);
  link.dataHost = 'wsl:Ubuntu';
  assert.equal(new WorkspaceStarsStore().isStarred('/work/Important'), true);
  store.toggle('C:\\Users\\Gink\\Project\\');
  assert.equal(store.isStarred('c:/users/gink/project'), true);
  assert.equal(workspacePath('/'), '/');
  assert.equal(workspacePath('\\\\Server\\Share\\Project'), workspacePath('//server/share/project/'));
  assert.equal(workspacePath('C:\\'), workspacePath('c:/'));
  assert.equal(workspaceStarKey('/a', '__proto__'), '["__proto__","/a"]');
});

test('invalid/absent storage is tolerated, transient filtering is not persisted, and stale links cannot change stars', () => {
  const good = workspaceStarKey('/good');
  for (const value of ['broken', '{}', 'null', JSON.stringify([null, 1, 'bad', '[1,2]', '["","/good"]', good])]) {
    storage({ [WORKSPACE_STARS_KEY]: value });
    assert.deepEqual(loadWorkspaceStars(), value.startsWith('[') ? [good] : []);
  }
  storage();
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, get() { throw new Error('disabled'); } });
  try {
    const store = new WorkspaceStarsStore();
    store.toggle('/memory');
    assert.equal(store.isStarred('/memory'), true);
    store.onlyStarred = true;
    assert.equal(new WorkspaceStarsStore().onlyStarred, false);
    link.stale = true; store.toggle('/memory');
    assert.equal(store.isStarred('/memory'), true);
    link.stale = false; link.locked = true; store.toggle('/locked');
    assert.equal(store.isStarred('/locked'), false);
  } finally { delete globalThis.localStorage; link.locked = false; }
  assert.deepEqual(loadWorkspaceStars(), []);
});

test('directory and agent grouping keep full matching workspaces, prune other agents and update context counts', () => {
  const a = directory('/a', [session('a', '/a/subdir')]);
  const b = directory('/b', [session('b', '/b')]);
  const match = cwd => cwd === '/a';
  assert.deepEqual(filterStarredWorkspaces([a, b], match), [a], 'directory identity wins over leaf cwd');
  assert.deepEqual(filterStarredWorkspaces([b], cwd => cwd === '/b/sub'), []);
  const agent = { key: 'agent', kind: 'agent', sessions: [ref('a'), ref('b')], sessionCount: 2, children: [a, b] };
  const source = structuredClone(agent);
  const filtered = filterStarredWorkspaces([agent, { ...agent, key: 'other', children: [b] }], match);
  assert.equal(filtered.length, 1);
  assert.deepEqual(filtered[0].children, [a]);
  assert.deepEqual(filtered[0].sessions, [ref('a')]);
  assert.equal(filtered[0].sessionCount, 1);
  assert.deepEqual(agent, source, 'source selection and catalog remain unchanged');
  assert.deepEqual(filterStarredWorkspaces([directory('/empty')], cwd => cwd === '/empty').length, 1);
});

test('flat grouping keeps starred families and context for independently matching subagents', () => {
  const child = session('child', '/a');
  const parent = session('parent', '/b', [child, session('other', '/b')]);
  const filtered = filterStarredWorkspaces([parent, session('unrelated', '/b')], cwd => cwd === '/a');
  assert.equal(filtered[0].key, 'parent');
  assert.deepEqual(filtered[0].children, [child]);
  assert.deepEqual(filterStarredWorkspaces([parent], cwd => cwd === '/b'), [parent]);
  assert.deepEqual(filterStarredWorkspaces([parent], () => false), []);
});

test('store filters only the supplied catalog and preserves all groups when disabled', () => {
  storage();
  const store = new WorkspaceStarsStore();
  store.toggle('/a');
  const groups = [directory('/a'), directory('/b')];
  assert.equal(store.visibleGroups(groups), groups);
  store.onlyStarred = true;
  assert.deepEqual(store.visibleGroups(groups), [groups[0]]);
  assert.deepEqual(store.visibleGroups([groups[1]]), [], 'stars do not bypass directory/session filters');
  assert.equal(store.isStarred('/a'), true, 'absence never prunes a saved star');
});

async function component(name, replacements = []) {
  const url = new URL(`../src/lib/components/${name}.svelte`, import.meta.url);
  let code = clientImports(compile(readFileSync(url, 'utf8'), { generate: 'client', filename: fileURLToPath(url), css: 'external' }).js.code);
  for (const [from, to] of [
    ["import { workspaceStars } from '../../stores/workspaceStars.svelte';", 'const workspaceStars = new Proxy({}, { get: (_, key) => { const s = globalThis.__starStore; const v = s[key]; return typeof v === "function" ? v.bind(s) : v; }, set: (_, key, value) => { globalThis.__starStore[key] = value; return true; } });'],
    ["import Icon from '../common/Icon.svelte';", 'const Icon = () => {};'],
    ["import { appState } from '../../stores/appState.svelte';", 'const appState = new Proxy({}, { get: (_, key) => globalThis.__starAppState[key] });'],
    ["'svelte'", client('index-client.js')],
    ...replacements,
  ]) code = code.replaceAll(from, to);
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}
const Row = await component('sidebar/GroupNodeItem', [
  ["import GroupNodeItem from './GroupNodeItem.svelte';", 'const GroupNodeItem = () => {};'],
  ["import AgentIcon from '../common/AgentIcon.svelte';", 'const AgentIcon = () => {};'],
  ["import ContextMenu from '../common/ContextMenu.svelte';", 'const ContextMenu = () => {};'],
  ["'../../tree'", lib('tree')],
  ["import { importer } from '../../stores/importer.svelte';", 'const importer = {};'],
  ["import { files } from '../../stores/files.svelte';", 'const files = {};'],
  ["import { newSession } from '../../stores/newSession.svelte';", 'const newSession = { available: false };'],
  ["'../../newSession'", lib('newSession')],
  ["import { workspaces } from '../../stores/workspaces.svelte';", 'const workspaces = { directories: [] };'],
]);
const Filter = await component('sidebar/PathFilter');

function open(Component, props = {}) {
  storage();
  const store = globalThis.__starStore = new WorkspaceStarsStore();
  globalThis.__starAppState = { filter: {} };
  const target = document.createElement('div'); document.body.append(target);
  const instance = mount(Component, { target, props }); flushSync();
  return { target, store, close() { unmount(instance); target.remove(); } };
}

test('row star mouse and keyboard events never select or collapse a workspace, and state is reactive', () => {
  let selected = 0, collapsed = 0;
  const row = open(Row, { node: directory('/important'), selectedKey: null, collapsedKeys: new Set(), defaultCollapsed: new Set(),
    onSelect() { selected++; }, onToggleCollapse() { collapsed++; } });
  try {
    const button = row.target.querySelector('.star-btn');
    assert.equal(button.getAttribute('aria-pressed'), 'false');
    for (const key of ['Enter', ' ']) {
      button.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
      assert.equal(selected, 0);
    }
    button.click(); flushSync();
    assert.equal(selected, 0); assert.equal(collapsed, 0);
    assert.equal(button.getAttribute('aria-pressed'), 'true');
    assert.match(button.getAttribute('aria-label'), /Unstar workspace/);
    button.click(); flushSync();
    assert.equal(button.getAttribute('aria-pressed'), 'false');
    row.target.querySelector('.node-row').click();
    assert.equal(selected, 1, 'ordinary row navigation still works');
  } finally { row.close(); }
});

test('star filter is next to directory search and toggles without touching its filter', () => {
  const filter = open(Filter);
  try {
    const input = filter.target.querySelector('input[aria-label="Filter by directory"]');
    const button = filter.target.querySelector('button[aria-label="Show only starred workspaces"]');
    assert.ok(input); assert.ok(button);
    button.click(); flushSync();
    assert.equal(filter.store.onlyStarred, true);
    assert.equal(button.getAttribute('aria-pressed'), 'true');
    assert.equal(button.title, 'Show all workspaces');
    assert.deepEqual(globalThis.__starAppState.filter, {});
    button.click(); flushSync();
    assert.equal(button.getAttribute('aria-pressed'), 'false');
  } finally { filter.close(); }
});

const Tree = await loadServerComponent(new URL('../src/lib/components/sidebar/GroupTree.svelte', import.meta.url), [
  ["import { workspaceStars } from '../../stores/workspaceStars.svelte';", 'const workspaceStars = new Proxy({}, { get: (_, key) => { const s = globalThis.__componentFixture.workspaceStars; const v = s[key]; return typeof v === "function" ? v.bind(s) : v; } });'],
  ["import { appState } from '../../stores/appState.svelte';", fixtureStore('appState')],
  ["import { discovery } from '../../stores/discovery.svelte';", 'const discovery = { outcome: "unavailable", status: null };'],
  ["'../../discovery'", lib('discovery')], ["'../../tree'", lib('tree')], ["'../../manage'", lib('manage')],
  ["import { manage } from '../../stores/manage.svelte';", 'const manage = {};'],
  ["import { terminals } from '../../stores/terminals.svelte';", 'const terminals = {};'],
  ["import { link } from '../../stores/link.svelte';", 'const link = {};'],
  ["import GroupNodeItem from './GroupNodeItem.svelte';", 'const GroupNodeItem = (renderer, props) => renderer.push(`<li>${props.node.key}</li>`);'],
]);
test('tree renders only starred rows or an actionable empty state', async () => {
  storage();
  const store = new WorkspaceStarsStore(); store.onlyStarred = true;
  const fixture = { workspaceStars: store, appState: { groups: [directory('/a'), directory('/b')], filter: {}, selectedSessionRef: null, collapsedKeys: new Set(), defaultCollapsed: new Set() } };
  let html = await renderWithFixture(Tree, fixture);
  assert.match(html, /No starred workspaces match/); assert.match(html, /Show all workspaces/);
  assert.doesNotMatch(html, /No Agents Detected/);
  store.toggle('/a');
  html = await renderWithFixture(Tree, fixture);
  assert.match(html, /dir:\/a/); assert.doesNotMatch(html, /dir:\/b/);
});

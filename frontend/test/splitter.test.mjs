import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import ts from 'typescript';
import { loadServerModule } from './helpers.mjs';

const source = readFileSync(new URL('../src/lib/components/common/Splitter.svelte', import.meta.url), 'utf8');
function splitter(direction) {
  const script = source.match(/<script lang="ts">([\s\S]*?)<\/script>/)[1].replace(/^\s*import .*;$/gm, '');
  const js = ts.transpileModule(script, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
  const resized = [], equalized = [];
  const handlers = new Function('$props', '$state', `${js}; return { handleKeyDown };`)(
    () => ({ direction, onResize: delta => resized.push(delta), onEqualize: edge => equalized.push(edge) }),
    value => value,
  );
  const dblclick = source.match(/ondblclick=\{\(event\) => \{([\s\S]*?)\}\}/)[1];
  return { ...handlers, resized, equalized, doubleClick: new Function('event', 'onEqualize', dblclick) };
}

for (const direction of ['horizontal', 'vertical']) {
  test(`${direction} splitter equalizes on double-click and Enter, keeping arrow resizing`, () => {
    const control = splitter(direction);
    const edge = {};
    let prevented = 0;
    const event = { currentTarget: edge, preventDefault: () => prevented++ };
    control.doubleClick(event, edge => control.equalized.push(edge));
    control.handleKeyDown({ ...event, key: 'Enter' });
    assert.deepEqual(control.equalized, [edge, edge]);
    assert.equal(prevented, 2);
    const keys = direction === 'horizontal' ? ['ArrowLeft', 'ArrowRight'] : ['ArrowUp', 'ArrowDown'];
    control.handleKeyDown({ ...event, key: keys[0] });
    control.handleKeyDown({ ...event, key: keys[1], shiftKey: true });
    assert.deepEqual(control.resized, [-10, 50]);
  });
}

const { PreferencesStore } = await loadServerModule(new URL('../src/lib/stores/preferences.svelte.ts', import.meta.url),
  ['transcript', 'date', 'terminalFont'].map(name => [`'../${name}'`, JSON.stringify(new URL(`../src/lib/${name}.ts`, import.meta.url).href)]),
);
test('transcript equalizes against both navigation columns, not just the session list', () => {
  const store = new PreferencesStore();
  store.equalizeNavigationAndTranscript(1400);
  assert.deepEqual(store.widths, { sidebar: 260, sessionList: 440 });
  assert.equal(store.widths.sidebar + store.widths.sessionList, 1400 / 2);
});

test('navigation equalization adjusts the directory only when the list hits its minimum', () => {
  const store = new PreferencesStore();
  store.equalizeNavigationAndTranscript(1800);
  assert.deepEqual(store.widths, { sidebar: 260, sessionList: 640 });
  store.widths = { sidebar: 450, sessionList: 550 };
  store.equalizeNavigationAndTranscript(1000);
  assert.deepEqual(store.widths, { sidebar: 240, sessionList: 260 });
});

test('navigation equalization respects minimum sizes without fixed maximums', () => {
  const store = new PreferencesStore();
  store.equalizeNavigationAndTranscript(800);
  assert.deepEqual(store.widths, { sidebar: 200, sessionList: 260 });
  store.equalizeNavigationAndTranscript(2400);
  assert.deepEqual(store.widths, { sidebar: 200, sessionList: 1000 });
});

test('sidebar and session list equalize together within shared size limits', () => {
  const store = new PreferencesStore();
  store.equalizeSidebarAndSessionList();
  assert.deepEqual(store.widths, { sidebar: 300, sessionList: 300 });
  store.widths = { sidebar: 200, sessionList: 260 };
  store.equalizeSidebarAndSessionList();
  assert.deepEqual(store.widths, { sidebar: 260, sessionList: 260 });
  store.widths = { sidebar: 450, sessionList: 550 };
  store.equalizeSidebarAndSessionList();
  assert.deepEqual(store.widths, { sidebar: 500, sessionList: 500 });
});

test('manual resizing allows wide navigation columns and retains minimums', () => {
  const store = new PreferencesStore();
  store.setSidebarWidth(900);
  store.setSessionListWidth(1100);
  assert.deepEqual(store.widths, { sidebar: 900, sessionList: 1100 });
  store.setSidebarWidth(1);
  store.setSessionListWidth(1);
  assert.deepEqual(store.widths, { sidebar: 200, sessionList: 260 });
});

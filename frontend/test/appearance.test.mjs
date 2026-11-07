import test from 'node:test';
import assert from 'node:assert/strict';
import { FONT_SIZES, ROOT_VARS, appearanceVars, applyRootVars, fontSize, sessionFontVar, stepSize } from '../src/lib/appearance.ts';
import { SYSTEM_MONO_FONTS } from '../src/lib/terminalFont.ts';
import { loadServerModule } from './helpers.mjs';

const storeURL = new URL('../src/lib/stores/preferences.svelte.ts', import.meta.url);
const DEFAULTS = { editorFont: '', editorFontSize: null, docsFontSize: null };

test('the defaults set no custom properties at all', () => {
  assert.deepEqual(appearanceVars(DEFAULTS), {});
  assert.equal(sessionFontVar(null), undefined);
});

test('chosen fonts and sizes become root custom properties', () => {
  assert.deepEqual(appearanceVars({ editorFont: 'Fira Code', editorFontSize: 16, docsFontSize: 17.5 }), {
    '--editor-font-family': `"Fira Code", ${SYSTEM_MONO_FONTS}`,
    '--editor-font-size': '16px',
    '--docs-font-size': '17.5px',
  });
  assert.equal(sessionFontVar(18), '18px');
  // Invalid values from anywhere still fall back to the default.
  assert.deepEqual(appearanceVars({ editorFont: 'bad\nfont', editorFontSize: 99, docsFontSize: Number.NaN }), {});
  assert.equal(sessionFontVar(30), undefined);
});

test('sizes are kept to 0.5px within their range; anything else is the default', () => {
  assert.equal(fontSize('session', 15.3), 15.5);
  assert.equal(fontSize('session', '16'), 16);
  assert.equal(fontSize('session', '14.5'), 14.5);
  assert.equal(fontSize('editor', 10), 10);
  assert.equal(fontSize('editor', 28), 28);
  for (const bad of ['abc', '-3', -3, '1e9', 1e9, '', ' 14', '14px', null, undefined, Number.NaN, Infinity, {}, '0x10']) {
    assert.equal(fontSize('session', bad), null, String(bad));
  }
  assert.equal(fontSize('session', 25), null);
  assert.equal(fontSize('docs', 25), 25);
  assert.equal(fontSize('docs', 10.5), null);
  assert.equal(fontSize('editor', 9.5), null);
});

test('steps are whole pixels and stay within the range', () => {
  assert.equal(stepSize('session', null, 1), 15);
  assert.equal(stepSize('session', null, -1), 14);
  assert.equal(stepSize('editor', null, 1), 15);
  assert.equal(stepSize('editor', null, -1), 14);
  assert.equal(stepSize('docs', 16, 1), 17);
  assert.equal(stepSize('docs', 16, -1), 15);
  for (const [kind, { min, max }] of Object.entries(FONT_SIZES)) {
    assert.equal(stepSize(kind, max, 1), max);
    assert.equal(stepSize(kind, min, -1), min);
  }
});

test('root properties are set, and removed again at the default', () => {
  const props = new Map();
  const style = { setProperty: (k, v) => props.set(k, v), removeProperty: (k) => props.delete(k) };
  applyRootVars(style, appearanceVars({ editorFont: 'Menlo', editorFontSize: 12, docsFontSize: 20 }));
  assert.deepEqual([...props.keys()].sort(), [...ROOT_VARS].sort());
  applyRootVars(style, appearanceVars({ ...DEFAULTS, docsFontSize: 20 }));
  assert.deepEqual(Object.fromEntries(props), { '--docs-font-size': '20px' });
  applyRootVars(style, appearanceVars(DEFAULTS));
  assert.equal(props.size, 0);
});

async function storeWith(values, { failWrites = false } = {}) {
  const { PreferencesStore } = await loadServerModule(storeURL, [
    ["'../terminalFont'", JSON.stringify(new URL('../src/lib/terminalFont.ts', import.meta.url).href)],
    ["'../transcript'", JSON.stringify(new URL('../src/lib/transcript.ts', import.meta.url).href)],
    ["'../date'", JSON.stringify(new URL('../src/lib/date.ts', import.meta.url).href)],
  ]);
  globalThis.localStorage = {
    getItem: (k) => values.get(k) ?? null,
    setItem: (k, v) => { if (failWrites) throw new Error('disabled'); values.set(k, v); },
    removeItem: (k) => { if (failWrites) throw new Error('disabled'); values.delete(k); },
  };
  return PreferencesStore;
}

test('font preferences persist, validate and reset', async () => {
  const previous = globalThis.localStorage;
  const values = new Map();
  try {
    const PreferencesStore = await storeWith(values);
    const prefs = new PreferencesStore();
    assert.deepEqual([prefs.editorFont, prefs.editorFontSize, prefs.docsFontSize, prefs.sessionFontSize], ['', null, null, null]);
    prefs.setEditorFont('Fira Code');
    prefs.setFontSize('editor', 16);
    prefs.setFontSize('docs', 17.3);
    prefs.setFontSize('session', 20);
    assert.equal(values.get('agent-sessions:editor-font'), 'Fira Code');
    assert.equal(values.get('agent-sessions:docs-font-size'), '17.5');

    const restored = new PreferencesStore();
    restored.init();
    assert.deepEqual([restored.editorFont, restored.editorFontSize, restored.docsFontSize, restored.sessionFontSize], ['Fira Code', 16, 17.5, 20]);

    // Invalid input is ignored and keeps the current choice.
    restored.setFontSize('session', 99);
    restored.setEditorFont('bad\u0007font');
    assert.equal(restored.sessionFontSize, 20);
    assert.equal(restored.editorFont, 'Fira Code');

    // Reset removes the stored value.
    restored.setFontSize('session', null);
    restored.setEditorFont('');
    assert.equal(restored.sessionFontSize, null);
    assert.equal(values.has('agent-sessions:session-font-size'), false);
    assert.equal(values.has('agent-sessions:editor-font'), false);
  } finally { globalThis.localStorage = previous; }
});

test('garbage in storage loads as the defaults', async () => {
  const previous = globalThis.localStorage;
  const values = new Map([
    ['agent-sessions:editor-font', 'bad\nfont'],
    ['agent-sessions:editor-font-size', 'abc'],
    ['agent-sessions:docs-font-size', '-3'],
    ['agent-sessions:session-font-size', '1e9'],
  ]);
  try {
    const PreferencesStore = await storeWith(values);
    const prefs = new PreferencesStore();
    prefs.init();
    assert.deepEqual([prefs.editorFont, prefs.editorFontSize, prefs.docsFontSize, prefs.sessionFontSize], ['', null, null, null]);
  } finally { globalThis.localStorage = previous; }
});

test('without storage the choice still applies for the run', async () => {
  const previous = globalThis.localStorage;
  try {
    const PreferencesStore = await storeWith(new Map(), { failWrites: true });
    const prefs = new PreferencesStore();
    prefs.setFontSize('session', 18);
    prefs.setEditorFont('Menlo');
    assert.equal(prefs.sessionFontSize, 18);
    assert.equal(prefs.editorFont, 'Menlo');
    prefs.setFontSize('session', null);
    assert.equal(prefs.sessionFontSize, null);
  } finally { globalThis.localStorage = previous; }
});

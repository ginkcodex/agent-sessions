import test from 'node:test';
import assert from 'node:assert/strict';
import { isTerminalFont, terminalFontFamily, SYSTEM_MONO_FONTS } from '../src/lib/terminalFont.ts';
import { loadServerModule } from './helpers.mjs';

const storeURL = new URL('../src/lib/stores/preferences.svelte.ts', import.meta.url);

test('terminal font quotes installed family names and retains a monospace fallback', () => {
  assert.equal(terminalFontFamily('', 'app-default'), 'app-default');
  assert.equal(terminalFontFamily('FiraCode Nerd Font Mono', 'app-default'), `"FiraCode Nerd Font Mono", ${SYSTEM_MONO_FONTS}`);
  assert.equal(terminalFontFamily('Font "quoted"', 'app-default'), `"Font \\"quoted\\"", ${SYSTEM_MONO_FONTS}`);
  assert.equal(isTerminalFont('bad\nfont'), false);
  assert.equal(isTerminalFont('x'.repeat(201)), false);
});

test('terminal font preference persists locally and survives unavailable storage', async () => {
  const { PreferencesStore } = await loadServerModule(storeURL, [
    ["'../terminalFont'", JSON.stringify(new URL('../src/lib/terminalFont.ts', import.meta.url).href)],
    ["'../transcript'", JSON.stringify(new URL('../src/lib/transcript.ts', import.meta.url).href)],
    ["'../date'", JSON.stringify(new URL('../src/lib/date.ts', import.meta.url).href)],
  ]);
  const previous = globalThis.localStorage;
  const values = new Map();
  globalThis.localStorage = { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, v) };
  try {
    const prefs = new PreferencesStore();
    prefs.setTerminalFont('FiraCode Nerd Font Mono');
    const restored = new PreferencesStore();
    restored.init();
    assert.equal(restored.terminalFont, 'FiraCode Nerd Font Mono');
    restored.setTerminalFont('bad\nfont');
    assert.equal(restored.terminalFont, 'FiraCode Nerd Font Mono');
    globalThis.localStorage.setItem = () => { throw new Error('disabled'); };
    restored.setTerminalFont('Menlo');
    assert.equal(restored.terminalFont, 'Menlo');
  } finally { globalThis.localStorage = previous; }
});

test('terminals open at the chosen size and take a new font or size at once', async () => {
  const { readFileSync } = await import('node:fs');
  const view = readFileSync(new URL('../src/lib/components/terminal/TerminalView.svelte', import.meta.url), 'utf8');
  assert.match(view, /return preferences\.terminalFontSize \?\? FONT_SIZES\.terminal\.default;/, 'null is 13px');
  // The font is loaded at the size the cells are measured at.
  assert.match(view, /document\.fonts\.load\(`\$\{size\}px \$\{font\}`\)/);
  assert.doesNotMatch(view, /fontSize: 13\b|`13px /, 'no fixed 13px left');
  assert.match(view, /new Terminal\(\{\s*fontFamily: font,\s*fontSize: size,/);
  // A change applies to the open terminal, then refits so the program learns its columns and rows.
  assert.match(view, /if \(!t \|\| \(t\.options\.fontFamily === font && t\.options\.fontSize === size\)\) return;/);
  assert.match(view, /t\.options\.fontFamily = font;\s*t\.options\.fontSize = size;\s*resize\(\);/);
  assert.match(view, /if \(tab\.status === 'running'\) processSize\?\.request\(term\.cols, term\.rows\);/);
});

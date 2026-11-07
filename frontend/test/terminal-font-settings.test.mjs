import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerComponent, loadTerminalFontSetting, renderWithFixture } from './helpers.mjs';

test('terminal font setting offers the app default, current selection and Nerd Font sample', async () => {
  const component = await loadTerminalFontSetting();
  const html = await renderWithFixture(component, { preferences: { terminalFont: 'FiraCode Nerd Font Mono' } });
  assert.match(html, /<h4[^>]*>Built-in terminal font<\/h4>/);
  // Until the installed fonts are listed, the chosen one is not called unavailable.
  assert.match(html, /aria-label="Built-in terminal font"[\s\S]*?class="trigger-label[^"]*">FiraCode Nerd Font Mono<\/span>/);
  const byDefault = await renderWithFixture(component, { preferences: { terminalFont: '' } });
  assert.match(byDefault, /class="trigger-label[^"]*">App default \(JetBrains Mono\)<\/span>/);
  assert.match(html, /No additional fonts are bundled or downloaded/);
  assert.match(html, /Terminal font preview/);
});

test('the terminal text size steps from 13px and the sample shows it', async () => {
  const component = await loadTerminalFontSetting();
  const sample = (html) => html.match(/<div class="sample[^"]*" aria-label="Terminal font preview"[^>]*>/)?.[0] ?? '';
  const stepper = (html) => html.match(/<div class="stepper[^"]*" role="group" aria-label="Terminal text size">[\s\S]*?<\/div>/)?.[0] ?? '';

  const byDefault = await renderWithFixture(component, { preferences: { terminalFont: '', terminalFontSize: null } });
  assert.match(byDefault, /<h4[^>]*>Built-in terminal text size<\/h4>/);
  assert.match(stepper(byDefault), />13px \(default\)</);
  assert.match(stepper(byDefault), /aria-label="Reset terminal text size" disabled/);
  assert.match(sample(byDefault), /font-size: 13px;/);

  const chosen = await renderWithFixture(component, { preferences: { terminalFont: '', terminalFontSize: 18 } });
  assert.match(stepper(chosen), />18px</);
  assert.doesNotMatch(stepper(chosen), /aria-label="Reset terminal text size" disabled/);
  assert.match(sample(chosen), /font-size: 18px;/);

  const largest = await renderWithFixture(component, { preferences: { terminalFont: '', terminalFontSize: 28 } });
  assert.match(stepper(largest), /aria-label="Increase terminal text size"[^>]*disabled/);
});

const monoURL = new URL('../src/lib/components/common/MonoFontSetting.svelte', import.meta.url);
const PICKER = {
  heading: 'Editor font', ariaLabel: 'Editor font', onChange() {}, hint: 'Pick one.',
  sampleLabel: 'Editor font preview', sampleLines: ['const a = 1;', '0O 1lI'],
};

test('the shared font picker lists installed fonts and marks a removed one unavailable', async () => {
  const component = await loadServerComponent(monoURL, [
    ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
    ["'../../terminalFont'", JSON.stringify(new URL('../../terminalFont.ts', monoURL).href)],
    ["import { installedFonts } from '../../stores/installedFonts.svelte';", fixtureStore('installedFonts')],
  ]);
  const listed = { installedFonts: { fonts: ['Fira Code', 'Menlo'], loading: false, error: '' } };
  const html = (await renderWithFixture(component, listed, { ...PICKER, value: 'Iosevka' })).replace(/<!--[\s\S]*?-->/g, '');
  assert.match(html, /<h4[^>]*>Editor font<\/h4>/);
  assert.match(html, /class="trigger-label[^"]*">Iosevka \(unavailable\)<\/span>/);
  assert.match(html, /The selected font is no longer installed/);
  assert.match(html, /aria-label="Editor font preview"[^>]*>[\s\S]*const a = 1;<br\/?>0O 1lI/);
  assert.match(html, /font-family: &quot;Iosevka&quot;, ui-monospace/);
  assert.match(html, />Refresh</);

  const installed = await renderWithFixture(component, listed, { ...PICKER, value: 'Menlo' });
  assert.doesNotMatch(installed, /unavailable|no longer installed/);
  const byDefault = await renderWithFixture(component, listed, { ...PICKER, value: '' });
  assert.match(byDefault, /class="trigger-label[^"]*">App default \(JetBrains Mono\)<\/span>/);
  assert.match(byDefault, /font-family: var\(--font-mono\)/);

  const failed = await renderWithFixture(component, { installedFonts: { fonts: [], loading: false, error: 'fc-list missing' } }, { ...PICKER, value: 'Iosevka' });
  assert.match(failed, /role="alert">Could not list installed fonts: fc-list missing/);
  assert.doesNotMatch(failed, /no longer installed/);
});

test('font pickers share one listing per Settings opening; Refresh lists again', async () => {
  const { loadServerModule } = await import('./helpers.mjs');
  const { InstalledFonts } = await loadServerModule(new URL('../src/lib/stores/installedFonts.svelte.ts', import.meta.url), [
    ["import { api } from '../api';", 'const api = {};'],
  ]);
  let calls = 0;
  let fail = false;
  const store = new InstalledFonts(async () => { calls++; if (fail) throw new Error('no fc-list'); return ['Fira Code']; });
  assert.equal(store.loading, true);
  await Promise.all([store.ensure(), store.ensure()]); // two pickers mount
  assert.equal(calls, 1);
  assert.deepEqual([store.fonts, store.loading, store.error], [['Fira Code'], false, '']);
  await store.ensure(); // switching sections in the same opening
  assert.equal(calls, 1);
  await store.refresh();
  assert.equal(calls, 2);
  store.markStale(); // Settings closed
  await store.ensure();
  assert.equal(calls, 3);
  fail = true;
  await store.refresh();
  assert.equal(store.error, 'no fc-list');
  assert.deepEqual(store.fonts, ['Fira Code']);
  fail = false;
  await store.ensure(); // a failed listing is tried again
  assert.equal(calls, 5);
  assert.equal(store.error, '');
});

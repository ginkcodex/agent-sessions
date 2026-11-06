import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerComponent, renderWithFixture } from './helpers.mjs';

const url = new URL('../src/lib/components/common/TerminalFontSetting.svelte', import.meta.url);

test('terminal font setting offers the app default, current selection and Nerd Font sample', async () => {
  const component = await loadServerComponent(url, [
    ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
    ["import { api } from '../../api';", 'const api = {};'],
    ["'../../terminalFont'", JSON.stringify(new URL('../../terminalFont.ts', url).href)],
    ["import { preferences } from '../../stores/preferences.svelte';", fixtureStore('preferences')],
  ]);
  const html = await renderWithFixture(component, { preferences: { terminalFont: 'FiraCode Nerd Font Mono' } });
  assert.match(html, /<h4[^>]*>Built-in terminal font<\/h4>/);
  // Until the installed fonts are listed, the chosen one is not called unavailable.
  assert.match(html, /aria-label="Built-in terminal font"[\s\S]*?class="trigger-label[^"]*">FiraCode Nerd Font Mono<\/span>/);
  const byDefault = await renderWithFixture(component, { preferences: { terminalFont: '' } });
  assert.match(byDefault, /class="trigger-label[^"]*">App default \(JetBrains Mono\)<\/span>/);
  assert.match(html, /No additional fonts are bundled or downloaded/);
  assert.match(html, /Terminal font preview/);
});

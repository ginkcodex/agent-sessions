// The About window: version line, and the Buy me a coffee button.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { render } from 'svelte/server';
import { loadServerComponent } from './helpers.mjs';
import { BUY_ME_A_COFFEE_URL, versionLabel } from '../src/lib/about.ts';

const url = new URL('../src/lib/components/common/AboutPage.svelte', import.meta.url);
const lib = (name) => JSON.stringify(new URL(`../src/lib/${name}.ts`, import.meta.url).href);
const component = (await loadServerComponent(url, [
  ["'../../about'", lib('about')],
])).default;

test('the version comes from the page query, and a dev build says so', () => {
  assert.equal(versionLabel('?version=1.2.2'), 'Version 1.2.2');
  assert.equal(versionLabel('?version=dev'), 'Development build');
  assert.equal(versionLabel(''), '');
});

test('the About page shows the app and a Buy me a coffee button for ginkcode', () => {
  assert.equal(BUY_ME_A_COFFEE_URL, 'https://buymeacoffee.com/ginkcode');
  const { body } = render(component, { props: { version: 'Version 1.2.2', icon: '/about-icon.png', openURL() {} } });
  assert.match(body, /<img class="app-icon[^"]*" src="\/about-icon\.png"/);
  assert.match(body, /<h1[^>]*>Agent Sessions<\/h1>/);
  assert.match(body, /class="version[^"]*">Version 1\.2\.2</);
  assert.match(body, /<button type="button" class="coffee[^"]*" title="https:\/\/buymeacoffee\.com\/ginkcode">[\s\S]*Buy me a coffee/);
  assert.match(body, /class="coffee[^"]*"[^>]*>\s*<svg class="icon[^"]*"[^>]*width="20"[\s\S]*?<path d="M10 2v2"/, 'a coffee cup leads the label');
  assert.doesNotMatch(render(component, { props: { version: '', icon: '/about-icon.png', openURL() {} } }).body, /class="version/);

  const source = readFileSync(url, 'utf8');
  assert.match(source, /onclick=\{\(\) => openURL\(BUY_ME_A_COFFEE_URL\)\}/);
});

test('about.html is built as a page of its own and opens links in the browser', () => {
  const vite = readFileSync(new URL('../vite.config.ts', import.meta.url), 'utf8');
  assert.match(vite, /about: fileURLToPath\(new URL\('\.\/about\.html'/);
  const entry = readFileSync(new URL('../src/about.ts', import.meta.url), 'utf8');
  assert.match(entry, /if \(isDesktop\(\)\) \{\s*openDesktopURL\(url\)/);
  // macOS shows the icon on its tile, as in the Dock; Windows and Linux the bare mark.
  assert.match(entry, /icon: \/Mac\/\.test\(navigator\.userAgent\) \? macIcon : icon/);
});

test('on Windows and Linux the sidebar logo opens About; macOS keeps it in the app menu', () => {
  const app = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8');
  assert.match(app, /let canShowAbout = \$derived\(isWails && !isMac && typeof api\.showAbout === 'function'\)/);
  assert.match(app, /\{#if canShowAbout\}\s*<button type="button" class="app-icon-button" title="About Agent Sessions" aria-label="About Agent Sessions" onclick=\{showAbout\}>\s*<img class="app-icon"/);
  const api = readFileSync(new URL('../src/lib/api.ts', import.meta.url), 'utf8');
  assert.match(api, /showAbout\?\(\): Promise<void>;/, 'the browser demo has no About window');
  assert.match(api, /return await this\.binding\.ShowAbout\(\);/);
});

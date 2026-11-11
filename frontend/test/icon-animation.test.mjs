// Drive the real Svelte action wiring, including class changes on an existing
// SVG. Helper-only tests would miss a broken action parameter/update binding.
import test, { after } from 'node:test';
import { spinTicker, blinkTicker } from '../src/lib/animationVisibility.ts';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import { compile } from 'svelte/compiler';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
const observers = [];
globalThis.IntersectionObserver = class {
  constructor(callback) { this.callback = callback; observers.push(this); }
  observe(target) { this.target = target; }
  disconnect() { this.disconnected = true; }
  emit(visible) { this.callback([{ target: this.target, isIntersecting: visible }]); }
};
const client = path => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
const url = new URL('../src/lib/components/common/Icon.svelte', import.meta.url);
let code = compile(readFileSync(url, 'utf8'), { generate: 'client', filename: fileURLToPath(url), css: 'external' }).js.code;
code = code
  .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
  .replace("'svelte/internal/client'", client('internal/client/index.js'))
  .replace("'../../icons'", JSON.stringify(new URL('../src/lib/icons.ts', import.meta.url).href))
  .replace("'../../animationVisibility'", JSON.stringify(new URL('../src/lib/animationVisibility.ts', import.meta.url).href));
const Icon = (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;

test('mounted Icon starts and stops its observer when its spin class changes', async () => {
  const target = document.createElement('div');
  document.body.appendChild(target);
  // Use a reactive harness to drive updates on the existing Icon instance.
  const harnessSource = `<script>import Icon from './Icon.svelte'; let spinning = $state(false); export function setSpin(value) { spinning = value; }</script><Icon name="loader-circle" label="Working" class={spinning ? 'spin other' : 'other'} />`;
  let harnessCode = compile(harnessSource, { generate: 'client', filename: 'IconHarness.svelte', css: 'external' }).js.code
    .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'));
  globalThis.__animationIcon = Icon;
  harnessCode = harnessCode.replace("import Icon from './Icon.svelte';", 'const Icon = globalThis.__animationIcon;');
  const Harness = (await import(`data:text/javascript,${encodeURIComponent(harnessCode)}`)).default;
  const component = mount(Harness, { target });
  try {
    flushSync();
    assert.equal(observers.length, 0, 'ordinary icons need no observer');
    const svg = target.querySelector('svg');
    assert.equal(svg.getAttribute('aria-label'), 'Working');
    flushSync(() => component.setSpin(true));
    assert.equal(observers.length, 1);
    observers[0].emit(true);
    assert.equal(svg.hasAttribute('data-animation-visible'), true);
    observers[0].emit(false);
    assert.equal(svg.hasAttribute('data-animation-visible'), false);
    observers[0].emit(true);
    flushSync(() => component.setSpin(false));
    assert.equal(observers[0].disconnected, true);
    assert.equal(svg.hasAttribute('data-animation-visible'), false);
    flushSync(() => component.setSpin(true));
    assert.equal(observers.length, 2);
  } finally {
    await unmount(component);
    target.remove();
  }
  assert.equal(observers[1].disconnected, true);
  assert.equal(spinTicker.listeners, 0);
});

after(() => dom.window.close());

test('mounted AttentionDot preserves status labels and stops motion on state changes and unmount', async t => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  const dotURL = new URL('../src/lib/components/common/AttentionDot.svelte', import.meta.url);
  let dotCode = compile(readFileSync(dotURL, 'utf8'), { generate: 'client', filename: fileURLToPath(dotURL), css: 'external' }).js.code
    .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'))
    .replace("'../../alerts'", JSON.stringify(new URL('../src/lib/alerts.ts', import.meta.url).href))
    .replace("'../../animationVisibility'", JSON.stringify(new URL('../src/lib/animationVisibility.ts', import.meta.url).href));
  globalThis.__animationDot = (await import(`data:text/javascript,${encodeURIComponent(dotCode)}`)).default;
  const harness = `<script>let state = $state('needs'); export function setState(value) { state = value; } const Dot = globalThis.__animationDot;</script><Dot {state} announce />`;
  const harnessCode = compile(harness, { generate: 'client', filename: 'DotHarness.svelte', css: 'external' }).js.code
    .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'));
  const Harness = (await import(`data:text/javascript,${encodeURIComponent(harnessCode)}`)).default;
  const target = document.createElement('div');
  document.body.appendChild(target);
  const component = mount(Harness, { target });
  const start = observers.length;
  try {
    flushSync();
    assert.equal(observers.length, start, 'non-working states do not observe');
    assert.equal(target.querySelector('span').getAttribute('aria-label'), 'Needs you');
    flushSync(() => component.setState('working'));
    const dot = target.querySelector('span');
    assert.equal(dot.getAttribute('aria-label'), 'Working');
    assert.equal(dot.title, 'Working');
    observers[start].emit(true);
    assert.equal(blinkTicker.listeners, 1);
    t.mock.timers.tick(800);
    assert.equal(dot.hasAttribute('data-dim'), true);
    flushSync(() => component.setState('done'));
    assert.equal(dot.getAttribute('aria-label'), 'Finished');
    assert.equal(dot.hasAttribute('data-dim'), false);
    assert.equal(blinkTicker.listeners, 0);
    assert.equal(observers[start].disconnected, true);
    observers[start].emit(true);
    assert.equal(blinkTicker.listeners, 0, 'stale working callback cannot restart a finished dot');
    flushSync(() => component.setState('working'));
    observers[start + 1].emit(true);
    flushSync(() => component.setState('idle'));
    assert.equal(target.querySelector('span'), null);
    assert.equal(blinkTicker.listeners, 0);
    flushSync(() => component.setState('working'));
    observers[start + 2].emit(true);
    assert.equal(blinkTicker.listeners, 1);
  } finally {
    await unmount(component);
    target.remove();
    delete globalThis.__animationDot;
  }
  assert.equal(blinkTicker.listeners, 0);
  assert.equal(observers[start + 2].disconnected, true);
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { JSDOM } from 'jsdom';
import { compile } from 'svelte/compiler';
import { animateWhenVisible, blinkWhenVisible, createTicker, spinTicker, blinkTicker } from '../src/lib/animationVisibility.ts';

const motionPreference = { matches: false };

const source = path => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8');

test('presence indicators remain visible without continuous liveness animations', () => {
  const header = source('lib/components/transcript/SessionHeader.svelte');
  const row = source('lib/components/sessionlist/SessionRow.svelte');
  assert.match(header, /\{#if meta.live\}[\s\S]*?live-badge[\s\S]*?Live/);
  assert.match(row, /\{:else if session.live\}[\s\S]*?live-dot/);
  for (const [filename, text, selector] of [['header.svelte', header, 'live-pulse'], ['row.svelte', row, 'live-dot']]) {
    const css = compile(text, { generate: 'client', filename }).css.code;
    const rules = [...css.matchAll(new RegExp(`\\.${selector}[^{}]*\\{([^}]*)\\}`, 'g'))];
    assert.ok(rules.length);
    for (const rule of rules) assert.doesNotMatch(rule[1], /animation:/);
    assert.doesNotMatch(css, /@keyframes[^{}]*(?:live-ring|pulse)\s*\{/);
  }
});

test('spinner observer starts static, follows intersection, handles updates and cleans up', () => {
  const dom = new JSDOM('<svg></svg>');
  const node = dom.window.document.querySelector('svg');
  const previous = globalThis.IntersectionObserver;
  const observers = [];
  class Observer {
    constructor(callback) { this.callback = callback; observers.push(this); }
    observe(target) { this.target = target; }
    disconnect() { this.disconnected = true; }
    emit(visible) { this.callback([{ target: this.target, isIntersecting: visible }]); }
  }
  globalThis.IntersectionObserver = Observer;
  try {
    const action = animateWhenVisible(node, false);
    assert.equal(observers.length, 0);
    action.update(true);
    const first = observers[0];
    assert.equal(first.target, node);
    assert.equal(node.hasAttribute('data-animation-visible'), false);
    first.emit(true);
    assert.equal(node.hasAttribute('data-animation-visible'), true);
    first.emit(false);
    assert.equal(node.hasAttribute('data-animation-visible'), false);
    first.emit(true);
    action.update(false);
    assert.equal(first.disconnected, true);
    first.emit(true); // queued callback from an old observer must be ignored
    assert.equal(node.hasAttribute('data-animation-visible'), false);
    action.update(true);
    observers[1].emit(true);
    assert.equal(node.hasAttribute('data-animation-visible'), true);
    action.destroy();
    assert.equal(observers[1].disconnected, true);
    observers[1].emit(true);
    assert.equal(node.hasAttribute('data-animation-visible'), false);
  } finally {
    globalThis.IntersectionObserver = previous;
    dom.window.close();
  }
});

test('without IntersectionObserver spinner stays static and its status remains available', () => {
  const previous = globalThis.IntersectionObserver;
  globalThis.IntersectionObserver = undefined;
  const dom = new JSDOM('<span><svg></svg>running</span>');
  try {
    const node = dom.window.document.querySelector('svg');
    const action = animateWhenVisible(node, true);
    assert.equal(node.hasAttribute('data-animation-visible'), false);
    assert.equal(node.parentElement.textContent, 'running');
    action.destroy();
  } finally {
    globalThis.IntersectionObserver = previous;
    dom.window.close();
  }
});

test('work indicators use discrete action steps, with no continuous CSS interpolation', () => {
  const icon = source('lib/components/common/Icon.svelte');
  assert.match(icon, /use:animateWhenVisible=\{className.split\(\/\\s\+\/\).includes\('spin'\)\}/);
  assert.match(icon, /transform-origin: center/);
  assert.match(icon, /transform-box: view-box/);
  const dot = source('lib/components/common/AttentionDot.svelte');
  assert.match(dot, /use:blinkWhenVisible=\{state === 'working'\}/);
  assert.match(dot, /\.working:global\(\[data-dim\]\)\s*\{\s*opacity: 0.45/);
  assert.doesNotMatch(compile(dot, { generate: 'client', filename: 'AttentionDot.svelte' }).css.code, /animation:|transition:|@keyframes/);
  const css = source('style.css');
  assert.doesNotMatch(css, /icon-spin|\.icon\.spin[^{}]*\{[^}]*animation:/);
  const reduced = css.slice(css.indexOf('@media (prefers-reduced-motion: reduce)')).split('/* Inherited text')[0];
  assert.match(reduced, /animation-iteration-count: 1 !important/);
});

test('shared clock schedules one interval and stops when its last subscriber leaves', t => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  const ticker = createTicker(125);
  const a = [], b = [];
  const stopA = ticker.subscribe(tick => a.push(tick));
  const stopB = ticker.subscribe(tick => b.push(tick));
  assert.equal(ticker.listeners, 2);
  t.mock.timers.tick(250);
  assert.deepEqual(a, [0, 1, 2]);
  assert.deepEqual(b, a);
  stopA();
  t.mock.timers.tick(125);
  assert.deepEqual(a, [0, 1, 2]);
  assert.deepEqual(b, [0, 1, 2, 3]);
  stopB();
  assert.equal(ticker.listeners, 0);
  t.mock.timers.tick(1000);
  const c = [];
  const stopC = ticker.subscribe(tick => c.push(tick));
  assert.deepEqual(c, [3], 'unused clocks do not advance');
  t.mock.timers.tick(125);
  assert.deepEqual(c, [3, 4]);
  stopC();
});

test('visible rotation and dot steps share clocks, honor reduced motion and fully clean up', t => {
  t.mock.timers.enable({ apis: ['setInterval'] });
  const previousObserver = globalThis.IntersectionObserver;
  const previousMatch = globalThis.matchMedia;
  const observers = [];
  globalThis.matchMedia = () => motionPreference;
  globalThis.IntersectionObserver = class {
    constructor(callback) { this.callback = callback; observers.push(this); }
    observe(target) { this.target = target; }
    disconnect() { this.disconnected = true; }
    emit(visible) { this.callback([{ target: this.target, isIntersecting: visible }]); }
  };
  const dom = new JSDOM('<svg></svg><svg></svg><span title="Working"></span>');
  const [a, b] = dom.window.document.querySelectorAll('svg');
  const dot = dom.window.document.querySelector('span');
  const actions = [];
  try {
    actions.push(animateWhenVisible(a, true), animateWhenVisible(b, true), blinkWhenVisible(dot, true));
    assert.equal(spinTicker.listeners, 0);
    assert.equal(blinkTicker.listeners, 0);
    for (const observer of observers) { observer.emit(true); observer.emit(true); }
    assert.equal(spinTicker.listeners, 2, 'duplicate observer callbacks must not subscribe twice');
    assert.equal(blinkTicker.listeners, 1);
    const angle = () => Number(a.style.transform.match(/rotate\((\d+)deg\)/)[1]);
    const initial = angle();
    t.mock.timers.tick(124);
    assert.equal(angle(), initial);
    t.mock.timers.tick(1);
    assert.equal(angle(), (initial + 45) % 360);
    assert.equal(b.style.transform, a.style.transform, 'visible icons share a clock');
    t.mock.timers.tick(675);
    assert.equal(dot.hasAttribute('data-dim'), true);
    assert.equal(dot.title, 'Working');
    motionPreference.matches = true; // live preference change, not just at mount
    const angles = [];
    for (let i = 0; i < 4; i++) { t.mock.timers.tick(125); angles.push(angle()); }
    assert.ok(angles[0] === angles[1] || angles[1] === angles[2]);
    assert.equal(angles[2], (angles[0] + 45) % 360);
    assert.equal(angles[3], (angles[1] + 45) % 360);
    t.mock.timers.tick(300);
    assert.equal(dot.hasAttribute('data-dim'), false, 'reduced-motion dot stays steady');
    observers[0].emit(false);
    assert.equal(a.style.transform, '');
    assert.equal(spinTicker.listeners, 1);
    observers[0].emit(true);
    assert.equal(spinTicker.listeners, 2);
    actions[2].update(false);
    observers[2].emit(true); // queued callback cannot restart an old state
    assert.equal(blinkTicker.listeners, 0);
    assert.equal(dot.hasAttribute('data-dim'), false);
  } finally {
    for (const action of actions) action.destroy();
    assert.equal(spinTicker.listeners, 0);
    assert.equal(blinkTicker.listeners, 0);
    motionPreference.matches = false;
    globalThis.IntersectionObserver = previousObserver;
    globalThis.matchMedia = previousMatch;
    dom.window.close();
  }
});

test('no global animation pause can strand dialogs, menus or loading states', () => {
  // Entrance animations (dialog-in, pop-in, scrim-in) start at opacity 0, and
  // stale panes show loading pulses while a host connects. A paused clock on
  // `*` would leave them invisible or frozen.
  const dialogKeyframes = source('style.css').match(/@keyframes dialog-in\s*\{[\s\S]*?\}\s*\}/);
  assert.ok(dialogKeyframes);
  assert.match(dialogKeyframes[0], /opacity: 0/);
  const files = ['style.css', 'App.svelte', 'lib/animationVisibility.ts', 'lib/components/common/Icon.svelte'];
  for (const file of files) {
    const text = source(file);
    assert.doesNotMatch(text, /animation-play-state/, file);
    assert.doesNotMatch(text, /data-animations-paused|trackDocumentVisibility/, file);
    assert.doesNotMatch(text, /\.stale \*/, file);
  }
});

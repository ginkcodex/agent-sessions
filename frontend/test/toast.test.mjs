import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fixtureStore, loadServerComponent, loadServerModule, renderWithFixture } from './helpers.mjs';

const { ToastStore } = await loadServerModule(new URL('../src/lib/stores/toast.svelte.ts', import.meta.url), []);
const message = (extra = {}) => ({ title: 'Choose a file', body: 'Select a full path.', ...extra });

function timedStore(t) {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const store = new ToastStore();
  t.after(() => store.dismiss());
  return store;
}

test('toast IDs scope dismissal and stale actions cannot act on replacements', (t) => {
  const store = timedStore(t);
  const calls = [];
  const old = store.show(message({
    onDismiss: () => calls.push('old dismissed'),
    actions: [{ label: '/old/file', onSelect: () => calls.push('old selected') }],
  }));
  const current = store.show(message({
    onDismiss: () => calls.push('new dismissed'),
    actions: [{ label: '/new/file', onSelect: () => { assert.equal(store.current, null); calls.push('new selected'); } }],
  }));
  assert.ok(current > old);
  assert.equal(store.current.id, current);
  assert.deepEqual(calls, ['old dismissed']);
  store.dismiss(old);
  store.invoke(old, 0);
  for (const index of [-1, 1, 0.5, NaN]) store.invoke(current, index);
  assert.equal(store.current.id, current);
  assert.deepEqual(calls, ['old dismissed']);
  store.invoke(current, 0);
  store.invoke(current, 0);
  store.dismiss(current);
  assert.deepEqual(calls, ['old dismissed', 'new dismissed', 'new selected']);
});

test('persistent toast never expires, including after hover and focus', (t) => {
  const store = timedStore(t);
  let dismissed = 0;
  const id = store.show(message({ persistent: true, onDismiss: () => dismissed++ }));
  t.mock.timers.tick(100_000);
  store.pause('pointer');
  store.pause('focus');
  store.resume('pointer');
  store.resume('focus');
  t.mock.timers.tick(100_000);
  assert.equal(store.current.id, id);
  assert.equal(dismissed, 0);
  store.dismiss(id);
  assert.equal(dismissed, 1);
});

test('ordinary toast expires after eight seconds and notifies once', (t) => {
  const store = timedStore(t);
  let dismissed = 0;
  store.show(message({ onDismiss: () => dismissed++ }));
  t.mock.timers.tick(7999);
  assert.ok(store.current);
  t.mock.timers.tick(1);
  assert.equal(store.current, null);
  assert.equal(dismissed, 1);
  store.dismiss();
  assert.equal(dismissed, 1);
});

test('hover and keyboard focus independently pause ordinary toast expiry', (t) => {
  const store = timedStore(t);
  store.show(message());
  t.mock.timers.tick(7000);
  store.pause('pointer');
  store.pause('focus');
  t.mock.timers.tick(20_000);
  store.resume('pointer');
  t.mock.timers.tick(20_000);
  assert.ok(store.current, 'still keyboard focused');
  store.resume('focus');
  t.mock.timers.tick(7999);
  assert.ok(store.current);
  t.mock.timers.tick(1);
  assert.equal(store.current, null);
});

test('replacement cancels prior timeout and dismissal callbacks may show a toast', (t) => {
  const store = timedStore(t);
  let dismissed = 0;
  store.show(message({ onDismiss: () => dismissed++ }));
  t.mock.timers.tick(7000);
  const replacement = store.show(message({ persistent: true }));
  t.mock.timers.tick(10_000);
  assert.equal(store.current.id, replacement);
  assert.equal(dismissed, 1);
  store.show(message({ onDismiss: () => store.show(message({ title: 'Callback toast', persistent: true })) }));
  store.dismiss();
  assert.equal(store.current.title, 'Callback toast');
});

test('toaster renders escaped full-path buttons and accessible dismiss control', async () => {
  const url = new URL('../src/lib/components/common/Toaster.svelte', import.meta.url);
  const component = await loadServerComponent(url, [
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
  ]);
  const label = '/project/very/long/full/path/<script>alert(1)</script>/file.ts';
  const html = await renderWithFixture(component, {
    toast: { current: { ...message({ actions: [{ label, onSelect() {} }] }), id: 7 } },
  });
  assert.match(html, /role="status"/);
  assert.match(html, /role="group" aria-label="Toast actions"/);
  assert.match(html, /aria-label="Dismiss"/);
  assert.ok(html.replaceAll('&lt;', '<').replaceAll('&gt;', '>').includes(label));
  assert.ok(html.includes('&lt;script>') || html.includes('&lt;script&gt;'));
  assert.ok(!html.includes('<script>'));
  const source = readFileSync(url, 'utf8');
  assert.match(source, /max-height: calc\(100dvh - 32px\)/);
  assert.match(source, /overflow-y: auto/);
  assert.match(source, /overflow-wrap: anywhere/);
  assert.match(source, /onfocusin=/);
  assert.match(source, /onfocusout=/);
});

test('toast buttons sit at the end with the first one filled; choices list in full', async () => {
  const url = new URL('../src/lib/components/common/Toaster.svelte', import.meta.url);
  const component = await loadServerComponent(url, [
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
  ]);
  const render = (extra) => renderWithFixture(component, { toast: { current: { ...message(extra), id: 1 } } });
  const actions = [{ label: 'Open', onSelect() {} }, { label: 'Later', onSelect() {} }];

  const buttons = await render({ actions });
  assert.match(buttons, /class="toast-action[^"]*primary[^"]*"[^>]*>\s*Open/);
  assert.doesNotMatch(buttons, /class="toast-action[^"]*primary[^"]*"[^>]*>\s*Later/);
  assert.doesNotMatch(buttons, /class="toast-actions[^"]*choices/);
  assert.doesNotMatch(buttons, /class="toast [^"]*wide/);

  const choices = await render({ actions, choices: true });
  assert.match(choices, /class="toast [^"]*wide/);
  assert.match(choices, /class="toast-actions[^"]*choices/);
  assert.doesNotMatch(choices, /primary/);

  // A tone icon leads: info, or an alert for errors.
  assert.match(await render({}), /class="toast-icon[^"]*"[^>]*aria-hidden="true"/);
  assert.match(await render({ tone: 'error' }), /class="toast [^"]*error/);

  const source = readFileSync(url, 'utf8');
  assert.match(source, /name=\{message\.tone === 'error' \? 'circle-alert' : 'info'\}/);
  assert.match(source, /\.toast-actions \{[^}]*justify-content: flex-end;/);
  assert.match(source, /\.toast-actions\.choices \.toast-action \{[^}]*text-align: left;/);
  assert.doesNotMatch(source, /border-left: 3px/, 'no accent stripe');

  const fileOpen = readFileSync(new URL('../src/lib/messageFileOpen.ts', import.meta.url), 'utf8');
  assert.match(fileOpen, /choices: true/, 'the file chooser lists its paths');
});

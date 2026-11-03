import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { isFullscreenShortcut, toggleFullscreen, preventDesktopEscapeFullscreen, keyHandled } from '../src/lib/fullscreen.ts';
import { terminalKeyAction } from '../src/lib/terminal.ts';
import { flushRuntime, installDesktopRuntime } from './desktop-app.mjs';

const key = { type: 'keydown', key: 'f', code: 'KeyF', metaKey: true, shiftKey: false, ctrlKey: true, altKey: false };

test('fullscreen shortcuts use Ctrl+Cmd+F on macOS and F11 elsewhere, including terminals', () => {
  assert.equal(isFullscreenShortcut(key, true), true);
  assert.equal(isFullscreenShortcut({ ...key, ctrlKey: false }, true), false);
  assert.equal(isFullscreenShortcut({ ...key, metaKey: false }, true), false);
  assert.equal(isFullscreenShortcut({ ...key, shiftKey: true }, true), false);
  assert.equal(isFullscreenShortcut({ ...key, altKey: true }, true), false);
  assert.equal(isFullscreenShortcut({ ...key, ctrlKey: false, shiftKey: true }, true), false);
  assert.equal(isFullscreenShortcut({ ...key, metaKey: false, shiftKey: true }, true), false);
  assert.equal(isFullscreenShortcut(key, false), false);
  const f11 = { ...key, key: 'F11', code: 'F11', metaKey: false, ctrlKey: false };
  assert.equal(isFullscreenShortcut(f11, false), true);
  for (const modifier of ['ctrlKey', 'metaKey', 'shiftKey', 'altKey']) {
    assert.equal(isFullscreenShortcut({ ...f11, [modifier]: true }, false), false);
  }
  assert.equal(terminalKeyAction(key, true), 'app');
  assert.equal(terminalKeyAction(f11, false), 'app');
});

const ROUTER = 'github.com/ginkcode/agent-sessions/internal/app.Router.Call';

// A desktop runtime whose ToggleFullscreen call is answered by toggle.
function desktopFullscreen(toggle) {
  const calls = [];
  const restore = installDesktopRuntime({ Call: { ByName: (name, ...args) => { calls.push([name, ...args]); return toggle(); } } });
  const oldDocument = globalThis.document;
  globalThis.document = {
    fullscreenElement: null,
    documentElement: { requestFullscreen: async () => { throw new Error('browser fullscreen used'); } },
  };
  return {
    calls,
    restore() {
      restore();
      if (oldDocument === undefined) delete globalThis.document;
      else globalThis.document = oldDocument;
    },
  };
}

test('desktop fullscreen calls the App method through the router', async () => {
  const desktop = desktopFullscreen(async () => {});
  try {
    await toggleFullscreen();
    await toggleFullscreen();
    assert.deepEqual(desktop.calls, [[ROUTER, 'ToggleFullscreen', []], [ROUTER, 'ToggleFullscreen', []]]);
  } finally { desktop.restore(); }
});

test('fullscreen awaits the app binding and suppresses overlapping requests', async () => {
  let complete;
  const desktop = desktopFullscreen(() => new Promise(resolve => { complete = resolve; }));
  try {
    let finished = false;
    const first = toggleFullscreen().then(() => { finished = true; });
    await toggleFullscreen();
    await flushRuntime();
    assert.equal(desktop.calls.length, 1);
    assert.equal(finished, false);
    complete();
    await first;
    const next = toggleFullscreen();
    await flushRuntime();
    assert.equal(desktop.calls.length, 2);
    complete();
    await next;
  } finally { desktop.restore(); }
});

test('binding failures release the guard without toggling through another path', async () => {
  const desktop = desktopFullscreen(async () => { throw new Error('native failure'); });
  try {
    await assert.rejects(toggleFullscreen(), /native failure/);
    await assert.rejects(toggleFullscreen(), /native failure/);
    assert.equal(desktop.calls.length, 2);
  } finally { desktop.restore(); }
});

test('session pane suppresses only its container outline while retaining keyboard focus', () => {
  const pane = readFileSync(new URL('../src/lib/components/sessionlist/SessionList.svelte', import.meta.url), 'utf8');
  assert.match(pane, /class="session-list-pane"[\s\S]*?tabindex="-1"/);
  assert.match(pane, /\.session-list-pane\s*\{[^}]*outline:\s*none;/);
  assert.doesNotMatch(pane, /\.session-list-pane\s+[^\s{}][^{}]*\{[^}]*outline:\s*none;/);
});

test('fullscreen falls back to browser APIs and recovers from failures', async () => {
  const oldWindow = globalThis.window;
  const oldDocument = globalThis.document;
  let full = false;
  globalThis.window = {};
  globalThis.document = {
    get fullscreenElement() { return full ? {} : null; },
    documentElement: { requestFullscreen: async () => { full = true; } },
    exitFullscreen: async () => { full = false; },
  };
  try {
    await toggleFullscreen();
    assert.equal(full, true);
    await toggleFullscreen();
    assert.equal(full, false);
    globalThis.document.documentElement.requestFullscreen = async () => { throw new Error('denied'); };
    await assert.rejects(toggleFullscreen(), /denied/);
    await assert.rejects(toggleFullscreen(), /denied/);
  } finally {
    if (oldWindow === undefined) delete globalThis.window;
    else globalThis.window = oldWindow;
    if (oldDocument === undefined) delete globalThis.document;
    else globalThis.document = oldDocument;
  }
});

test('desktop Escape safeguard uses capture so dialogs cannot bypass it by stopping propagation', () => {
  const app = readFileSync(new URL('../src/App.svelte', import.meta.url), 'utf8');
  assert.match(app, /window\.addEventListener\('keydown', preventDesktopEscapeFullscreen, true\)/);
  assert.match(app, /window\.removeEventListener\('keydown', preventDesktopEscapeFullscreen, true\)/);
});

test('native macOS fullscreen does not exit on Escape', () => {
  const gui = readFileSync(new URL('../../cmd/agent-sessions/shell.go', import.meta.url), 'utf8');
  assert.match(gui, /DisableEscapeExitsFullscreen:\s*true/);
});

test('desktop Escape cancels WebKit fullscreen exit without swallowing the key', () => {
  const previous = globalThis.window;
  let cancelled = 0;
  const escape = { key: 'Escape', preventDefault: () => cancelled++ };
  try {
    globalThis.window = {};
    preventDesktopEscapeFullscreen(escape);
    assert.equal(cancelled, 0, 'browser keeps its native Escape behavior');
    globalThis.window = { __AGENT_SESSIONS_DESKTOP__: true };
    preventDesktopEscapeFullscreen(escape);
    assert.equal(cancelled, 1);
    preventDesktopEscapeFullscreen({ ...escape, key: 'Enter' });
    assert.equal(cancelled, 1);
  } finally {
    if (previous === undefined) delete globalThis.window;
    else globalThis.window = previous;
  }
});

test('the desktop Escape safeguard does not make Escape look already handled', () => {
  const previous = globalThis.window;
  // A minimal event whose defaultPrevented reflects preventDefault calls.
  const event = (key = 'Escape') => {
    const e = { key, defaultPrevented: false };
    e.preventDefault = function () { this.defaultPrevented = true; };
    return e;
  };
  try {
    globalThis.window = { __AGENT_SESSIONS_DESKTOP__: true };
    const fresh = event();
    preventDesktopEscapeFullscreen(fresh);
    assert.equal(fresh.defaultPrevented, true, 'WebKit fullscreen exit is still cancelled');
    assert.equal(keyHandled(fresh), false, 'dialogs must still close on an unused Escape');

    const used = event();
    preventDesktopEscapeFullscreen(used);
    used.preventDefault(); // e.g. a find bar or menu consuming Escape
    assert.equal(keyHandled(used), true);

    globalThis.window = {};
    const browser = event();
    preventDesktopEscapeFullscreen(browser);
    assert.equal(keyHandled(browser), false);
    browser.preventDefault();
    assert.equal(keyHandled(browser), true);
  } finally {
    if (previous === undefined) delete globalThis.window;
    else globalThis.window = previous;
  }
});

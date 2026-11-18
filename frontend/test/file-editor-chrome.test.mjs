import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { extname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { spawn } from 'node:child_process';
import { fileURLToPath } from 'node:url';

// Opt in to real-browser editor checks without making Chrome a dependency of npm test:
// AS_TEST_CHROME=/path/to/chrome node --loader ./test/resolver.mjs --test test/file-editor-chrome.test.mjs
// This builds the real fileEditor.ts with Vite, mounts it in headless Chrome
// and types with real key events over the DevTools protocol. It is not the
// Wails web view (WKWebView, WebView2 or WebKitGTK) and not the app's menus.

const EDITOR = fileURLToPath(new URL('../src/lib/fileEditor.ts', import.meta.url));
const COLORS = '--bg-primary:#fff;--bg-secondary:#f6f8fa;--text-primary:#1f2328;--text-muted:#656d76;--text-secondary:#656d76;--accent-color:#0969da;--border-color:#d0d7de;--hover-bg:#eaeef2;--font-mono:monospace;--font-sans:sans-serif';

async function buildPage(dir, text, path) {
  const root = join(dir, 'page');
  mkdirSync(root);
  writeFileSync(join(root, 'index.html'), `<!doctype html><meta charset="utf-8"><style>:root{${COLORS}}#host{height:300px;width:600px}</style><div id="host"></div><script type="module" src="./entry.ts"></script>`);
  writeFileSync(join(root, 'entry.ts'), `
    import { createFileEditor } from ${JSON.stringify(EDITOR)};
    const state = { text: ${JSON.stringify(text)} };
    const opts = (o = {}) => ({ id: {}, text: state.text, path: ${JSON.stringify(path)}, version: 1, wrap: false, onchange: (t) => { state.text = t; }, onsave() {}, ...o });
    const editor = createFileEditor(document.getElementById('host'), opts());
    window.probe = { editor, state, opts };
  `);
  const { build } = await import('vite');
  await build({ root, logLevel: 'error', configFile: false, build: { outDir: join(dir, 'dist'), emptyOutDir: true } });
  return join(dir, 'dist');
}

// Serves files inside dist only: a path that resolves outside it is not found.
function serve(dist) {
  const root = resolve(dist);
  const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css' };
  const server = createServer((req, res) => {
    let file;
    try {
      const path = decodeURIComponent(new URL(req.url, 'http://localhost').pathname);
      file = resolve(root, '.' + (path === '/' ? '/index.html' : path));
    } catch { res.writeHead(400); res.end(); return; }
    const inside = relative(root, file);
    let body;
    try {
      if (!inside || inside.split(sep)[0] === '..' || isAbsolute(inside)) throw new Error('outside');
      body = readFileSync(file);
    } catch { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { 'content-type': types[extname(file)] ?? 'application/octet-stream' });
    res.end(body);
  });
  return new Promise((done, fail) => {
    server.once('error', fail);
    server.listen(0, '127.0.0.1', () => done(server));
  });
}

// Settles with promise, or fails with what after ms; the timer never outlives it.
function within(promise, ms, what) {
  let timer;
  const late = new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(`${what}: no answer after ${ms} ms`)), ms); });
  return Promise.race([promise, late]).finally(() => clearTimeout(timer));
}

// Starts headless Chrome on url and waits until the page set window.probe.
// Any failure on the way kills Chrome and says why; close() kills it and
// resolves once it exited, so its profile folder can be removed.
async function openChrome(dir, url) {
  const binary = process.env.AS_TEST_CHROME;
  const chrome = spawn(binary, ['--headless=new', '--no-first-run', '--remote-debugging-port=0', `--user-data-dir=${join(dir, 'profile')}`, 'about:blank'], { stdio: ['ignore', 'ignore', 'pipe'] });
  const exited = new Promise(done => { chrome.once('exit', done); chrome.once('error', done); });
  let ws;
  const close = async () => {
    try { ws?.close(); } catch { /* already closed */ }
    if (chrome.exitCode === null && chrome.signalCode === null) chrome.kill();
    await within(exited, 5000, 'Chrome to exit').catch(() => chrome.kill('SIGKILL'));
  };
  try {
    let log = '';
    const wsUrl = await within(new Promise((done, fail) => {
      chrome.once('error', e => fail(new Error(`Chrome did not start (${binary}): ${e.message}`)));
      chrome.once('exit', code => fail(new Error(`Chrome exited (${code})`)));
      chrome.stderr.on('data', d => { log += d; const m = log.match(/DevTools listening on (ws:\S+)/); if (m) done(m[1]); });
    }), 20000, 'Chrome\'s DevTools address').catch(e => { throw new Error(log ? `${e.message}; Chrome said: ${log}` : e.message); });

    ws = new WebSocket(wsUrl);
    await within(new Promise((done, fail) => {
      ws.onopen = done;
      ws.onerror = () => fail(new Error(`DevTools connection to ${wsUrl} failed`));
    }), 10000, 'DevTools connection');
    let id = 0;
    const pending = new Map();
    ws.onmessage = e => {
      const m = JSON.parse(e.data);
      const call = pending.get(m.id);
      if (!call) return;
      pending.delete(m.id);
      if (m.error) call.fail(new Error(`${call.method}: ${JSON.stringify(m.error)}`));
      else call.done(m.result);
    };
    ws.onclose = () => {
      for (const call of pending.values()) call.fail(new Error(`${call.method}: the DevTools connection closed`));
      pending.clear();
    };
    const send = (method, params = {}, sessionId) => {
      if (ws.readyState !== WebSocket.OPEN) return Promise.reject(new Error(`${method}: the DevTools connection is closed`));
      const n = ++id;
      const answer = new Promise((done, fail) => pending.set(n, { method, done, fail }));
      ws.send(JSON.stringify({ id: n, method, params, sessionId }));
      return within(answer, 15000, method).finally(() => pending.delete(n));
    };
    const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
    const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
    const page = (method, params) => send(method, params, sessionId);
    await page('Page.navigate', { url });
    const evaluate = async expression => {
      const r = await page('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
      if (r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails));
      return r.result.value;
    };
    await until(() => evaluate('!!window.probe'), Boolean, `the page at ${url} to mount the editor`, 10000);
    return { page, evaluate, close };
  } catch (e) {
    await close();
    throw e;
  }
}

const sleep = ms => new Promise(done => setTimeout(done, ms));

// Reads until ok(value) holds and returns that value. Fails after ms with
// the last value read, or the last error.
async function until(read, ok, what, ms = 3000) {
  const end = Date.now() + ms;
  let last;
  for (;;) {
    try {
      const value = await read();
      if (ok(value)) return value;
      last = JSON.stringify(value);
    } catch (e) { last = e.message; }
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}; last: ${last}`);
    await sleep(25);
  }
}

// Nothing is expected to happen: as long as a popup or an answer takes to show.
const settle = () => sleep(400);
// Waits until the popup shows items that pass ok and have not changed for longer
// than CodeMirror's interactionDelay (75 ms), which ignores accept keys after
// the options change, so the next Tab or Enter accepts.
async function acceptable(popup, ok, what, ms = 3000) {
  const end = Date.now() + ms;
  for (;;) {
    const items = await until(popup, ok, what, Math.max(end - Date.now(), 0));
    await sleep(150);
    if (JSON.stringify(await popup()) === JSON.stringify(items)) return items;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what} to hold still`);
  }
}

// Longer than the history's newGroupDelay (500 ms): the next edit is its own undo step.
const newUndoStep = () => sleep(600);

const MODIFIERS = { alt: 1, ctrl: 2, meta: 4, shift: 8 };
// Punctuation has its own key codes: '.' as its char code (46) would be Delete.
const KEY_CODES = { Tab: 9, Enter: 13, Escape: 27, ' ': 32, '.': 190 };
const KEY_NAMES = { ' ': 'Space', '.': 'Period' };

// Puts the caret at the end, and waits until the editor took it: its active line is the last one.
async function caretToEnd(evaluate) {
  for (let i = 0; i < 40; i++) {
    await evaluate(`(() => { const c = document.querySelector('.cm-content'); c.focus(); const s = getSelection(); s.selectAllChildren(c); s.collapseToEnd(); })()`);
    await sleep(50);
    if (await evaluate(`document.querySelector('.cm-content > .cm-line:last-child')?.classList.contains('cm-activeLine') ?? false`)) return;
  }
  throw new Error('the editor did not take the caret');
}

function keyboard(page) {
  async function press(key, mods = []) {
    const modifiers = mods.reduce((all, m) => all | MODIFIERS[m], 0);
    const code = KEY_NAMES[key] ?? (key.length === 1 ? `Key${key.toUpperCase()}` : key);
    const text = mods.length || key.length > 1 ? undefined : key;
    const windowsVirtualKeyCode = KEY_CODES[key] ?? key.toUpperCase().charCodeAt(0);
    await page('Input.dispatchKeyEvent', { type: text ? 'keyDown' : 'rawKeyDown', key, code, windowsVirtualKeyCode, modifiers, text, unmodifiedText: text });
    await page('Input.dispatchKeyEvent', { type: 'keyUp', key, code, windowsVirtualKeyCode, modifiers });
  }
  const type = async text => { for (const c of text) await press(c); };
  return { press, type };
}

// Builds the editor page for text at path, opens it in Chrome and gives fn
// the page and its helpers; closes and removes everything afterwards.
async function withEditor(text, path, fn) {
  const dir = mkdtempSync(join(tmpdir(), 'as-completion-'));
  let server, browser;
  try {
    const dist = await buildPage(dir, text, path);
    server = await serve(dist);
    browser = await openChrome(dir, `http://127.0.0.1:${server.address().port}/`);
    const { evaluate, page } = browser;
    const isMac = /Mac/.test(await evaluate('navigator.platform'));
    await fn({
      evaluate,
      ...keyboard(page),
      popup: () => evaluate(`[...document.querySelectorAll('.cm-tooltip-autocomplete li')].map(li => li.textContent)`),
      text: () => evaluate('window.probe.state.text'),
      isMac,
    });
  } finally {
    await browser?.close();
    if (server) {
      server.closeAllConnections();
      await new Promise(done => server.close(done));
    }
    rmSync(dir, { recursive: true, force: true });
  }
}

test('Chrome: suggestions open while typing, Tab and Enter accept, Escape closes, the shortcut works with the setting off', { skip: !process.env.AS_TEST_CHROME, timeout: 120000 }, async () => {
  await withEditor('const alphabet = 1;\nconst alpine = 2;\n', 'a.js', async ({ evaluate, press, type, popup, text, isMac }) => {
    const lastLine = async () => (await text()).split('\n').at(-1);
    const open = what => until(popup, items => items.length > 0, what);
    const closed = what => until(popup, items => items.length === 0, what);
    await caretToEnd(evaluate);

    await type('al');
    await acceptable(popup, items => items.includes('alphabet') && items.includes('alpine'), 'words from the file');
    const look = await evaluate(`(() => { const t = document.querySelector('.cm-tooltip-autocomplete'); return { selected: getComputedStyle(t.querySelector('li[aria-selected]')).backgroundColor, overflow: getComputedStyle(t).overflow }; })()`);
    assert.doesNotMatch(look.selected, /17, 119, 204/, 'not CodeMirror\'s own blue row');
    assert.equal(look.overflow, 'visible');

    await press('Tab');
    await until(lastLine, line => line === 'alphabet', 'Tab accepts');
    await closed('the popup to close after Tab');

    await press('Enter');
    await type('alp');
    await acceptable(popup, items => items.length > 0, 'suggestions for alp');
    await press('Enter');
    await until(lastLine, line => /^alp(habet|ine)$/.test(line), 'Enter accepts instead of breaking the line');

    // The language offers `alpine` (a declared const) even once it is typed in full.
    await press('Enter');
    await type('alpine');
    await acceptable(popup, items => items.some(i => i.startsWith('alpine')), 'alpine offered in full');
    await press('Enter');
    await until(text, t => /\nalpine\n$/.test(t), 'a finished name: Enter breaks the line');

    await type('al');
    await open('suggestions for al');
    const before = await text();
    await press('Escape');
    await closed('Escape closes the popup');
    assert.equal(await text(), before);

    await press('Enter');
    const length = (await until(text, t => t.endsWith('\n'), 'Enter breaks the line')).length;
    await press('Tab');
    await until(text, t => t.length > length && /[\t ]$/.test(t), 'without a popup Tab indents');

    await evaluate('window.probe.editor.update(window.probe.opts({ text: window.probe.state.text, suggest: false }))');
    await type('al');
    await settle();
    assert.deepEqual(await popup(), [], 'setting off: typing opens nothing');
    if (isMac) {
      await press('i', ['meta']);
      await open('Cmd+I opens suggestions');
      await press('Escape');
      await closed('Escape after Cmd+I');
    }
    await press(' ', ['ctrl']);
    await open('Ctrl+Space opens suggestions');
    await press('Escape');
    await closed('Escape after Ctrl+Space');

    await evaluate('window.probe.editor.update(window.probe.opts({ text: window.probe.state.text, disabled: true }))');
    await press(' ', ['ctrl']);
    await settle();
    assert.deepEqual(await popup(), [], 'read-only: no popup');
  });
});

test('Chrome: language-server suggestions after a dot, filtered as typed, with details; Enter applies the import and the snippet in one undo step', { skip: !process.env.AS_TEST_CHROME, timeout: 120000 }, async () => {
  await withEditor('package main\n\nfunc f() {\n\t', 'a.go', async ({ evaluate, press, type, popup, text, isMac }) => {
    await evaluate(`(() => {
      window.asks = [];
      const items = [
        { label: 'Println', kind: 3, sortText: 'a', snippet: true, detail: 'func(a ...any)',
          edit: { range: { start: { line: 3, character: 5 }, end: { line: 3, character: 5 } }, newText: 'Println(\${1:a ...any})\$0' },
          extraEdits: [{ range: { start: { line: 1, character: 0 }, end: { line: 1, character: 0 } }, newText: 'import "fmt"\\n' }] },
        { label: 'Printf', kind: 3, sortText: 'b', deprecated: true },
      ];
      const symbols = { ask() {}, hover: async () => undefined, complete(position, trigger) {
        window.asks.push({ position, trigger });
        if (trigger.kind !== 'character') return Promise.resolve({ items: [], incomplete: false, triggerCharacters: '.' });
        return new Promise(r => setTimeout(() => r({ items, incomplete: false, triggerCharacters: '.',
          details: async i => ({ detail: i ? 'func Printf' : 'func Println(a ...any) (n int, err error)', html: '<p>Writes a line.</p>' }) }), 50));
      } };
      window.probe.editor.update(window.probe.opts({ text: window.probe.state.text, symbols }));
    })()`);
    await caretToEnd(evaluate);

    await type('fmt.');
    const items = await until(popup, items => /^Println/.test(items[0] ?? '') && items.some(i => i.startsWith('Printf')), 'server items first');
    const asks = await evaluate('window.asks');
    assert.deepEqual(asks.at(-1), { position: { line: 3, character: 5 }, trigger: { kind: 'character', character: '.' } });
    assert.ok(items.some(i => i.startsWith('Printf')));
    assert.equal(await evaluate(`[...document.querySelectorAll('.cm-tooltip-autocomplete li')].find(li => li.textContent.startsWith('Printf')).classList.contains('cm-completion-deprecated')`), true);
    const struck = await evaluate(`getComputedStyle(document.querySelector('.cm-completion-deprecated .cm-completionLabel')).textDecorationLine`);
    assert.equal(struck, 'line-through');

    await type('Pr');
    await until(() => evaluate(`document.querySelector('.cm-completionInfo')?.textContent ?? ''`), info => /func Println\(a \.\.\.any\).*Writes a line\./s.test(info), 'the details panel');
    await acceptable(popup, items => /^Println/.test(items[0] ?? ''), 'Println first');
    assert.equal((await evaluate('window.asks')).length, asks.length, 'typing on filters the answer, without asking again');

    await press('Enter');
    await until(text, t => t === 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(a ...any)', 'the item and its import');
    assert.equal(await evaluate(`document.querySelector('.cm-snippetField')?.textContent ?? ''`), 'a ...any', 'the placeholder is a field');
    const undoKey = () => press('z', [isMac ? 'meta' : 'ctrl']);
    await newUndoStep();
    await type('X');
    await until(text, t => t === 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(X)', 'typing replaces the selected placeholder');
    await newUndoStep();
    await press('Tab');
    await type(';');
    await until(text, t => t === 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(X);', 'Tab goes on to $0, after the call');
    await until(() => evaluate(`document.querySelectorAll('.cm-snippetField, .cm-snippetFieldPosition').length`), n => n === 0, 'the snippet is done');
    await undoKey();
    await until(text, t => t === 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(X)', 'the first undo takes the ;');
    await undoKey();
    await until(text, t => t === 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(a ...any)', 'the typing undoes first');
    await undoKey();
    await until(text, t => t === 'package main\n\nfunc f() {\n\tfmt.Pr', 'one undo takes the suggestion and its import back');
  });
});

test('Chrome: Enter on an item whose import comes on resolve adds it with the item in one undo step, even typed in full; typing during the wait keeps the typing', { skip: !process.env.AS_TEST_CHROME, timeout: 120000 }, async () => {
  await withEditor('export {};\n', 'a.ts', async ({ evaluate, press, type, popup, text, isMac }) => {
    // Each resolve waits at a gate the test opens, after its own keystrokes.
    await evaluate(`(() => {
      window.resolves = [];
      window.gates = [];
      const imp = { range: { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } }, newText: 'import { helperValue } from "./helper.js";\\n' };
      const symbols = { ask() {}, hover: async () => undefined, complete() {
        return Promise.resolve({ items: [{ label: 'helperValue', kind: 6, labelDescription: './helper.js' }], incomplete: false,
          resolveEdits: (index, current, signal) => { window.resolves.push(index); return new Promise(r => window.gates.push(() => r(signal.aborted ? undefined : [imp]))); } });
      } };
      window.probe.editor.update(window.probe.opts({ text: window.probe.state.text, symbols }));
    })()`);
    const offered = what => acceptable(popup, items => /^helperValue/.test(items[0] ?? ''), what);
    // Opens the gate of the one resolve asked since the last.
    const answer = async () => {
      await until(() => evaluate('window.gates.length'), n => n === 1, 'a resolve on accept');
      await evaluate('window.gates.shift()()');
    };
    await caretToEnd(evaluate);

    await type('helperVa');
    await offered('helperValue offered');
    await press('Enter');
    assert.equal(await evaluate(`document.querySelector('.cm-tooltip-autocomplete') === null`), true, 'the popup closes at once');
    assert.equal(await text(), 'export {};\nhelperVa', 'the item waits for its import');
    await answer();
    await until(text, t => t === 'import { helperValue } from "./helper.js";\nexport {};\nhelperValue', 'the item with its import');
    assert.deepEqual(await evaluate('window.resolves'), [0]);
    await press('z', [isMac ? 'meta' : 'ctrl']);
    await until(text, t => t === 'export {};\nhelperVa', 'one undo takes the item and its import back');

    // Typed on before the import came: the typing stays, the item goes in alone,
    // and the import answered afterwards adds nothing.
    await newUndoStep();
    await type('l');
    await offered('helperValue offered again');
    await press('Enter');
    await press(' ');
    await until(text, t => t === 'export {};\nhelperValue ', 'the item alone, then the typing');
    await answer();
    await settle();
    assert.equal(await text(), 'export {};\nhelperValue ', 'a late import adds nothing');
    assert.deepEqual(await evaluate('window.resolves'), [0, 0]);

    // Typed in full, an item that may gain an import is accepted, not a new line.
    await type('helperValue');
    await offered('helperValue offered in full');
    await press('Enter');
    await answer();
    await until(text, t => t === 'import { helperValue } from "./helper.js";\nexport {};\nhelperValue helperValue', 'the import for the name typed in full');

    // From a server that does not resolve (no resolveEdits), the same item goes
    // in at once, and typed in full, Enter breaks the line.
    await evaluate(`(() => {
      const symbols = { ask() {}, hover: async () => undefined, complete() {
        return Promise.resolve({ items: [{ label: 'helperValue', kind: 6, labelDescription: './helper.js' }], incomplete: false });
      } };
      window.probe.editor.update(window.probe.opts({ text: window.probe.state.text, symbols }));
    })()`);
    await caretToEnd(evaluate);
    await type(' helperVa');
    await offered('helperValue offered without resolve');
    await press('Enter');
    await until(text, t => t.endsWith('helperValue helperValue helperValue'), 'the item at once');
    await type(' helperValue');
    await offered('helperValue offered in full without resolve');
    await press('Enter');
    await until(text, t => t.endsWith('helperValue helperValue helperValue helperValue\n'), 'a new line after the name typed in full');
    assert.deepEqual(await evaluate('window.resolves'), [0, 0, 0], 'no resolve');
  });
});

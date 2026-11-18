import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { extname, join } from 'node:path';
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

function serve(dist) {
  const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css' };
  const server = createServer((req, res) => {
    const file = join(dist, req.url === '/' ? 'index.html' : decodeURIComponent(req.url.split('?')[0]));
    let body;
    try { body = readFileSync(file); } catch { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { 'content-type': types[extname(file)] ?? 'application/octet-stream' });
    res.end(body);
  }).listen(0, '127.0.0.1');
  return new Promise(resolve => server.once('listening', () => resolve(server)));
}

async function openChrome(dir, url) {
  const chrome = spawn(process.env.AS_TEST_CHROME, ['--headless=new', '--no-first-run', '--remote-debugging-port=0', `--user-data-dir=${join(dir, 'profile')}`, 'about:blank']);
  const wsUrl = await new Promise((resolve, reject) => {
    let log = '';
    chrome.stderr.on('data', d => { log += d; const m = log.match(/DevTools listening on (ws:\S+)/); if (m) resolve(m[1]); });
    chrome.once('exit', code => reject(new Error(`Chrome exited (${code}): ${log}`)));
    setTimeout(() => reject(new Error('Chrome gave no DevTools address: ' + log)), 20000);
  });
  const ws = new WebSocket(wsUrl);
  await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
  let id = 0;
  const pending = new Map();
  ws.onmessage = e => { const m = JSON.parse(e.data); pending.get(m.id)?.(m); pending.delete(m.id); };
  const send = (method, params = {}, sessionId) => new Promise((resolve, reject) => {
    const n = ++id;
    pending.set(n, m => m.error ? reject(new Error(`${method}: ${JSON.stringify(m.error)}`)) : resolve(m.result));
    ws.send(JSON.stringify({ id: n, method, params, sessionId }));
  });
  const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
  const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
  const page = (method, params) => send(method, params, sessionId);
  await page('Page.navigate', { url });
  const evaluate = async expression => {
    const r = await page('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails));
    return r.result.value;
  };
  for (let i = 0; i < 200 && !(await evaluate('!!window.probe').catch(() => false)); i++) await sleep(50);
  const close = () => { ws.close(); chrome.kill(); };
  return { page, evaluate, close };
}

const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const MODIFIERS = { alt: 1, ctrl: 2, meta: 4, shift: 8 };
// Punctuation has its own key codes: '.' as its char code (46) would be Delete.
const KEY_CODES = { Tab: 9, Enter: 13, Escape: 27, ' ': 32, '.': 190 };
const KEY_NAMES = { ' ': 'Space', '.': 'Period' };

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

test('Chrome: suggestions open while typing, Tab and Enter accept, Escape closes, the shortcut works with the setting off', { skip: !process.env.AS_TEST_CHROME, timeout: 120000 }, async () => {
  const dir = mkdtempSync(join(tmpdir(), 'as-completion-'));
  let server, browser;
  try {
    const dist = await buildPage(dir, 'const alphabet = 1;\nconst alpine = 2;\n', 'a.js');
    server = await serve(dist);
    browser = await openChrome(dir, `http://127.0.0.1:${server.address().port}/`);
    const { evaluate, page } = browser;
    const { press, type } = keyboard(page);
    const popup = () => evaluate(`[...document.querySelectorAll('.cm-tooltip-autocomplete li')].map(li => li.textContent)`);
    const text = () => evaluate('window.probe.state.text');
    const lastLine = async () => (await text()).split('\n').at(-1);
    const toEnd = () => evaluate(`(() => { const c = document.querySelector('.cm-content'); c.focus(); const s = getSelection(); s.selectAllChildren(c); s.collapseToEnd(); })()`);
    const isMac = /Mac/.test(await evaluate('navigator.platform'));
    await toEnd();
    await sleep(100);

    await type('al');
    await sleep(400);
    const items = await popup();
    assert.ok(items.includes('alphabet') && items.includes('alpine'), `words from the file: ${items}`);
    const look = await evaluate(`(() => { const t = document.querySelector('.cm-tooltip-autocomplete'); return { selected: getComputedStyle(t.querySelector('li[aria-selected]')).backgroundColor, overflow: getComputedStyle(t).overflow }; })()`);
    assert.doesNotMatch(look.selected, /17, 119, 204/, 'not CodeMirror\'s own blue row');
    assert.equal(look.overflow, 'visible');

    await press('Tab');
    await sleep(150);
    assert.equal(await lastLine(), 'alphabet', 'Tab accepts');
    assert.deepEqual(await popup(), []);

    await press('Enter');
    await type('alp');
    await sleep(400);
    assert.ok((await popup()).length > 0);
    await press('Enter');
    await sleep(150);
    assert.match(await lastLine(), /^alp(habet|ine)$/, 'Enter accepts instead of breaking the line');

    // The language offers `alpine` (a declared const) even once it is typed in full.
    await press('Enter');
    await type('alpine');
    await sleep(400);
    await press('Enter');
    await sleep(150);
    assert.match(await text(), /\nalpine\n$/, 'a finished name: Enter breaks the line');

    await type('al');
    await sleep(400);
    assert.ok((await popup()).length > 0);
    const before = await text();
    await press('Escape');
    await sleep(150);
    assert.deepEqual(await popup(), [], 'Escape closes the popup');
    assert.equal(await text(), before);

    await press('Enter');
    await sleep(100);
    const length = (await text()).length;
    await press('Tab');
    await sleep(150);
    assert.ok((await text()).length > length && /[\t ]$/.test(await text()), 'without a popup Tab indents');

    await evaluate('window.probe.editor.update(window.probe.opts({ text: window.probe.state.text, suggest: false }))');
    await type('al');
    await sleep(400);
    assert.deepEqual(await popup(), [], 'setting off: typing opens nothing');
    if (isMac) {
      await press('i', ['meta']);
      await sleep(400);
      assert.ok((await popup()).length > 0, 'Cmd+I opens suggestions');
      await press('Escape');
      await sleep(100);
    }
    await press(' ', ['ctrl']);
    await sleep(400);
    assert.ok((await popup()).length > 0, 'Ctrl+Space opens suggestions');
    await press('Escape');
    await sleep(100);

    await evaluate('window.probe.editor.update(window.probe.opts({ text: window.probe.state.text, disabled: true }))');
    await press(' ', ['ctrl']);
    await sleep(300);
    assert.deepEqual(await popup(), [], 'read-only: no popup');
  } finally {
    browser?.close();
    server?.close();
    rmSync(dir, { recursive: true, force: true });
  }
});

test('Chrome: language-server suggestions after a dot, filtered as typed, with details; Enter applies the import and the snippet in one undo step', { skip: !process.env.AS_TEST_CHROME, timeout: 120000 }, async () => {
  const dir = mkdtempSync(join(tmpdir(), 'as-completion-'));
  let server, browser;
  try {
    const dist = await buildPage(dir, 'package main\n\nfunc f() {\n\t', 'a.go');
    server = await serve(dist);
    browser = await openChrome(dir, `http://127.0.0.1:${server.address().port}/`);
    const { evaluate, page } = browser;
    const { press, type } = keyboard(page);
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
    const popup = () => evaluate(`[...document.querySelectorAll('.cm-tooltip-autocomplete li')].map(li => li.textContent)`);
    const text = () => evaluate('window.probe.state.text');
    await evaluate(`(() => { const c = document.querySelector('.cm-content'); c.focus(); const s = getSelection(); s.selectAllChildren(c); s.collapseToEnd(); })()`);
    const isMac = /Mac/.test(await evaluate('navigator.platform'));
    await sleep(100);

    await type('fmt.');
    await sleep(500);
    const asks = await evaluate('window.asks');
    assert.deepEqual(asks.at(-1), { position: { line: 3, character: 5 }, trigger: { kind: 'character', character: '.' } });
    const items = await popup();
    assert.match(items[0] ?? '', /^Println/, `server items first: ${items}`);
    assert.ok(items.some(i => i.startsWith('Printf')));
    assert.equal(await evaluate(`[...document.querySelectorAll('.cm-tooltip-autocomplete li')].find(li => li.textContent.startsWith('Printf')).classList.contains('cm-completion-deprecated')`), true);
    const struck = await evaluate(`getComputedStyle(document.querySelector('.cm-completion-deprecated .cm-completionLabel')).textDecorationLine`);
    assert.equal(struck, 'line-through');

    await type('Pr');
    await sleep(400);
    assert.equal((await evaluate('window.asks')).length, asks.length, 'typing on filters the answer, without asking again');
    assert.match((await popup())[0] ?? '', /^Println/);
    const info = await evaluate(`document.querySelector('.cm-completionInfo')?.textContent ?? ''`);
    assert.match(info, /func Println\(a \.\.\.any\).*Writes a line\./s, `details panel: ${info}`);

    await press('Enter');
    await sleep(150);
    assert.equal(await text(), 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(a ...any)');
    assert.equal(await evaluate(`document.querySelector('.cm-snippetField')?.textContent ?? ''`), 'a ...any', 'the placeholder is a field');
    const undoKey = () => press('z', [isMac ? 'meta' : 'ctrl']);
    await type('X');
    await sleep(600);
    assert.equal(await text(), 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(X)', 'typing replaces the selected placeholder');
    await press('Tab');
    await sleep(100);
    await type(';');
    await sleep(600);
    assert.equal(await text(), 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(X);', 'Tab goes on to $0, after the call');
    assert.equal(await evaluate(`document.querySelectorAll('.cm-snippetField, .cm-snippetFieldPosition').length`), 0, 'the snippet is done');
    await undoKey();
    await sleep(100);
    await undoKey();
    await sleep(100);
    assert.equal(await text(), 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(a ...any)', 'the typing undoes first');
    await undoKey();
    await sleep(100);
    assert.equal(await text(), 'package main\n\nfunc f() {\n\tfmt.Pr', 'one undo takes the suggestion and its import back');
  } finally {
    browser?.close();
    server?.close();
    rmSync(dir, { recursive: true, force: true });
  }
});

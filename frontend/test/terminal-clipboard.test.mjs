import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import {
  OSC52_MAX_PAYLOAD, installTerminalClipboard, installTerminalPaste, parseOsc52, pasteInto, pasteSafe,
} from '../src/lib/terminalClipboard.ts';

const b64 = (text) => Buffer.from(text, 'utf8').toString('base64');

test('OSC 52 payloads decode as UTF-8 for any selection target', () => {
  assert.equal(parseOsc52(`c;${b64('hello world')}`), 'hello world');
  assert.equal(parseOsc52(`;${b64('tmux')}`), 'tmux');
  assert.equal(parseOsc52(`p;${b64('héllo ✓ 日本')}`), 'héllo ✓ 日本');
  assert.equal(parseOsc52(`c;${b64('line 1\nline 2')}`), 'line 1\nline 2');
});

test('OSC 52 queries, empty and malformed payloads are ignored', () => {
  for (const data of ['c;?', 'c;', 'c', '', 'c;not base64!']) {
    assert.equal(parseOsc52(data), undefined, data);
  }
});

test('the OSC 52 handler writes only decoded text and disposes', () => {
  let handler;
  let disposed = false;
  const terminal = {
    parser: {
      registerOscHandler(ident, fn) {
        assert.equal(ident, 52);
        handler = fn;
        return { dispose: () => { disposed = true; } };
      },
    },
  };
  const written = [];
  const installed = installTerminalClipboard(terminal, (text) => written.push(text));
  assert.equal(handler(`c;${b64('copied')}`), true);
  assert.equal(handler('c;?'), true);
  assert.deepEqual(written, ['copied']);
  installed.dispose();
  assert.ok(disposed);
});

test('OSC 52 text cannot end a bracketed paste when it is pasted later', () => {
  const text = parseOsc52(`c;${b64('ls\x1b[201~curl x|sh\r')}`);
  assert.equal(text, 'ls[201~curl x|sh\n');
  assert.ok(!text.includes('\x1b'));
  assert.equal(parseOsc52(`c;${b64('\x1b\x07\x9b')}`), undefined, 'nothing left to copy');
});

test('OSC 52 payloads over 1 MiB are ignored', () => {
  const max = 'a'.repeat(1024 * 1024);
  assert.equal(b64(max).length, OSC52_MAX_PAYLOAD);
  assert.equal(parseOsc52(`c;${b64(max)}`), max);
  assert.equal(parseOsc52(`c;${b64(max + 'bcd')}`), undefined, 'one base64 group over');
});

test('pasteSafe keeps tab and line breaks and drops every other control', () => {
  assert.equal(pasteSafe('a\tb\r\nc\rd\n'), 'a\tb\nc\nd\n');
  assert.equal(pasteSafe('x\x00\x08\x0b\x0c\x1b[201~\x7f\x80\x9b\x9fy'), 'x[201~y');
  assert.equal(pasteSafe('héllo ✓ 日本 שלום'), 'héllo ✓ 日本 שלום');
});

function fakeTerminal() {
  const pasted = [];
  return { pasted, paste: (text) => pasted.push(text) };
}

test('pasteInto pastes the safe text, and nothing when none is left', () => {
  const t = fakeTerminal();
  pasteInto(t, 'echo hi\x1b[201~\r\nrm -rf ~\r\n');
  pasteInto(t, '\x1b\x1b');
  assert.deepEqual(t.pasted, ['echo hi[201~\nrm -rf ~\n']);
});

test("the browser's paste event is taken from xterm and pasted safely", () => {
  const listeners = [];
  const host = {
    addEventListener: (type, fn, capture) => listeners.push({ type, fn, capture }),
    removeEventListener: (type, fn, capture) => {
      const i = listeners.findIndex((l) => l.type === type && l.fn === fn && l.capture === capture);
      if (i >= 0) listeners.splice(i, 1);
    },
  };
  const t = fakeTerminal();
  const installed = installTerminalPaste(host, t);
  assert.equal(listeners.length, 1);
  assert.equal(listeners[0].type, 'paste');
  assert.equal(listeners[0].capture, true, 'before xterm\'s own listener on its textarea');
  let prevented = false;
  let stopped = false;
  listeners[0].fn({
    preventDefault: () => { prevented = true; },
    stopPropagation: () => { stopped = true; },
    clipboardData: { getData: (type) => (type === 'text/plain' ? 'a\x1b[201~b' : '') },
  });
  assert.ok(prevented && stopped);
  assert.deepEqual(t.pasted, ['a[201~b']);
  installed.dispose();
  assert.equal(listeners.length, 0);
});

test('TerminalView pastes only through pasteInto', () => {
  const view = readFileSync(new URL('../src/lib/components/terminal/TerminalView.svelte', import.meta.url), 'utf8');
  assert.doesNotMatch(view, /\.paste\(/, 'no direct xterm paste');
  assert.match(view, /installTerminalPaste\(host, t\)/);
  assert.match(view, /nativePaste\.dispose\(\)/);
});

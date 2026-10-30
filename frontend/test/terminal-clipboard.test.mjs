import test from 'node:test';
import assert from 'node:assert/strict';
import { installTerminalClipboard, parseOsc52 } from '../src/lib/terminalClipboard.ts';

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

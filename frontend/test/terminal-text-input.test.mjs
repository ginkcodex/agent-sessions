import test from 'node:test';
import assert from 'node:assert/strict';
import { installTerminalCompositionText, terminalReplacementText } from '../src/lib/terminalTextInput.ts';

const key = (text, overrides = {}) => ({
  key: text, keyCode: 65, ctrlKey: false, metaKey: false,
  altKey: false, isComposing: false, ...overrides,
});

test('WebKit replacement strings are forwarded whole, including decomposed Vietnamese', () => {
  for (const text of ['ộng', 'Cộng', 'hòa', 'à', 'Cộng hòa']) {
    assert.equal(terminalReplacementText(key(text)), text);
  }
  assert.equal(terminalReplacementText(key('ng', { keyCode: 78 })), 'ng');
  assert.equal(terminalReplacementText(key('hò', { keyCode: 72 })), 'hò');
});

test('ordinary typing, composition, shortcuts and named keys stay with xterm', () => {
  for (const text of ['a', 'C', 'ộ', ' ', '😀']) assert.equal(terminalReplacementText(key(text)), null);
  for (const text of ['Dead', 'Unidentified', 'Process']) assert.equal(terminalReplacementText(key(text)), null);
  for (const modifier of ['ctrlKey', 'metaKey', 'altKey', 'isComposing']) {
    assert.equal(terminalReplacementText(key('ộng', { [modifier]: true })), null);
  }
  for (const [text, code] of [['Enter', 13], ['Backspace', 8], ['Delete', 46], ['ArrowLeft', 37], ['F1', 112], ['Process', 229], ['Unidentified', 0]]) {
    assert.equal(terminalReplacementText(key(text, { keyCode: code })), null);
  }
  assert.equal(terminalReplacementText(key('Cộng', { shiftKey: true })), 'Cộng');
});

function fixture() {
  const textarea = new EventTarget();
  textarea.value = '';
  const sent = [];
  const bridge = installTerminalCompositionText(textarea, text => sent.push(text));
  return { textarea, sent, bridge, input(data, value = data, inputType = 'insertFromComposition') {
    textarea.value = value;
    textarea.dispatchEvent(Object.assign(new Event('input'), { data, inputType }));
  }, end() { textarea.dispatchEvent(new Event('compositionend')); } };
}

test('preedit remains native and only committed text reaches the terminal', () => {
  const f = fixture();
  f.textarea.dispatchEvent(new Event('compositionstart'));
  for (const text of ['co', 'công', 'cộng']) f.input(text, text, 'insertCompositionText');
  assert.deepEqual(f.sent, []);
  assert.equal(f.textarea.value, 'cộng');
  f.input('cộng ', 'cộng\u00a0');
  assert.deepEqual(f.sent, ['cộng ']);
  assert.equal(f.textarea.value, 'cộng\u00a0', 'do not alter native preedit offsets');
  f.end();
  assert.equal(f.textarea.value, '');
});

test('nam comma space and repeated spaces commit once without stale text', () => {
  const f = fixture();
  for (const text of ['nam,', ' ', ' ', 'hòa.']) {
    f.input(text, text.replaceAll(' ', '\u00a0'));
    f.end();
    assert.equal(f.textarea.value, '');
  }
  assert.equal(f.sent.join(''), 'nam,  hòa.');
});

test('IME key 229 bypasses textarea diff while ordinary keys stay with xterm', () => {
  const f = fixture();
  assert.equal(f.bridge.key({ keyCode: 229 }), false);
  for (const keyCode of [13, 8, 65, 37]) assert.equal(f.bridge.key({ keyCode }), true);
});

test('other input events, intentional NBSP, cancellation and disposal', () => {
  const f = fixture();
  f.input('a', 'a', 'insertText');
  f.end();
  assert.equal(f.textarea.value, 'a');
  assert.deepEqual(f.sent, []);
  f.input('a\u00a0');
  f.end();
  assert.deepEqual(f.sent, ['a\u00a0']);
  f.input('');
  f.end();
  assert.deepEqual(f.sent, ['a\u00a0']);
  f.bridge.dispose();
  f.input('nam,');
  f.end();
  assert.deepEqual(f.sent, ['a\u00a0']);
  assert.equal(f.textarea.value, 'nam,');
  installTerminalCompositionText(undefined, () => {}).dispose();
});

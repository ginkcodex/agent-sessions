import test from 'node:test';
import assert from 'node:assert/strict';
import { terminalReplacementText } from '../src/lib/terminalTextInput.ts';

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

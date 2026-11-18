import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { JSDOM } from 'jsdom';
import {
  TEXT_INPUT_ATTRIBUTES,
  installTextInputDefaults,
  isTextEntry,
} from '../src/lib/textInputDefaults.ts';

function page() {
  return new JSDOM(`<!doctype html><body>
    <textarea id="area"></textarea>
    <input id="text">
    <input id="search" type="search">
    <input id="checked" type="checkbox">
    <input id="number" type="range">
    <button id="button">Go</button>
    <div id="editor" contenteditable="true"></div>
    <textarea id="asked" spellcheck="true" autocorrect="on"></textarea>
  </body>`).window;
}

const attributes = (el) => Object.fromEntries(Object.keys(TEXT_INPUT_ATTRIBUTES).map((name) => [name, el.getAttribute(name)]));
const off = { ...TEXT_INPUT_ATTRIBUTES };
const none = { spellcheck: null, autocorrect: null, autocapitalize: null, writingsuggestions: null };

test('the page inherits spellcheck and writing suggestions off from <html>', () => {
  const win = page();
  const stop = installTextInputDefaults(win.document);
  const html = win.document.documentElement;
  assert.equal(html.getAttribute('spellcheck'), 'false');
  assert.equal(html.getAttribute('writingsuggestions'), 'false');
  stop();
});

test('each text field gets every attribute when it is focused', () => {
  const win = page();
  const doc = win.document;
  const stop = installTextInputDefaults(doc);
  const editor = doc.getElementById('editor');
  // jsdom has no isContentEditable; WebKit does.
  Object.defineProperty(editor, 'isContentEditable', { value: true });
  for (const id of ['area', 'text', 'search', 'editor', 'asked']) {
    const el = doc.getElementById(id);
    el.dispatchEvent(new win.FocusEvent('focusin', { bubbles: true }));
    assert.deepEqual(attributes(el), off, id);
  }
  for (const id of ['checked', 'number', 'button']) {
    const el = doc.getElementById(id);
    el.dispatchEvent(new win.FocusEvent('focusin', { bubbles: true }));
    assert.deepEqual(attributes(el), none, id);
  }
  stop();
  const later = doc.createElement('textarea');
  doc.body.append(later);
  later.dispatchEvent(new win.FocusEvent('focusin', { bubbles: true }));
  assert.deepEqual(attributes(later), none, 'nothing changes once removed');
});

test('a field that already has focus is covered at install time', () => {
  const win = page();
  const area = win.document.getElementById('area');
  area.focus();
  const stop = installTextInputDefaults(win.document);
  assert.deepEqual(attributes(area), off);
  stop();
});

test('isTextEntry knows text fields from other controls', () => {
  const win = page();
  const doc = win.document;
  for (const id of ['area', 'text', 'search', 'asked']) assert.equal(isTextEntry(doc.getElementById(id)), true, id);
  for (const id of ['checked', 'number', 'button', 'editor']) assert.equal(isTextEntry(doc.getElementById(id)), false, id);
  assert.equal(isTextEntry(null), false);
  assert.equal(isTextEntry(win), false);
});

test('the app installs the defaults at startup on every platform', () => {
  const main = readFileSync(new URL('../src/main.ts', import.meta.url), 'utf8');
  assert.match(main, /^installTextInputDefaults\(\)/m);
});

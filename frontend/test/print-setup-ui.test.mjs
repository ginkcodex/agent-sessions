import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { compile } from 'svelte/compiler';
import { JSDOM } from 'jsdom';
import * as printing from '../src/lib/print.ts';
import * as setup from '../src/lib/printPageSetup.ts';

const dom = new JSDOM('<!doctype html><head><title>App title</title></head><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });
globalThis.Event = dom.window.Event;
dom.window.Element.prototype.scrollIntoView ??= function () {};
const client = path => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);

async function component(source, filename, replacements = []) {
  let code = compile(source, { generate: 'client', filename, css: 'external' }).js.code
    .replace("'svelte/internal/disclose-version'", client('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", client('internal/client/index.js'))
    .replace("'svelte/internal/flags/legacy'", client('internal/flags/legacy.js'))
    .replace("'svelte'", client('index-client.js'));
  for (const [from, to] of replacements) {
    const pattern = from.startsWith('import {')
      ? new RegExp(from.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/\s+/g, '\\s*'))
      : from;
    code = code.replace(pattern, to);
  }
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}
const page = await component('<script>export function drawn() { return Promise.resolve(); }</script><div class="fake-page">Paper content</div>', 'FakePrintPage.svelte');
const dropdownURL = new URL('../src/lib/components/common/Dropdown.svelte', import.meta.url);
const dropdown = await component(readFileSync(dropdownURL, 'utf8'), fileURLToPath(dropdownURL), [["import Icon from './Icon.svelte';", 'const Icon = () => {};']]);
globalThis.__printUI = { ...printing, ...setup, page, dropdown,
  modalFocus: () => {}, theme: { resolved: 'light' }, loadAttachments: async () => ({}),
};
const common = [
  ["import { printDocument, printMarkdown, setPrintEditor } from '../../print';", 'const { printDocument, printMarkdown, setPrintEditor } = globalThis.__printUI;'],
  ["import { DEFAULT_PRINT_PAGE_SETUP, isPrintPageSetup, printPageDimensions, printPageSetup } from '../../printPageSetup';", 'const { DEFAULT_PRINT_PAGE_SETUP, isPrintPageSetup, printPageDimensions, printPageSetup } = globalThis.__printUI;'],
  ["import { modalFocus } from '../../modalFocus';", 'const { modalFocus } = globalThis.__printUI;'],
  ["import { theme } from '../../stores/theme.svelte';", 'const { theme } = globalThis.__printUI;'],
  ["import PrintPage from './PrintPage.svelte';", 'const PrintPage = globalThis.__printUI.page;'],
  ["import Dropdown from './Dropdown.svelte';", 'const Dropdown = globalThis.__printUI.dropdown;'],
  ["import { loadAttachments } from './PrintSheet.svelte';", 'const { loadAttachments } = globalThis.__printUI;'],
];
const dialogURL = new URL('../src/lib/components/common/PrintDialog.svelte', import.meta.url);
const Dialog = await component(readFileSync(dialogURL, 'utf8'), fileURLToPath(dialogURL), common);
const settle = async () => { for (let i = 0; i < 15; i++) await Promise.resolve(); flushSync(); };
const input = (el, value) => { el.value = value; el.dispatchEvent(new Event('input', { bubbles: true })); flushSync(); };
const open = Component => {
  const target = document.createElement('div'); document.body.appendChild(target);
  const instance = mount(Component, { target }); flushSync();
  return { target, close: async () => { await unmount(instance); target.remove(); } };
};
const choose = (target, label, option) => {
  target.querySelector(`[aria-label="${label}"]`).click(); flushSync();
  const item = [...target.querySelectorAll('[role="option"]')].find(el => el.textContent.trim() === option);
  assert.ok(item, option); item.click(); flushSync();
};

test('print editor controls defaults, changes, validation and dispatch for messages/conversations/files', async () => {
  const documents = [];
  printing.setPrinter(doc => documents.push(doc));
  const d = open(Dialog);
  try {
    printing.printMessage({ sessionRef: { agent: 'codex', id: 'one' }, title: 'Session', parts: [{ kind: 'text', text: 'Conversation' }] }); flushSync();
    const margin = () => d.target.querySelector('[aria-label="Print margins in millimetres"]');
    const print = () => [...d.target.querySelectorAll('button')].find(el => el.textContent.trim() === 'Print…');
    assert.equal(margin().value, '14');
    assert.match(d.target.querySelector('.preview-content').style.width, /^182mm$/);
    choose(d.target, 'Print paper size', 'Letter');
    choose(d.target, 'Print orientation', 'Landscape');
    input(margin(), '10');
    assert.equal(d.target.querySelector('.preview-content').style.width, '259.4mm');
    input(margin(), '');
    assert.equal(print().disabled, true);
    assert.match(d.target.querySelector('[role="alert"]').textContent, /0 to 40/);
    d.target.querySelector('textarea').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }));
    assert.equal(documents.length, 0);
    input(margin(), '41'); assert.equal(print().disabled, true);
    input(margin(), '10'); print().click(); await settle();
    assert.deepEqual(documents[0].pageSetup, { paper: 'letter', orientation: 'landscape', marginMm: 10 });
    assert.equal(documents[0].markdown, 'Conversation');
    assert.equal(d.target.querySelector('[role="dialog"]'), null);
    printing.printFile('/docs/readme.md', '# File'); flushSync();
    assert.equal(margin().value, '10', 'setup retained within window');
    assert.equal(d.target.querySelector('input[placeholder="No title"]'), null);
    print().click(); await settle();
    assert.equal(documents[1].showTitle, false);
    assert.deepEqual(documents[1].pageSetup, documents[0].pageSetup);
    assert.notEqual(documents[1].pageSetup, documents[0].pageSetup);
  } finally { await d.close(); printing.setPrinter(null); }
});

const sheetURL = new URL('../src/lib/components/common/PrintSheet.svelte', import.meta.url);
const Sheet = await component(readFileSync(sheetURL, 'utf8'), fileURLToPath(sheetURL), [
  ["import { loadImage } from '../transcript/parts/ImagePart.svelte';", 'const loadImage = async () => ({});'],
  ["import { api } from '../../api';", 'const api = { printWindow: (...args) => globalThis.__printUI.nativePrint(...args) };'],
  ["import { printTitle, setPrinter } from '../../print';", 'const { printTitle, setPrinter } = globalThis.__printUI;'],
  ["import { DEFAULT_PRINT_PAGE_SETUP, installPrintPageSetup, printPageDimensions, printPageSetup } from '../../printPageSetup';", 'const { DEFAULT_PRINT_PAGE_SETUP, installPrintPageSetup, printPageDimensions, printPageSetup } = globalThis.__printUI;'],
  ["import { toast } from '../../stores/toast.svelte';", 'const toast = { show: notice => globalThis.__printUI.notices.push(notice) };'],
  ["import PrintPage from './PrintPage.svelte';", 'const PrintPage = globalThis.__printUI.page;'],
]);
const doc = { sessionRef: null, title: 'Test PDF', markdown: 'Hi', attachments: [], pageSetup: { paper: 'letter', orientation: 'landscape', marginMm: 10 } };

test('sheet installs setup before native print, matches geometry and restores on afterprint', async () => {
  let called = 0;
  globalThis.__printUI.notices = [];
  globalThis.__printUI.nativePrint = async options => {
    called++;
    assert.deepEqual(options, doc.pageSetup);
    assert.equal(document.title, 'Test PDF');
    assert.equal(document.querySelector('[data-print-page-setup]').textContent, '@page { size: 279.4mm 215.9mm; margin: 10mm; }');
    assert.equal(document.querySelector('.print-sheet').style.maxWidth, '259.4mm');
  };
  const s = open(Sheet);
  try {
    printing.printDocument(doc); await settle();
    assert.equal(called, 1);
    window.dispatchEvent(new Event('afterprint')); flushSync();
    assert.equal(document.title, 'App title');
    assert.equal(document.querySelector('[data-print-page-setup]'), null);
    assert.deepEqual(globalThis.__printUI.notices, []);
  } finally { await s.close(); }
});

test('sheet applies defaults for legacy document callers and never leaks setup after cancellation', async () => {
  let calls = 0;
  globalThis.__printUI.notices = [];
  globalThis.__printUI.nativePrint = async options => { calls++; assert.deepEqual(options, setup.DEFAULT_PRINT_PAGE_SETUP); };
  const s = open(Sheet);
  try {
    printing.printDocument({ ...doc, pageSetup: undefined }); await settle();
    assert.equal(calls, 1);
    window.dispatchEvent(new Event('afterprint'));
    assert.equal(document.querySelector('[data-print-page-setup]'), null);
    printing.printDocument({ ...doc, pageSetup: { ...doc.pageSetup, marginMm: NaN } }); await settle();
    assert.equal(calls, 1, 'invalid setup never reaches native print');
    assert.equal(globalThis.__printUI.notices.length, 1);
    assert.equal(document.title, 'App title');
    assert.equal(document.querySelector('[data-print-page-setup]'), null);
  } finally { await s.close(); }
});

test('sheet restores setup on print failure, fallback interaction and teardown', async () => {
  for (const action of ['error', 'pointer', 'unmount']) {
    globalThis.__printUI.notices = [];
    globalThis.__printUI.nativePrint = async () => { if (action === 'error') throw new Error('native failed'); };
    const s = open(Sheet);
    printing.printDocument(doc); await settle();
    if (action === 'pointer') window.dispatchEvent(new Event('pointerdown'));
    if (action === 'unmount') await s.close();
    assert.equal(document.title, 'App title', action);
    assert.equal(document.querySelector('[data-print-page-setup]'), null, action);
    assert.equal(globalThis.__printUI.notices.length, action === 'error' ? 1 : 0);
    if (action !== 'unmount') await s.close();
  }
});

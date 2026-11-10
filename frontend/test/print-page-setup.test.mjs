import test from 'node:test';
import assert from 'node:assert/strict';
import { JSDOM } from 'jsdom';
import { DEFAULT_PRINT_PAGE_SETUP, isPrintPageSetup, printPageSetup, printPageDimensions, printPageCSS, installPrintPageSetup } from '../src/lib/printPageSetup.ts';
import { withDesktopApp } from './desktop-app.mjs';

const defaults = { paper: 'a4', orientation: 'portrait', marginMm: 14 };

test('print setup defaults to A4 portrait, 14 mm and returns independent copies', () => {
  assert.deepEqual(DEFAULT_PRINT_PAGE_SETUP, defaults);
  const setup = printPageSetup();
  setup.marginMm = 20;
  assert.deepEqual(printPageSetup(), defaults);
  assert.equal(Object.isFrozen(DEFAULT_PRINT_PAGE_SETUP), true);
});

test('print setup validates supported paper/orientation and finite 0–40 mm margins', () => {
  for (const bad of [null, {}, '', { ...defaults, paper: 'legal' }, { ...defaults, orientation: 'sideways' },
    ...[-1, 41, NaN, Infinity, -Infinity, '', '14', undefined].map(marginMm => ({ ...defaults, marginMm }))]) {
    assert.equal(isPrintPageSetup(bad), false, JSON.stringify(bad));
    assert.throws(() => printPageSetup(bad), /margin from 0 to 40 mm/);
  }
  for (const marginMm of [0, 0.5, 14, 40]) assert.equal(isPrintPageSetup({ ...defaults, marginMm }), true);
});

test('paper dimensions and printable widths match A4/Letter and orientation', () => {
  assert.deepEqual(printPageDimensions(defaults), { width: 210, height: 297, contentWidth: 182, contentHeight: 269 });
  assert.deepEqual(printPageDimensions({ ...defaults, paper: 'letter', orientation: 'landscape', marginMm: 10 }),
    { width: 279.4, height: 215.9, contentWidth: 259.4, contentHeight: 195.9 });
  assert.equal(printPageDimensions({ ...defaults, orientation: 'landscape' }).contentWidth, 269);
  assert.equal(printPageDimensions({ ...defaults, marginMm: 40 }).contentWidth, 130);
});

test('CSS declares physical size and one margin without content padding', () => {
  assert.equal(printPageCSS(defaults), '@page { size: 210mm 297mm; margin: 14mm; }');
  assert.equal(printPageCSS({ ...defaults, paper: 'letter', orientation: 'landscape', marginMm: 0 }), '@page { size: 279.4mm 215.9mm; margin: 0mm; }');
  assert.throws(() => printPageCSS({ ...defaults, marginMm: '0; color: red' }));
});

test('temporary page setup rule overrides defaults and is removed idempotently', () => {
  const dom = new JSDOM('<head><style>@page { margin:14mm; }</style></head>');
  const document = dom.window.document;
  const remove = installPrintPageSetup(document, { ...defaults, marginMm: 20 });
  assert.equal(document.head.lastElementChild.textContent, '@page { size: 210mm 297mm; margin: 20mm; }');
  assert.equal(document.querySelectorAll('[data-print-page-setup]').length, 1);
  remove(); remove();
  assert.equal(document.querySelectorAll('[data-print-page-setup]').length, 0);
  assert.equal(document.querySelectorAll('style').length, 1);
  dom.window.close();
});

test('desktop print uses validated setup and falls back only for missing bindings/methods', async () => {
  const seen = [];
  const setup = { ...defaults, paper: 'letter', marginMm: 8 };
  await withDesktopApp({ PrintWindowWithSetup: async options => seen.push(options), PrintWindow: async () => assert.fail('must use setup') }, async api => {
    await api.printWindow(setup);
    await api.printWindow();
    await assert.rejects(api.printWindow({ ...defaults, marginMm: -1 }));
  });
  assert.deepEqual(seen, [setup, defaults]);
  let fallbacks = 0;
  await withDesktopApp({ PrintWindow: async () => fallbacks++ }, api => api.printWindow(setup));
  await withDesktopApp({ PrintWindowWithSetup: async () => { throw new Error('unknown App method "PrintWindowWithSetup"'); }, PrintWindow: async () => fallbacks++ }, api => api.printWindow(setup));
  assert.equal(fallbacks, 2);
  await withDesktopApp({ PrintWindowWithSetup: async () => { throw new Error('printer offline'); }, PrintWindow: async () => assert.fail('no duplicate dialog') }, api => assert.rejects(api.printWindow(setup), /printer offline/));
});

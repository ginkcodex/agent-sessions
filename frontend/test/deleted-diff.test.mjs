import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';
import { compile } from 'svelte/compiler';
import ts from 'typescript';
import { fixtureStore, loadServerComponent, renderWithFixture } from './helpers.mjs';
import { diffRows, diffChangeStarts, stepDiffChange } from '../src/lib/files.ts';

const sourceURL = new URL('../src/lib/components/common/go-to-file/SourceView.svelte', import.meta.url);
const viewerSource = readFileSync(new URL('../src/lib/components/common/go-to-file/FileViewer.svelte', import.meta.url), 'utf8');
const stepSource = viewerSource.slice(viewerSource.indexOf('  function stepChange('), viewerSource.indexOf('  // The button says how the copy went'));
const pickSource = viewerSource.slice(viewerSource.indexOf('  function pick('), viewerSource.indexOf('  // A file link may carry'));
const javascript = source => ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText;
const component = await loadServerComponent(sourceURL, [
  ["import { files } from '../../../stores/files.svelte';", fixtureStore('files')],
  ...['fileSelection', 'sourceNavigation', 'sourceDiagnostics'].map(name => [`'../../../${name}'`, JSON.stringify(new URL(`../src/lib/${name}.ts`, import.meta.url).href)]),
]);
const css = compile(readFileSync(sourceURL, 'utf8'), { generate: 'client', filename: sourceURL.pathname }).css.code;
const changes = [
  { kind: 'deleted', start: 0, count: 0, removed: ['leading removal', ''] },
  { kind: 'deleted', start: 40, count: 0, removed: ['middle removal'] },
  { kind: 'deleted', start: 100, count: 0, removed: ['EOF removal'] },
];

async function renderRows(rows, plainLines, wrap) {
  return renderWithFixture(component, { files: { wrap } }, { rows, plainLines, htmlLines: null, showDiff: true, presentation: false, presentationFontSize: 20 });
}

test('deleted files from Files use HEAD review without overwriting a retained draft', () => {
  for (const accepted of [true, false]) {
    const calls = [];
    const files = { openGitDiff: (...args) => { calls.push(['diff', ...args]); return accepted; }, openFile: (...args) => calls.push(['open', ...args]) };
    const pick = new Function('files', 'fileStatus', `${javascript(pickSource)}; return pick;`)(files, () => 'D');
    pick('gone.ts', true);
    assert.deepEqual(calls, accepted ? [['diff', 'gone.ts', 'head']] : [['diff', 'gone.ts', 'head'], ['open', 'gone.ts', { keep: true, read: false }]]);
  }
});

test('real source renderer includes leading, middle, EOF and deletion-only navigation rows in both layouts', async () => {
  for (const wrap of [false, true]) {
    for (const lineCount of [0, 100]) {
      const rows = diffRows(lineCount ? changes : [{ kind: 'deleted', start: 0, count: 0, removed: ['all removed', ''] }], lineCount);
      const html = await renderRows(rows, Array.from({ length: lineCount }, (_, i) => `line ${i + 1}`), wrap);
      const spans = [...html.matchAll(/<span\b([^>]*)>([\s\S]*?)<\/span>/g)].filter(match => /class="[^"]*\bline\b/.test(match[1]));
      assert.equal(spans.length, rows.length);
      for (const start of diffChangeStarts(rows)) {
        assert.match(spans[start][1], /removed/);
        assert.doesNotMatch(spans[start][1], /data-source-line/);
      }
    }
  }
});

// Opt in to real layout checks without making Chrome a dependency of npm test:
// AS_TEST_CHROME=/path/to/chrome node --loader ./test/resolver.mjs --test test/deleted-diff.test.mjs
// This renders the actual SourceView and runs the actual viewer handler, but
// is not a mounted Svelte lifecycle test or a native Wails application test.
test('Chrome layout scrolls next/previous to removed rows, wrapped and unwrapped', { skip: !process.env.AS_TEST_CHROME }, async () => {
  const dir = mkdtempSync(join(tmpdir(), 'as-deleted-diff-'));
  try {
    for (const wrap of [false, true]) {
      for (const count of [0, 100]) {
        const rows = diffRows(count ? changes : [{ kind: 'deleted', start: 0, count: 0, removed: Array.from({ length: 50 }, (_, i) => `removed ${i}`) }], count);
        const starts = diffChangeStarts(rows);
        const markup = await renderRows(rows, Array.from({ length: count }, (_, i) => `line ${i + 1}`), wrap);
        const script = `
          let codeScroll = document.querySelector('#scroller');
          let changeLines = [...codeScroll.querySelectorAll('.line')];
          let changeStarts = ${JSON.stringify(starts)}, changeCurrent = -1;
          const stepDiffChange = ${stepDiffChange.toString()};
          ${javascript(stepSource)}
          const result = [];
          for (const direction of [1, 1, 1, 1, -1, -1, -1, -1]) {
            stepChange(direction);
            const target = changeLines[changeStarts[changeCurrent]].getBoundingClientRect();
            const viewport = codeScroll.getBoundingClientRect();
            if (!(target.bottom > viewport.top && target.top < viewport.bottom)) throw new Error('removed row outside viewport: ' + changeCurrent);
            result.push(changeCurrent);
          }
          document.body.dataset.result = JSON.stringify(result);
        `;
        const path = join(dir, `fixture-${wrap}-${count}.html`);
        writeFileSync(path, `<!doctype html><style>${css} #scroller{height:200px;width:450px;overflow:auto} body{margin:0}</style><div id="scroller">${markup}</div><script>try{${script}}catch(e){document.body.dataset.error=e.message}</script>`);
        const run = spawnSync(process.env.AS_TEST_CHROME, ['--headless', '--disable-gpu', '--no-first-run', `--user-data-dir=${join(dir, `profile-${wrap}-${count}`)}`, '--dump-dom', pathToFileURL(path).href], { encoding: 'utf8', timeout: 30000, maxBuffer: 4 * 1024 * 1024 });
        assert.equal(run.status, 0, run.stderr || String(run.error));
        assert.doesNotMatch(run.stdout, /data-error=/);
        assert.match(run.stdout, /data-result="/);
      }
    }
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

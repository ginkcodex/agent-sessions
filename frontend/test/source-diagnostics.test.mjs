import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { projectSourceDiagnostics, paintSourceDiagnostics } from '../src/lib/sourceDiagnostics.ts';
import { fixtureStore, loadServerComponent, renderWithFixture } from './helpers.mjs';

const saved = { dirty: false, revision: 'r1' };
const range = (a, b, c = a, d = b) => ({ start: { line: a, character: b }, end: { line: c, character: d } });
function finding(r, changes = {}) {
  const diagnostic = { path: 'x.ts', message: '<img onerror="evil"> & text', source: '<checker>', severity: 'error', freshness: 'current', basis: { kind: 'saved', revision: 'r1' }, ...(r ? { range: r } : {}), ...changes };
  return { diagnostic, entries: [{ providerId: 'checker', diagnostic }] };
}

test('projection uses UTF16 exclusive ends across CRLF, tabs, combining marks and emoji', () => {
  const text = '\t😀e\u0301<&\r\n\r\nlast\r\n';
  const p = projectSourceDiagnostics(text, [finding(range(0, 1, 2, 0)), finding(range(0, 3, 0, 5)), finding(range(3, 0))], saved);
  assert.deepEqual(p.segments, [
    { line: 0, start: 1, end: 7, finding: 0 },
    { line: 1, start: 0, end: 0, finding: 0 },
    { line: 0, start: 3, end: 5, finding: 1 },
    { line: 3, start: 0, end: 0, finding: 2 },
  ]);
  assert.ok(p.messages.every(m => !m.notice));
  assert.equal(projectSourceDiagnostics('', [finding(range(0, 0))], saved).segments.length, 1);
  for (const severity of ['error', 'warning', 'information', 'hint']) {
    assert.equal(projectSourceDiagnostics('x', [finding(range(0, 0, 0, 1), { severity })], saved).segments.length, 1);
  }
});

test('large diagnostic batches index source positions once rather than rescanning for every finding', () => {
  const text = 'const value = 123;\r\n'.repeat(10000);
  const findings = Array.from({ length: 1000 }, (_, i) => finding(range(9999, 0, 9999, 5), { message: `Finding ${i}` }));
  const started = performance.now();
  const projected = projectSourceDiagnostics(text, findings, saved);
  const elapsed = performance.now() - started;
  assert.equal(projected.messages.length, 1000);
  assert.equal(projected.segments.length, 1000);
  assert.ok(projected.messages.every(m => !m.notice));
  // Generous main-thread budget: the repeated scans took over eight seconds.
  assert.ok(elapsed < 2000, `diagnostic projection blocked for ${elapsed.toFixed(0)} ms`);
});

test('versioned live ranges share UTF16 multiline/EOF validation without upgrading saved or uncertain data', () => {
  const basis = { dirty: true, revision: 'r1', live: { documentId: 'doc', version: 3, synced: true } };
  const live = r => finding(r, { basis: { kind: 'document', documentId: 'doc', version: 3 } });
  const p = projectSourceDiagnostics('a😀\n\n', [live(range(0, 1, 2, 0)), live(range(2, 0)), finding(range(0, 0))], basis);
  assert.deepEqual(p.segments, [{ line: 0, start: 1, end: 3, finding: 0 }, { line: 1, start: 0, end: 0, finding: 0 }, { line: 2, start: 0, end: 0, finding: 1 }]);
  for (const changes of [{ freshness: 'uncertain' }, { basis: { kind: 'document', documentId: 'doc' } }, { basis: { kind: 'document', documentId: 'old', version: 3 } }]) {
    assert.equal(projectSourceDiagnostics('a😀\n\n', [finding(range(0, 1), changes)], basis).segments.length, 0);
  }
  assert.equal(projectSourceDiagnostics('a😀', [live(range(0, 2))], basis).segments.length, 0);
});

test('unversioned TypeScript observations paint source ranges without claiming confirmed freshness', () => {
  const f = finding(range(0, 0, 0, 3), { source: 'typescript/ts', freshness: 'uncertain', basis: { kind: 'document', documentId: 'doc', observedVersion: 2 } });
  const basis = { dirty: false, live: { documentId: 'doc', version: 2, synced: true } };
  const projected = projectSourceDiagnostics('sdf', [f], basis);
  assert.deepEqual(projected.segments, [{ line: 0, start: 0, end: 3, finding: 0 }]);
  assert.equal(projected.messages[0].finding.diagnostic.freshness, 'uncertain');
  for (const live of [{ ...basis.live, version: 3 }, { ...basis.live, synced: false }, { ...basis.live, documentId: 'new' }]) {
    assert.equal(projectSourceDiagnostics('sdf', [f], { ...basis, live }).segments.length, 0);
  }
  assert.equal(projectSourceDiagnostics('x', [f], basis).segments.length, 0, 'invalid endpoints are still rejected');
});

test('invalid endpoints never clamp or throw; file-level/stale/unverified/drafts are message-only', () => {
  const invalid = [range(-1, 0), range(0, 2), range(0, 0, 2, 0), range(0, 4, 0, 1), range(0, NaN), range(0, 0.5), { start: null, end: null }];
  for (const r of invalid) {
    const p = projectSourceDiagnostics('a😀b', [finding(r)], saved);
    assert.equal(p.segments.length, 0);
    assert.match(p.messages[0].notice, /outside the displayed text/); assert.equal(p.messages[0].kind, 'invalidRange');
  }
  const cases = [finding(), finding(range(0, 0), { freshness: 'stale' }), finding(range(0, 0), { freshness: 'uncertain' }), finding(range(0, 0), { basis: { kind: 'saved' } }), finding(range(0, 0), { locationIssue: 'invalidRange' }), finding(range(0, 0), { basis: { kind: 'document', documentId: 'd' } })];
  assert.equal(projectSourceDiagnostics('text', cases, saved).segments.length, 0);
  for (const state of [{ dirty: true, revision: 'r1' }, { dirty: false }, { dirty: false, revision: 'r2' }]) assert.equal(projectSourceDiagnostics('text', [finding(range(0, 0))], state).segments.length, 0);
  assert.equal(projectSourceDiagnostics('text', [finding(range(0, 0))], saved, false).segments.length, 0);
});

test('equivalent provider findings select a verified original while retaining all attribution', () => {
  const stale = finding(range(0, 0, 0, 2), { freshness: 'stale' });
  stale.entries.push({ providerId: 'second', diagnostic: finding(range(0, 0, 0, 2)).diagnostic });
  const p = projectSourceDiagnostics('text', [stale], saved);
  assert.equal(p.segments.length, 1);
  assert.equal(p.messages[0].finding.diagnostic.freshness, 'current');
  assert.equal(p.messages[0].finding.entries.length, 2);
  assert.deepEqual(projectSourceDiagnostics('text', [], saved), { messages: [], segments: [], text: 'text' });
});

const base = new URL('../src/lib/components/common/go-to-file/SourceView.svelte', import.meta.url);
const component = await loadServerComponent(base, [
  ["import { files } from '../../../stores/files.svelte';", fixtureStore('files')],
  ["'../../../fileSelection'", JSON.stringify(new URL('../src/lib/fileSelection.ts', import.meta.url).href)], ["'../../../sourceNavigation'", JSON.stringify(new URL('../src/lib/sourceNavigation.ts', import.meta.url).href)],
  ["'../../../sourceDiagnostics'", JSON.stringify(new URL('../src/lib/sourceDiagnostics.ts', import.meta.url).href)],
]);

test('SSR keeps syntax HTML intact, escapes messages and exposes overlaps without replacing Git/selection', async () => {
  const diagnostics = projectSourceDiagnostics('<&text', [finding(range(0, 0, 0, 2)), finding(range(0, 1, 0, 4), { severity: 'warning', code: '<code>' }), finding(), finding(range(0, 1), { freshness: 'stale' })], saved);
  const props = { rows: [{ kind: 'line', index: 0, mark: 'modified' }], htmlLines: ['<span class="hljs-string">&lt;&amp;text</span>'], plainLines: ['<&text'], showDiff: false, presentation: true, presentationFontSize: 24, diagnostics, selection: { anchor: 1, head: 1 } };
  for (const wrap of [true, false]) {
    const html = await renderWithFixture(component, { files: { wrap } }, props);
    assert.match(html, /<span class="hljs-string">&lt;&amp;text<\/span>/);
    assert.doesNotMatch(html, /<img|<checker>|<code>/);
    assert.match(html, /&lt;img/);
    assert.match(html, /diagnostics: error, warning/);
    assert.match(html, /aria-describedby="[^"]+ [^"]+"/);
    assert.match(html, /mark-modified/);
    assert.match(html, /aria-pressed="true"/);
    assert.match(html, /File-level finding/);
    assert.match(html, /Stale: the file or checker changed/);
    assert.doesNotMatch(html, /stale or unverified|remains in Problems only/);
    assert.match(html, /font-size:\s*24px/);
    const diff = await renderWithFixture(component, { files: { wrap } }, { ...props, showDiff: true });
    assert.doesNotMatch(diff, /Source diagnostics|has-diagnostics|diagnostics: error/);
    const plain = await renderWithFixture(component, { files: { wrap } }, { ...props, htmlLines: null });
    assert.match(plain, /&lt;&amp;text/);
  }
});

test('DOM seam merges same-severity paint ranges and cleans only owned highlights', () => {
  const registry = new Map([['file-find', 'keep']]);
  const nodes = [{ textContent: '<😀' }, { textContent: '&end' }];
  const ranges = [];
  let lookups = 0, walks = 0;
  class Highlight { constructor(...ranges) { this.ranges = ranges; } }
  const root = {
    querySelector(selector) { lookups++; assert.equal(selector, '[data-source-line="1"]'); return { textContent: '<😀&end' }; },
    ownerDocument: {
      defaultView: { CSS: { highlights: registry }, Highlight },
      createTreeWalker() { walks++; let i = 0; return { nextNode: () => nodes[i++] ?? null }; },
      createRange() { const r = { setStart(n, o) { this.start = [n, o]; }, setEnd(n, o) { this.end = [n, o]; } }; ranges.push(r); return r; },
    },
  };
  const p = projectSourceDiagnostics('<😀&end', [finding(range(0, 1, 0, 5)), finding(range(0, 3, 0, 6)), finding(range(0, 7))], saved);
  const cleanup = paintSourceDiagnostics(root, p, 'test');
  assert.equal(ranges.length, 1);
  assert.equal(lookups, 1, 'overlapping findings share one line lookup');
  assert.equal(walks, 1, 'overlapping findings share one syntax text walk');
  assert.deepEqual(ranges[0].start, [nodes[0], 1]);
  assert.deepEqual(ranges[0].end, [nodes[1], 3]);
  cleanup();
  assert.deepEqual([...registry], [['file-find', 'keep']]);
  root.querySelector = () => ({ textContent: 'different source' });
  paintSourceDiagnostics(root, p, 'test')();
  assert.equal(ranges.length, 1, 'mismatched rendered text cannot be painted');
  assert.doesNotThrow(() => paintSourceDiagnostics({ ownerDocument: { defaultView: null } }, p, 'test')());
});

test('disjoint same-severity ranges stay separate and painting preserves source validation', () => {
  const registry = new Map();
  class Highlight { constructor(...ranges) { this.ranges = ranges; } }
  const node = { textContent: 'abcdefgh' };
  const root = {
    querySelector() { return node; },
    ownerDocument: {
      defaultView: { CSS: { highlights: registry }, Highlight },
      createTreeWalker() { let done = false; return { nextNode() { if (done) return null; done = true; return node; } }; },
      createRange() { return { setStart(_, offset) { this.start = offset; }, setEnd(_, offset) { this.end = offset; } }; },
    },
  };
  const projection = projectSourceDiagnostics(node.textContent, [finding(range(0, 5, 0, 7)), finding(range(0, 0, 0, 2)), finding(range(0, 1, 0, 3), { severity: 'warning' })], saved);
  paintSourceDiagnostics(root, projection, 'separate');
  assert.deepEqual(registry.get('separate-error').ranges.map(r => [r.start, r.end]), [[0, 2], [5, 7]]);
  assert.deepEqual(registry.get('separate-warning').ranges.map(r => [r.start, r.end]), [[1, 3]]);
});

test('broad overlapping findings keep native paint ranges proportional to lines, not findings', () => {
  const text = 'abcdef\n'.repeat(100);
  const findings = Array.from({ length: 50 }, (_, i) => finding(range(0, 0, 100, 0), { message: `Finding ${i}`, severity: i === 49 ? 'warning' : 'error' }));
  const projection = projectSourceDiagnostics(text, findings, saved);
  const original = projection.segments.map(s => ({ ...s }));
  const registry = new Map();
  let created = 0;
  class Highlight { constructor(...ranges) { this.ranges = ranges; } }
  const node = { textContent: 'abcdef' };
  const root = {
    querySelector() { return { textContent: 'abcdef' }; },
    ownerDocument: {
      defaultView: { CSS: { highlights: registry }, Highlight },
      createTreeWalker() { let done = false; return { nextNode() { if (done) return null; done = true; return node; } }; },
      createRange() { created++; return { setStart() {}, setEnd() {} }; },
    },
  };
  const cleanup = paintSourceDiagnostics(root, projection, 'broad');
  assert.equal(projection.messages.length, 50, 'all findings retain attribution');
  assert.equal(projection.segments.length, 5000, 'all gutter locations remain available');
  assert.equal(created, 200, 'one native range per covered line and severity');
  assert.equal(registry.get('broad-error').ranges.length, 100);
  assert.equal(registry.get('broad-warning').ranges.length, 100);
  assert.deepEqual(projection.segments, original, 'painting must not mutate gutter segments');
  cleanup();
  assert.equal(registry.size, 0);
});

test('viewer gates overlays to the owned source file and retains lazy editor boundary', () => {
  const viewer = readFileSync(new URL('../src/lib/components/common/go-to-file/FileViewer.svelte', import.meta.url), 'utf8');
  assert.match(viewer, /codeDiagnostics.directory === files.dir/);
  assert.match(viewer, /!showDiff && !showPreview && !blameOpen/);
  assert.match(viewer, /f.diagnostic.path === files.selected/);
  assert.match(viewer, /edit\?\.dirty \|\| edit\?\.uncertain \|\| edit\?\.conflict/);
  assert.doesNotMatch(viewer, /from ['"]@codemirror/);
});

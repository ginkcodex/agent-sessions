import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerModule, loadServerComponent, renderWithFixture } from './helpers.mjs';
import { projectSourceDiagnostics } from '../src/lib/sourceDiagnostics.ts';
import { problemLocation } from '../src/lib/problemNavigation.ts';
import { editorDiagnostics } from '../src/lib/fileEditor.ts';
import { EditorState } from '@codemirror/state';
import { LiveDocumentBridge } from '../src/lib/liveDocuments.ts';

const lib = name => JSON.stringify(new URL(`../src/lib/${name}.ts`, import.meta.url).href);
const { CodeDiagnosticsStore } = await loadServerModule(new URL('../src/lib/stores/codeDiagnostics.svelte.ts', import.meta.url), [
  ["import { api } from '../api';", 'const api = {};'],
  ["import { epoch } from '../link';", 'const epoch = () => 0;'],
  ["'../codeDiagnostics'", lib('codeDiagnostics')], ["'../codeDiagnosticCounts'", lib('codeDiagnosticCounts')], ["'../codeNavigation'", lib('codeNavigation')],
]);
const { FilesStore } = await loadServerModule(new URL('../src/lib/stores/files.svelte.ts', import.meta.url), [
  ["import { api } from '../api';", fixtureStore('api', '__lifecycleFixture')],
  ["import { link } from './link.svelte';", fixtureStore('link', '__lifecycleFixture')],
  ...['manage', 'link', 'files', 'fileEditing'].map(n => [`'../${n}'`, lib(n)]),
]);
const tick = async () => { for (let i = 0; i < 8; i++) await new Promise(r => setImmediate(r)); };
const context = { connectionId: 'c', attachmentId: 'a', generation: 'g' };
const provider = id => ({ providerId: id, availability: 'available', phase: 'idle', freshness: 'uncertain', truncated: false, coverage: { kind: 'openedDocuments', roots: [''], partial: false, externalChanges: 'unwatched' } });
const content = text => ({ path: 'a.go', text, revision: 'r1', size: text.length, edit: { eligible: true, newline: 'lf' } });

async function harness(live = true, { path = 'a.go', providers = [provider('lsp'), provider('checker')], navigation = false, completion = false } = {}) {
  const calls = [], listeners = new Map(), timers = new Map();
  const fileContent = text => ({ ...content(text), path });
  let seq = 0, id = 0, doc;
  globalThis.__lifecycleFixture = {
    link: { dataHost: 'h', blockedReason: null },
    api: { listFiles: async () => ({ root: '/p', files: [path] }), readFile: async () => fileContent('a😀\r\nb'), saveFile: async (_d, r) => ({ status: 'saved', content: { ...fileContent(r.text), revision: 'r2' } }) },
  };
  const snapshot = sets => ({ contractVersion: 1, context, sequence: seq++, approval: { state: 'approved', mode: 'session', fingerprint: 'fp' }, providers, resultSets: sets });
  const api = {
    onEvent(n, fn) { listeners.set(n, fn); return () => listeners.delete(n); },
    async attachCodeDiagnostics() { return { project: { hostId: 'h', root: '/p' }, context, snapshot: snapshot([]) }; },
    async detachCodeDiagnostics() { return { context }; },
    async openCodeDiagnosticDocument(c, path, documentId, version, text) { calls.push(['open', version, text]); doc = { documentId, version }; return { context: c, ...doc }; },
    async changeCodeDiagnosticDocument(c, documentId, version, text) { calls.push(['change', version, text]); doc = { documentId, version }; return { context: c, ...doc }; },
    async saveCodeDiagnosticDocument(c, documentId, version) { calls.push(['save', version]); return { context: c, documentId }; },
    async closeCodeDiagnosticDocument(c, documentId) { calls.push(['close']); return { context: c, documentId }; },
  };
  const files = new FilesStore(), checks = new CodeDiagnosticsStore(api, () => 0);
  const bridge = new LiveDocumentBridge({ api, store: checks, source: () => ({ dir: files.dir, documents: files.displayedDocuments() }), newId: () => `d${++id}`, timers: { set(fn) { timers.set(fn, fn); return fn; }, clear(fn) { timers.delete(fn); } } });
  checks.setLiveBasis(path => bridge.basis(path));
  checks.setLiveFlush((p, bound) => bridge.flush(p, bound));
  checks.setLiveRecovery(() => bridge.statuses, (path, id) => bridge.reopen(path, id));
  bridge.onChange(() => checks.liveChanged());
  files.observe(bridge);
  const off = checks.onLifecycle(() => bridge.sync());
  await files.show('/p');
  await checks.attach('/p', { phase: 'connected', generation: 1, hostId: 'h', capabilities: { codeDiagnostics: true, ...(live ? { liveCodeDiagnostics: true } : {}), ...(navigation ? { codeNavigation: true } : {}), ...(completion ? { codeCompletion: true } : {}) } });
  function emit(version = doc?.version, documentId = doc?.documentId, providers = ['lsp']) {
    const d = { path: 'a.go', severity: 'error', message: '<script>&😀', source: 'fixture', freshness: version ? 'current' : 'uncertain', basis: { kind: 'document', documentId, ...(version ? { version } : {}) }, range: { start: { line: 0, character: 1 }, end: { line: 0, character: 3 } } };
    const s = snapshot(providers.map(providerId => ({ providerId, origin: 'lsp', scope: { kind: 'document', path: 'a.go', documentId }, diagnostics: [d] })));
    listeners.get('codeDiagnostics:updated')?.({ contractVersion: 1, context, sequence: s.sequence, updateId: `u${s.sequence}`, partIndex: 0, partCount: 1, snapshot: s });
  }
  return { files, checks, bridge, api, snapshot, calls, timers, emit, listeners, off, async flush() { for (const fn of [...timers.keys()]) { timers.delete(fn); fn(); } await tick(); } };
}

test('real files + diagnostics stores: freshness, attribution, save, close and host drop', async () => {
  const h = await harness();
  h.files.openFile('a.go', { keep: true }); await tick();
  const inline = () => h.checks.liveInline('a.go', h.bridge.basis('a.go'));
  h.emit(undefined, undefined, ['lsp', 'checker']);
  assert.equal(h.checks.counts.error, 1);
  assert.equal(h.checks.findings[0].entries.length, 2);
  assert.equal(inline().length, 1);
  h.emit(undefined, undefined, ['checker']); // full replacement leaves second attribution
  assert.equal(h.checks.counts.error, 1);
  assert.equal(h.checks.findings[0].entries[0].providerId, 'checker');
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'draft😀');
  assert.equal(inline().length, 0); // before debounce
  h.emit(); assert.equal(inline().length, 0); // even a matching old push cannot restore it
  await h.flush();
  const version = h.bridge.basis('a.go').version;
  h.emit(version - 1); assert.equal(inline().length, 0);
  h.emit(null); assert.equal(inline().length, 0);
  h.emit(version, 'old-document'); assert.equal(inline().length, 0);
  h.emit(); assert.equal(inline().length, 1);
  h.files.updateDraft('a.go', 'saved😀');
  await h.files.saveEdited('a.go'); await tick();
  assert.deepEqual(h.calls.slice(-2), [['change', version + 1, 'saved😀'], ['save', version + 1]]);
  h.files.updateDraft('a.go', 'retained draft');
  const late = h.listeners.get('codeDiagnostics:updated');
  h.checks.disconnect();
  assert.equal(h.timers.size, 0);
  await h.flush();
  assert.equal(h.files.activeTab.edit.draft, 'retained draft');
  assert.equal(inline().length, 0);
  late({ contractVersion: 1, context, sequence: 999 });
  assert.equal(h.checks.status, 'disconnected');
  assert.equal(h.calls.at(-1)[0], 'save');
  h.off(); await h.checks.detach();
});

test('R07 real store source/component/editor projection and navigation require the displayed live identity', async () => {
  const component = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/SourceView.svelte', import.meta.url), [
    ["import { files } from '../../../stores/files.svelte';", fixtureStore('files')],
    ["'../../../fileSelection'", lib('fileSelection')], ["'../../../sourceNavigation'", lib('sourceNavigation')], ["'../../../sourceDiagnostics'", lib('sourceDiagnostics')],
  ]);
  const h = await harness();
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go');
  h.files.updateDraft('a.go', 'a😀\nb\n'); await h.flush();
  const tab = h.files.activeTab, edit = tab.edit, version = edit.version;
  function projection(expected) {
    const basis = { dirty: true, revision: edit.revision, live: h.checks.liveBasis(tab.path) };
    const p = projectSourceDiagnostics(edit.draft, h.checks.findings, basis, h.checks.status === 'ready');
    assert.equal(p.segments.length, expected);
    const options = { id: tab, path: tab.path, text: edit.draft, diagnostics: { tab, path: tab.path, context: 'fixture', findings: h.checks.findings, ...basis, current: h.checks.status === 'ready' } };
    assert.equal(editorDiagnostics(EditorState.create({ doc: edit.draft }), options).length, expected);
    if (h.checks.findings.length) assert.equal(!problemLocation(h.checks.findings[0].diagnostic, basis), !!expected);
    assert.equal(edit.version, version);
    return p;
  }
  h.emit();
  const p = projection(1);
  const html = await renderWithFixture(component, { files: { wrap: false } }, { rows: [{ kind: 'line', index: 0 }], plainLines: ['a😀'], htmlLines: null, showDiff: false, diagnostics: p });
  assert.match(html, /has-diagnostics/);
  assert.doesNotMatch(html, /<script>/);
  const live = h.checks.liveBasis('a.go');
  h.emit(live.version, 'another-document'); projection(0);
  h.emit(live.version - 1); projection(0);
  h.emit(live.version + 1); projection(0);
  h.emit(null); projection(0);
  h.emit(); projection(1);
  assert.equal(h.checks.liveBasis('other.go'), undefined);
  // Wrong URI remains panel-only even with a valid document ID/version.
  const wrongPath = h.checks.findings.map(f => ({ ...f, diagnostic: { ...f.diagnostic, path: 'other.go' } }));
  assert.equal(editorDiagnostics(EditorState.create({ doc: edit.draft }), { id: tab, path: 'a.go', diagnostics: { tab, path: 'a.go', findings: wrongPath, dirty: true, live, current: true } }).length, 0);
  h.files.updateDraft(tab, 'a😀\nnew\n'); projection(0); // immediate, before debounce
  h.emit(); projection(0);
  await h.flush(); h.emit(); projection(1);
  h.checks.revoking = true; // pending revoke suppresses basis before its async reply
  assert.equal(h.checks.liveBasis('a.go'), undefined);
  projection(0);
  h.checks.disconnect(); projection(0);
  assert.equal(edit.draft, 'a😀\nnew\n');
  h.off(); await h.checks.detach();
});

const checksComponent = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/CodeChecks.svelte', import.meta.url), [
  ["import { codeDiagnostics as checks } from '../../../stores/codeDiagnostics.svelte';", fixtureStore('checks')],
]);
const problemsComponent = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/ProblemsPanel.svelte', import.meta.url), [
  ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
  ["import { codeDiagnostics as checks } from '../../../stores/codeDiagnostics.svelte';", fixtureStore('checks')],
  ["'../../../codeDiagnostics'", lib('codeDiagnostics')], ["'../../../problemNavigation'", lib('problemNavigation')], ["'../../../codeCheckSummary'", lib('codeCheckSummary')],
]);

test('R08 provider crash/truncation propagates through actual store, panel, counts and marker projections', async () => {
  const h = await harness();
  h.files.openFile('a.go', { keep: true }); await tick(); h.emit();
  const s = h.snapshot(h.checks.snapshot.resultSets);
  Object.assign(s.providers[0], { phase: 'failed', failure: { code: 'toolFailed', message: '<script>crash</script>' }, truncated: true, notice: '<img src=x> shortened' });
  s.providers[1].availability = 'missingTool';
  h.listeners.get('codeDiagnostics:updated')({ contractVersion: 1, context, sequence: s.sequence, updateId: 'failure', partIndex: 0, partCount: 1, snapshot: s });
  assert.equal(h.checks.status, 'ready'); // provider failure is not a browsing failure
  assert.equal(h.checks.counts.error, 1); assert.equal(h.checks.countsUnder('a.go'), 1);
  assert.equal(h.checks.findings[0].diagnostic.freshness, 'stale');
  assert.equal(h.checks.partial, true); assert.equal(h.checks.truncated, true);
  assert.equal(h.checks.liveInline('a.go', h.checks.liveBasis('a.go')).length, 0);
  const basis = { dirty: false, revision: 'r1', live: h.checks.liveBasis('a.go') };
  assert.equal(projectSourceDiagnostics(h.files.content.text, h.checks.findings, basis, true).segments.length, 0);
  assert.equal(editorDiagnostics(EditorState.create({ doc: h.files.content.text }), { id: 't', path: 'a.go', diagnostics: { tab: 't', path: 'a.go', findings: h.checks.findings, ...basis, current: true } }).length, 0);
  const html = await renderWithFixture(checksComponent, { checks: h.checks });
  for (const text of ['failed', 'Not installed', 'partial coverage', 'truncated', '&lt;script>', '&lt;img', 'bounded backoff', 'Retry lsp']) assert.ok(html.includes(text), text);
  assert.doesNotMatch(html, /<script>|<img/);
  const panel = await renderWithFixture(problemsComponent, { checks: h.checks }, { expanded: true, onnavigate() {} });
  assert.match(panel, /stale/); assert.match(panel, /results truncated/); assert.doesNotMatch(panel, /<script>/);
  let retried = false;
  h.api.runCodeDiagnostics = async (c, ids) => { assert.deepEqual(ids, ['lsp']); retried = true; return { context: c, runs: [] }; };
  await h.checks.retry('lsp');
  assert.equal(retried, true);
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'still browsable');
  assert.equal(h.files.activeTab.edit.draft, 'still browsable');
  h.off(); await h.checks.detach();
});

test('R08 bridge failure is rendered without raw error text; explicit recovery is bounded and identity guarded', async () => {
  const h = await harness();
  h.files.openFile('a.go', { keep: true }); await tick(); h.emit();
  h.api.changeCodeDiagnosticDocument = async () => { throw new Error('toolFailed: SECRET DOCUMENT TEXT'); };
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'latest displayed text'); await h.flush();
  const failed = h.checks.liveDocuments[0];
  assert.equal(failed.failure, 'toolFailed'); assert.equal(failed.canReopen, true);
  assert.equal(h.checks.findings[0].diagnostic.freshness, 'stale'); assert.equal(h.checks.partial, true);
  const html = await renderWithFixture(checksComponent, { checks: h.checks });
  assert.match(html, /Live document synchronization failed \(toolFailed\)/); assert.match(html, /Reopen live synchronization/);
  assert.doesNotMatch(html, /SECRET DOCUMENT TEXT|latest displayed text/);
  h.checks.reopenDocument('a.go', 'wrong-id'); await tick();
  assert.equal(h.checks.liveDocuments[0].documentId, failed.documentId);
  h.checks.reopenDocument('a.go', failed.documentId); await tick();
  assert.notEqual(h.checks.liveDocuments[0].documentId, failed.documentId);
  assert.deepEqual(h.calls.at(-1), ['open', 1, 'latest displayed text']);
  const fresh = h.checks.liveBasis('a.go');
  assert.equal(h.checks.findings[0].diagnostic.freshness, 'stale');
  h.emit(1, failed.documentId); assert.equal(h.checks.liveInline('a.go', fresh).length, 0);
  h.emit(1, fresh.documentId); assert.equal(h.checks.liveInline('a.go', fresh).length, 1);
  for (let n = 0; n < 2; n++) {
    h.files.updateDraft('a.go', `next${n}`); await h.flush();
    const d = h.checks.liveDocuments[0];
    h.checks.reopenDocument(d.path, d.documentId); await tick();
  }
  assert.equal(h.checks.liveDocuments[0].canReopen, false);
  const calls = h.calls.length, d = h.checks.liveDocuments[0];
  h.checks.reopenDocument(d.path, d.documentId); await tick(); assert.equal(h.calls.length, calls);
  h.checks.revoking = true;
  assert.deepEqual(h.checks.liveDocuments, []); h.checks.reopenDocument(d.path, d.documentId);
  await tick(); assert.equal(h.calls.length, calls);
  h.off(); await h.checks.detach();
});

test('R08 superseded open/change promises cannot poison a new attachment or restore markers', async () => {
  for (const operation of ['openCodeDiagnosticDocument', 'changeCodeDiagnosticDocument']) {
    const h = await harness();
    let reject;
    if (operation === 'changeCodeDiagnosticDocument') { h.files.openFile('a.go', { keep: true }); await tick(); }
    h.api[operation] = () => new Promise((_resolve, no) => { reject = no; });
    if (operation === 'openCodeDiagnosticDocument') { h.files.openFile('a.go', { keep: true }); await tick(); }
    else { h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'held'); await h.flush(); }
    const old = h.checks.liveDocuments[0].documentId;
    h.checks.disconnect();
    await h.checks.attach('/p', { phase: 'connected', generation: 2, hostId: 'h', capabilities: { codeDiagnostics: true, liveCodeDiagnostics: true } });
    h.api.openCodeDiagnosticDocument = async () => ({});
    reject(new Error('toolFailed: old body')); await tick();
    assert.equal(h.checks.status, 'ready');
    assert.notEqual(h.checks.liveDocuments[0].documentId, old);
    assert.equal(h.checks.liveDocuments[0].failure, undefined);
    h.emit(1, old); assert.equal(h.checks.liveInline('a.go', h.checks.liveBasis('a.go')).length, 0);
    h.off(); await h.checks.detach();
  }
});

test('old remote missing live flag keeps bridge entirely inert through edit/save/close', async () => {
  const h = await harness(false);
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'draft');
  await h.flush(); await h.files.saveEdited('a.go');
  h.files.closeTab('a.go'); await tick();
  assert.deepEqual(h.calls, []);
  assert.equal(h.timers.size, 0);
  assert.equal(h.checks.liveAttachment, undefined);
  h.off(); await h.checks.detach();
});

test('language coverage: saved and unsaved navigation uses acknowledged Go, TS/JS and Python documents', async t => {
  const languages = [
    { providerId: 'gopls', root: 'go', extensions: ['.go'] },
    { providerId: 'typescript', root: 'web', extensions: ['.ts', '.mts', '.cts', '.tsx', '.js', '.mjs', '.cjs', '.jsx'] },
    { providerId: 'pyright', root: 'svc', extensions: ['.py', '.pyi'] },
  ];
  for (const language of languages) for (const extension of language.extensions) {
    const path = `${language.root}/source${extension}`;
    await t.test(`${language.providerId} ${extension}`, async () => {
      const providers = [{ ...provider(language.providerId), documentExtensions: language.extensions,
        coverage: { ...provider(language.providerId).coverage, roots: [language.root] } }];
      // Python shares one live document between Ruff diagnostics and Pyright navigation.
      if (language.providerId === 'pyright') providers.unshift({ ...providers[0], providerId: 'ruff' });
      const h = await harness(true, { path, providers, navigation: true });
      try {
        assert.equal(h.checks.liveSupports(path), true);
        assert.equal(h.checks.liveSupports(`${language.root}-other/source${extension}`), false, 'coverage root is a path segment, not a prefix');
        assert.match(h.checks.navigationBlocker(`outside/source${extension}`), /No language server/);
        assert.match(h.checks.navigationBlocker(`${language.root}/readme.md`), /No language server/);
        h.files.openFile(path, { keep: true }); await tick();
        assert.deepEqual(h.calls, [['open', 1, 'a😀\r\nb']]);
        let acknowledged = h.calls[0][2];
        const target = `${language.root}/target${extension}`;
        const position = { line: 0, character: 3 }; // UTF-16 position after the astral character
        h.api.navigateCode = async (c, documentId, version, op, at, host) => {
          assert.deepEqual(c, context); assert.equal(host, 'h');
          assert.equal(documentId, h.bridge.basis(path).documentId);
          assert.equal(version, h.bridge.basis(path).version);
          assert.equal(h.bridge.basis(path).synced, true);
          assert.equal(acknowledged, h.files.activeTab.edit?.draft ?? h.files.content.text);
          assert.deepEqual(at, position);
          h.calls.push(['navigate', op, version, acknowledged]);
          return { op, documentId, version, ...(op === 'hover'
            ? { hover: { kind: 'plaintext', value: `${language.providerId} symbol` } }
            : { locations: [{ path: target, range: { start: position, end: position } }] }) };
        };
        const saved = await h.checks.navigate(path, 'definition', position);
        assert.equal(saved.kind, 'result'); assert.equal(saved.result.locations[0].path, target);
        assert.equal(h.calls.at(-1)[2], 1);

        const change = h.api.changeCodeDiagnosticDocument;
        let release;
        h.api.changeCodeDiagnosticDocument = (...args) => new Promise(resolve => {
          release = async () => { const reply = await change(...args); acknowledged = args[3]; resolve(reply); };
        });
        h.files.startEditing(path);
        h.files.updateDraft(path, 'a😀 unsaved\nsecond line\n');
        assert.equal(h.bridge.basis(path).synced, false);
        const asking = h.checks.navigate(path, 'definition', position, { wait: 1000 });
        assert.equal(h.calls.filter(c => c[0] === 'navigate').length, 1, 'no navigation against the saved version during debounce');
        await h.flush();
        assert.equal(typeof release, 'function');
        assert.equal(h.bridge.basis(path).synced, false);
        assert.equal(h.calls.filter(c => c[0] === 'navigate').length, 1, 'sending a change is not an acknowledgement');
        await release();
        const unsaved = await asking;
        assert.equal(unsaved.kind, 'result');
        assert.equal(unsaved.result.locations[0].path, target);
        const draftVersion = h.bridge.basis(path).version;
        assert.ok(draftVersion > 1, 'draft navigation must not reuse the saved version');
        assert.deepEqual(h.calls.slice(-2), [['change', draftVersion, acknowledged], ['navigate', 'definition', draftVersion, acknowledged]]);
        assert.equal(h.calls.some(c => c[0] === 'save'), false, 'unsaved navigation does not save the file');

        await h.files.saveEdited(path); await tick();
        assert.deepEqual(h.calls.at(-1), ['save', draftVersion], 'saving the acknowledged draft preserves its live version');
        for (const op of ['references', 'hover']) {
          const out = await h.checks.navigate(path, op, position);
          assert.equal(out.kind, 'result'); assert.equal(out.result.version, draftVersion);
          if (op === 'hover') assert.equal(out.result.hover.value, `${language.providerId} symbol`);
          else assert.equal(out.result.locations[0].path, target);
        }
        h.files.closeTab(path); await tick();
        assert.deepEqual(h.calls.at(-1), ['close']);
        assert.equal(h.bridge.basis(path), undefined);
      } finally { h.off(); await h.checks.detach(); }
    });
  }
});

test('vtsls shape: TS/JS extensions go live inside covered packages; unversioned pushes count in the tree but never draw inline', async () => {
  const listeners = new Map();
  let seq = 0;
  const lsp = (providerId, extra) => ({ ...provider(providerId), coverage: { kind: 'openedDocuments', roots: [''], partial: true, externalChanges: 'unwatched' }, ...extra });
  const providers = [lsp('gopls', { documentExtensions: ['.go'] }), lsp('typescript', { coverage: { kind: 'openedDocuments', roots: ['web'], partial: true, externalChanges: 'unwatched' }, documentExtensions: ['.ts', '.mts', '.cts', '.tsx', '.js', '.mjs', '.cjs', '.jsx'] }),
    lsp('svelte', { availability: 'unsupported' })];
  const snapshot = sets => ({ contractVersion: 1, context, sequence: seq++, approval: { state: 'approved', mode: 'session', fingerprint: 'fp' }, providers, resultSets: sets });
  const api = { onEvent(n, fn) { listeners.set(n, fn); return () => listeners.delete(n); }, async attachCodeDiagnostics() { return { project: { hostId: 'h', root: '/p' }, context, snapshot: snapshot([]) }; }, async detachCodeDiagnostics() { return { context }; } };
  const checks = new CodeDiagnosticsStore(api, () => 0);
  await checks.attach('/p', { phase: 'connected', generation: 1, hostId: 'h', capabilities: { codeDiagnostics: true, liveCodeDiagnostics: true } });
  for (const [path, live] of [['web/b.mjs', true], ['web/src/a.TS', true], ['web/c.cjs', true], ['a.go', true], ['b.mjs', false], ['web/c.svelte', false], ['web/readme.md', false]])
    assert.equal(checks.liveSupports(path), live, path);
  // vtsls publishes without a version: uncertain, no range, still a red tree count.
  const d = { path: 'web/b.mjs', severity: 'error', message: 'Expression expected.', code: '1109', source: 'typescript/ts', freshness: 'uncertain', basis: { kind: 'document', documentId: 'd1' } };
  const s = snapshot([{ providerId: 'typescript', origin: 'lsp', scope: { kind: 'document', path: 'web/b.mjs', documentId: 'd1' }, diagnostics: [d] }]);
  listeners.get('codeDiagnostics:updated')({ contractVersion: 1, context, sequence: s.sequence, updateId: 'ts', partIndex: 0, partCount: 1, snapshot: s });
  assert.equal(checks.counts.error, 1);
  assert.equal(checks.countsUnder('web'), 1); assert.equal(checks.countsUnder('web/b.mjs'), 1);
  assert.equal(checks.pathCounts.folders.get('web').uncertain, true);
  assert.equal(checks.liveInline('web/b.mjs', { documentId: 'd1', version: 1, synced: true }).length, 0);
  // A newer backend retains unversioned vtsls ranges as uncertain observations.
  const observed = { ...d, range: { start: { line: 0, character: 4 }, end: { line: 0, character: 5 } }, basis: { ...d.basis, observedVersion: 1 } };
  const positioned = snapshot([{ providerId: 'typescript', origin: 'lsp', scope: { kind: 'document', path: 'web/b.mjs', documentId: 'd1' }, diagnostics: [observed] }]);
  listeners.get('codeDiagnostics:updated')({ contractVersion: 1, context, sequence: positioned.sequence, updateId: 'observed', partIndex: 0, partCount: 1, snapshot: positioned });
  assert.equal(checks.liveInline('web/b.mjs', { documentId: 'd1', version: 1, synced: true }).length, 1);
  assert.equal(checks.liveInline('web/b.mjs', { documentId: 'd1', version: 2, synced: true }).length, 0, 'edit hides the old observation');
  assert.equal(checks.liveInline('web/b.mjs', { documentId: 'd1', version: 1, synced: false }).length, 0, 'pending sync hides it');
  assert.equal(checks.liveInline('web/b.mjs', { documentId: 'reopened', version: 1, synced: true }).length, 0, 'reopen never reuses it');
  assert.equal(checks.pathCounts.folders.get('web').uncertain, true, 'not misrepresented as server-confirmed');
  // The empty publication clears it.
  const clear = snapshot([{ providerId: 'typescript', origin: 'lsp', scope: { kind: 'document', path: 'web/b.mjs', documentId: 'd1' }, diagnostics: [] }]);
  listeners.get('codeDiagnostics:updated')({ contractVersion: 1, context, sequence: clear.sequence, updateId: 'clear', partIndex: 0, partCount: 1, snapshot: clear });
  assert.equal(checks.counts.error, 0); assert.equal(checks.countsUnder('web'), 0);
  await checks.detach();
});

test('Ruff shape: .py/.pyi go live inside covered projects; versioned pushes draw inline for the matching version only', async () => {
  const listeners = new Map();
  let seq = 0;
  const lsp = (providerId, extra) => ({ ...provider(providerId), coverage: { kind: 'openedDocuments', roots: [''], partial: true, externalChanges: 'unwatched' }, ...extra });
  const providers = [lsp('gopls', { documentExtensions: ['.go'] }), lsp('typescript', { availability: 'unsupported' }), lsp('svelte', { availability: 'unsupported' }),
    lsp('ruff', { coverage: { kind: 'openedDocuments', roots: ['svc'], partial: false, externalChanges: 'unwatched' }, documentExtensions: ['.py', '.pyi'] })];
  const snapshot = sets => ({ contractVersion: 1, context, sequence: seq++, approval: { state: 'approved', mode: 'session', fingerprint: 'fp' }, providers, resultSets: sets });
  const api = { onEvent(n, fn) { listeners.set(n, fn); return () => listeners.delete(n); }, async attachCodeDiagnostics() { return { project: { hostId: 'h', root: '/p' }, context, snapshot: snapshot([]) }; }, async detachCodeDiagnostics() { return { context }; } };
  const checks = new CodeDiagnosticsStore(api, () => 0);
  await checks.attach('/p', { phase: 'connected', generation: 1, hostId: 'h', capabilities: { codeDiagnostics: true, liveCodeDiagnostics: true } });
  for (const [path, live] of [['svc/app/a.py', true], ['svc/a.pyi', true], ['x.py', false], ['svc/n.ipynb', false], ['svc/b.ts', false], ['svc/pyproject.toml', false]])
    assert.equal(checks.liveSupports(path), live, path);
  // Ruff publishes with the document version: current, positioned, inline for that version.
  const range = { start: { line: 0, character: 7 }, end: { line: 0, character: 9 } };
  const d = { path: 'svc/app/a.py', severity: 'warning', message: '`os` imported but unused\n\nhelp: Remove unused import: `os`', code: 'F401', source: 'ruff/Ruff', freshness: 'current', range, basis: { kind: 'document', documentId: 'd1', version: 2 } };
  const s = snapshot([{ providerId: 'ruff', origin: 'lsp', scope: { kind: 'document', path: 'svc/app/a.py', documentId: 'd1' }, diagnostics: [d] }]);
  listeners.get('codeDiagnostics:updated')({ contractVersion: 1, context, sequence: s.sequence, updateId: 'ruff', partIndex: 0, partCount: 1, snapshot: s });
  assert.equal(checks.counts.warning, 1); assert.equal(checks.countsUnder('svc'), 1);
  assert.equal(checks.liveInline('svc/app/a.py', { documentId: 'd1', version: 2, synced: true }).length, 1);
  assert.equal(checks.liveInline('svc/app/a.py', { documentId: 'd1', version: 3, synced: true }).length, 0, 'never drawn on newer text');
  const clear = snapshot([{ providerId: 'ruff', origin: 'lsp', scope: { kind: 'document', path: 'svc/app/a.py', documentId: 'd1' }, diagnostics: [] }]);
  listeners.get('codeDiagnostics:updated')({ contractVersion: 1, context, sequence: clear.sequence, updateId: 'clear', partIndex: 0, partCount: 1, snapshot: clear });
  assert.equal(checks.counts.warning, 0); assert.equal(checks.countsUnder('svc'), 0);
  await checks.detach();
});

test('completionBasis sends pending typing at once and names the attachment it was confirmed on', async () => {
  const h = await harness(true, { completion: true });
  assert.equal(h.checks.completionEnabled, true);
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'package a\n'); await tick();
  assert.equal(h.timers.size, 1); // the debounce
  const before = h.bridge.basis('a.go');
  assert.equal(before.synced, false);
  const got = await h.checks.completionBasis('a.go');
  const version = before.version + 1;
  assert.deepEqual(got, { context, host: 'h', basis: { documentId: 'd1', version, synced: true } });
  assert.deepEqual(h.calls.at(-1), ['change', version, 'package a\n']);
  assert.equal(h.timers.size, 0);
  // The attachment ends while the change is answered: no basis.
  h.files.updateDraft('a.go', 'package b\n');
  const late = h.checks.completionBasis('a.go');
  h.checks.disconnect();
  assert.equal(await late, undefined);
  h.off();
});

test('completionBasis is undefined without the completion capability and sends nothing', async () => {
  const h = await harness(true, { navigation: true });
  assert.equal(h.checks.completionEnabled, false);
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'package a\n'); await tick();
  const sent = h.calls.length;
  assert.equal(await h.checks.completionBasis('a.go'), undefined);
  assert.equal(h.calls.length, sent);
  assert.equal(h.timers.size, 1); // the debounce is left as it was
  h.off();
});

test('complete asks at the confirmed version under a fresh request ID and drops answers for another document', async () => {
  const h = await harness(true, { completion: true });
  const asks = [], cancels = [];
  let answer = (documentId, version) => ({ documentId, version, resultId: 'r1', items: [{ label: 'Println', detail: 'func()' }], triggerCharacters: '.' });
  h.api.completeCode = async (c, requestId, documentId, version, position, trigger, host) => { asks.push({ c, requestId, documentId, version, position, trigger, host }); return answer(documentId, version); };
  h.api.cancelCodeCompletion = async requestId => { cancels.push(requestId); };
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'fmt.\n'); await tick();
  assert.equal(h.checks.completionBlocker('a.go'), undefined);
  const dot = { kind: 'character', character: '.' }, at = { line: 0, character: 4 };
  const got = await h.checks.complete('a.go', at, dot, { current: () => true });
  const version = h.bridge.basis('a.go').version;
  assert.deepEqual(asks[0], { c: context, requestId: asks[0].requestId, documentId: 'd1', version, position: at, trigger: dot, host: 'h' });
  assert.match(asks[0].requestId, /^completion-\d+$/);
  assert.equal(got.result.items[0].label, 'Println');
  assert.equal(got.result.triggerCharacters, '.');
  await h.checks.complete('a.go', at, dot);
  assert.notEqual(asks[1].requestId, asks[0].requestId, 'each request its own ID');

  // An answer for another version or document is not this text's.
  answer = (documentId, version) => ({ documentId, version: version + 1, items: [] });
  assert.equal(await h.checks.complete('a.go', at, dot), undefined);
  answer = (_d, version) => ({ documentId: 'other', version, items: [] });
  assert.equal(await h.checks.complete('a.go', at, dot), undefined);
  // The editor text changed before asking: nothing is sent.
  const sent = asks.length;
  assert.equal(await h.checks.complete('a.go', at, dot, { current: () => false }), undefined);
  assert.equal(asks.length, sent);
  // A failing server is no suggestions, never a throw.
  h.api.completeCode = async () => { throw Object.assign(new Error('timeout'), { code: 'timeout' }); };
  assert.equal(await h.checks.complete('a.go', at, dot), undefined);
  assert.deepEqual(cancels, [], 'nothing was aborted');
  h.off();
});

test('aborting a completion cancels it on the host; the attachment ending drops its answer', async () => {
  const h = await harness(true, { completion: true });
  const cancels = [];
  let finish;
  h.api.completeCode = (c, requestId, documentId, version) => new Promise(resolve => { finish = () => resolve({ documentId, version, items: [{ label: 'x' }] }); });
  h.api.cancelCodeCompletion = async requestId => { cancels.push(requestId); throw new Error('already finished'); };
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'x\n'); await tick();
  const controller = new AbortController();
  const pending = h.checks.complete('a.go', { line: 0, character: 1 }, { kind: 'invoked' }, { signal: controller.signal });
  await tick();
  controller.abort();
  assert.equal(cancels.length, 1);
  assert.match(cancels[0], /^completion-\d+$/);
  finish();
  assert.equal(await pending, undefined, 'an aborted answer is not used');

  const aborted = new AbortController(); aborted.abort();
  assert.equal(await h.checks.complete('a.go', { line: 0, character: 1 }, { kind: 'invoked' }, { signal: aborted.signal }), undefined);

  const late = h.checks.complete('a.go', { line: 0, character: 1 }, { kind: 'invoked' });
  await tick();
  h.checks.disconnect();
  finish();
  assert.equal(await late, undefined);
  assert.ok(h.checks.completionBlocker('a.go'), 'disconnected: blocked');
  h.off();
});

test('completion details resolve once per item, at the synced version, else details only or the listed ones', async () => {
  const h = await harness(true, { completion: true });
  const resolves = [];
  const items = ['a', 'b', 'c', 'd', 'e'].map(label => ({ label, detail: 'listed', documentation: { kind: 'plaintext', value: 'old' } }));
  h.api.completeCode = async (c, requestId, documentId, version) => ({ documentId, version, resultId: 'r7', items });
  let resolve = async (c, documentId, resultId, index) => ({ documentId, resultId, index, detail: 'func a()', documentation: { kind: 'markdown', value: '**A**' }, truncated: true });
  h.api.resolveCodeCompletion = async (...args) => { resolves.push(args); return resolve(...args); };
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'a\n'); await tick();
  const got = await h.checks.complete('a.go', { line: 0, character: 1 }, { kind: 'invoked' });
  const version = got.result.version;
  assert.deepEqual(await got.details(0), { detail: 'func a()', documentation: { kind: 'markdown', value: '**A**' }, truncated: true });
  assert.deepEqual(resolves[0], [context, 'd1', 'r7', 0, version, 'h'], 'at the synced version, so accept can reuse it');
  await got.details(0);
  assert.equal(resolves.length, 1, 'one resolve per item and version');
  assert.equal(await got.details(9), undefined, 'no such item');
  // The version is refused (an older answer, a newer host state): details only.
  resolve = async (c, documentId, resultId, index, v) => {
    if (v > 0) throw Object.assign(new Error('staleVersion'), { code: 'staleVersion' });
    return { documentId, resultId, index, detail: 'details only' };
  };
  assert.equal((await got.details(1)).detail, 'details only');
  assert.deepEqual(resolves.slice(1).map(r => [r[3], r[4]]), [[1, version], [1, 0]]);
  h.api.resolveCodeCompletion = async () => { throw Object.assign(new Error('staleVersion'), { code: 'staleVersion' }); };
  assert.deepEqual(await got.details(2), { detail: 'listed', documentation: { kind: 'plaintext', value: 'old' } });
  h.api.resolveCodeCompletion = async (c, documentId, resultId) => ({ documentId, resultId, index: 1, detail: 'wrong item' });
  assert.deepEqual(await got.details(3), { detail: 'listed', documentation: { kind: 'plaintext', value: 'old' } }, 'an answer about another item is not used');
  // Typing not yet sent: the confirmed version is not the text's, so details only.
  h.files.updateDraft('a.go', 'ab\n'); await tick();
  assert.equal(h.bridge.basis('a.go').synced, false);
  const late = [];
  h.api.resolveCodeCompletion = async (c, documentId, resultId, index, v) => { late.push(v); return { documentId, resultId, index, detail: 'late' }; };
  assert.equal((await got.details(4)).detail, 'late');
  assert.deepEqual(late, [0]);
  h.off();
});

test('extraEdits sends pending typing, resolves at that version, and shares the resolve with details', async () => {
  const h = await harness(true, { completion: true });
  const resolves = [];
  const imp = { range: { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } }, newText: 'import "x";\n' };
  const listedImport = { ...imp, newText: 'import "listed";\n' };
  const items = [{ label: 'a' }, { label: 'b', extraEdits: [listedImport] }, { label: 'c' }];
  h.api.completeCode = async (c, requestId, documentId, version) => ({ documentId, version, resultId: 'r7', resolves: true, items });
  let hold;
  h.api.resolveCodeCompletion = async (c, documentId, resultId, index, version) => {
    resolves.push([index, version]);
    if (hold) await hold;
    return { documentId, resultId, index, ...(version > 0 ? { version, extraEdits: [imp] } : {}) };
  };
  h.files.openFile('a.go', { keep: true }); await tick();
  h.files.startEditing('a.go'); h.files.updateDraft('a.go', 'a\n'); await tick();
  const got = await h.checks.complete('a.go', { line: 0, character: 1 }, { kind: 'invoked' });
  assert.deepEqual(got.extraEdits(1), [], 'listed edits go in with the item, no resolve');
  assert.equal(resolves.length, 0);

  // The user typed on: the typing is sent first, then the item resolves at its version.
  h.files.updateDraft('a.go', 'ab\n');
  const pending = got.extraEdits(0, { current: () => true });
  assert.ok(pending instanceof Promise, 'not known yet');
  assert.deepEqual(await pending, [imp]);
  const version = h.bridge.basis('a.go').version;
  assert.ok(version > got.result.version);
  assert.deepEqual(resolves, [[0, version]]);
  assert.deepEqual(got.extraEdits(0, { current: () => true }), [imp], 'known for this text: at once');
  const gone = new AbortController(); gone.abort();
  assert.equal(await got.extraEdits(0, { current: () => true, signal: gone.signal }), undefined, 'known, but aborted');
  await got.details(0);
  assert.equal(resolves.length, 1, 'details reuse the resolve');

  // Selected first: the info panel's resolve is the one accept uses.
  await got.details(2);
  assert.deepEqual(got.extraEdits(2), [imp]);
  assert.equal(resolves.length, 2);

  // The editor text changed during the resolve: no edits.
  h.files.updateDraft('a.go', 'abc\n');
  let release; hold = new Promise(r => { release = r; });
  let shown = true;
  const changed = got.extraEdits(0, { current: () => shown });
  await tick(); await tick();
  shown = false; release();
  assert.equal(await changed, undefined);
  hold = undefined;
  // Aborted or failing: undefined, never a throw.
  const aborted = new AbortController(); aborted.abort();
  h.files.updateDraft('a.go', 'abcd\n');
  assert.equal(await got.extraEdits(0, { current: () => true, signal: aborted.signal }), undefined);
  h.api.resolveCodeCompletion = async () => { throw Object.assign(new Error('staleVersion'), { code: 'staleVersion' }); };
  assert.equal(await got.extraEdits(0, { current: () => true }), undefined);
  // An older host answers without a version: no edits.
  h.api.resolveCodeCompletion = async (c, documentId, resultId, index) => ({ documentId, resultId, index });
  assert.deepEqual(await got.extraEdits(0, { current: () => true }), []);

  // An answer from a server that does not resolve (or an older host, which
  // does not say): no extra edits, at once, and no resolve.
  let asked = 0;
  h.api.resolveCodeCompletion = async () => { asked++; return {}; };
  for (const resolves of [false, undefined]) {
    h.api.completeCode = async (c, requestId, documentId, version) => ({ documentId, version, resultId: 'r8', ...(resolves === undefined ? {} : { resolves }), items });
    const plain = await h.checks.complete('a.go', { line: 0, character: 1 }, { kind: 'invoked' });
    assert.deepEqual(plain.extraEdits(0, { current: () => true }), [], String(resolves));
  }
  assert.equal(asked, 0);
  h.off();
});

test('complete is blocked without the capability and for files no server declares', async () => {
  const off = await harness(true, { navigation: true });
  // complete() swallows a throw, so the calls are counted.
  let asked = 0;
  off.api.completeCode = async () => { asked++; throw new Error('must not be asked'); };
  off.files.openFile('a.go', { keep: true }); await tick();
  assert.equal(off.checks.completionBlocker('a.go'), 'Code suggestions are not available for this host.');
  assert.equal(await off.checks.complete('a.go', { line: 0, character: 0 }, { kind: 'invoked' }), undefined);
  assert.equal(asked, 0);
  off.off();
  const md = await harness(true, { completion: true, path: 'notes.md', providers: [{ ...provider('lsp'), documentExtensions: ['.go'] }] });
  md.api.completeCode = async () => { asked++; throw new Error('must not be asked'); };
  md.files.openFile('notes.md', { keep: true }); await tick();
  assert.ok(md.checks.completionBlocker('notes.md'));
  assert.equal(await md.checks.complete('notes.md', { line: 0, character: 0 }, { kind: 'invoked' }), undefined);
  assert.equal(asked, 0);
  md.off();
});

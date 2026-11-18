import test from 'node:test';
import assert from 'node:assert/strict';
import { desktopAPI, useDesktopApp } from './desktop-app.mjs';
import { codeDiagnosticsError } from '../src/lib/codeDiagnostics.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

const context = { connectionId: 'connection', attachmentId: 'attachment', generation: 'generation' };
const at = { line: 2, character: 5 };
const invoked = { kind: 'invoked' };

test('the Wails binding gets the exact arguments; an old binding is unsupported', async () => {
  const binding = {};
  useDesktopApp(binding);
  const api = desktopAPI();
  await assert.rejects(api.completeCode(context, 'q1', 'd1', 3, at, invoked, 'host'), e => e.code === 'unsupported');
  await assert.rejects(api.resolveCodeCompletion(context, 'q2', 'd1', 'r1', 0, 0, 'host'), e => e.code === 'unsupported');
  await api.cancelCodeCompletion('q1'); // an old binding has nothing to cancel

  let args;
  binding.CompleteCode = async (...a) => { args = a; return { documentId: 'd1', version: 3, resultId: 'r1', items: null }; };
  const dot = { kind: 'character', character: '.' };
  const got = await api.completeCode(context, 'q1', 'd1', 3, at, dot, 'host');
  assert.deepEqual(args, [context, 'q1', 'd1', 3, at, dot, 'host']);
  assert.deepEqual(got.items, [], 'a Go nil list is an empty list');
  binding.ResolveCodeCompletion = async (...a) => { args = a; return { documentId: 'd1', resultId: 'r1', index: 2, detail: 'func f()' }; };
  assert.equal((await api.resolveCodeCompletion(context, 'q2', 'd1', 'r1', 2, 0, 'host')).detail, 'func f()');
  assert.deepEqual(args, [context, 'q2', 'd1', 'r1', 2, 0, 'host']);
  const imports = [{ range: { start: { line: 0, character: 0 }, end: { line: 0, character: 0 } }, newText: 'import a\n' }];
  binding.ResolveCodeCompletion = async (...a) => { args = a; return { documentId: 'd1', resultId: 'r1', index: 2, version: 4, extraEdits: imports }; };
  const resolved = await api.resolveCodeCompletion(context, 'q2', 'd1', 'r1', 2, 4, 'host');
  assert.deepEqual(args, [context, 'q2', 'd1', 'r1', 2, 4, 'host'], 'the version is passed through');
  assert.deepEqual([resolved.version, resolved.extraEdits], [4, imports]);
  binding.CancelCodeCompletion = async (...a) => { args = a; };
  await api.cancelCodeCompletion('q7');
  assert.deepEqual(args, ['q7']);

  for (const code of ['canceled', 'staleVersion', 'timeout', 'notRunning']) {
    binding.CompleteCode = async () => { throw `${code}: something`; };
    await assert.rejects(api.completeCode(context, 'q1', 'd1', 3, at, invoked, 'host'), e => e.code === code);
  }
  binding.ResolveCodeCompletion = async () => { throw 'staleVersion: The completion answer is no longer kept'; };
  await assert.rejects(api.resolveCodeCompletion(context, 'q2', 'd1', 'r1', 0, 0, 'host'), e => e.code === 'staleVersion');
});

test('the mock suggests words of its open documents and resolves its last answer', async () => {
  const mock = new MockBackendAPI();
  mock.liveDocuments = true;
  const { context: c, snapshot } = await mock.attachCodeDiagnostics('/p', 1, 'mock:local');
  await mock.approveCodeDiagnostics(c, 'session', snapshot.proposal.fingerprint, 'mock:local');
  await mock.openCodeDiagnosticDocument(c, 'a.ts', 'd1', 1, 'function helper() {}\nhel\n', undefined, 'mock:local');
  await mock.openCodeDiagnosticDocument(c, 'b.ts', 'd2', 1, 'const help = 1;\nfunction helping(a, $b) {}\n', undefined, 'mock:local');
  const end = { line: 1, character: 3 };
  const got = await mock.completeCode(c, 'q1', 'd1', 1, end, invoked, 'mock:local');
  assert.deepEqual(got.items.map(i => i.label), ['help', 'helper', 'helping']);
  assert.deepEqual(got.items[0].edit, { range: { start: { line: 1, character: 0 }, end }, newText: 'help' });
  assert.equal(got.items[0].snippet, undefined);
  assert.deepEqual(got.items[1].edit, { range: { start: { line: 1, character: 0 }, end }, newText: 'helper()$0' }, 'a function: a snippet');
  assert.equal(got.items[1].snippet, true);
  assert.equal(got.items[2].edit.newText, 'helping(${1:a}, ${2:\\$b})$0', 'parameters are tab stops, escaped');
  const details = await mock.resolveCodeCompletion(c, 'q2', 'd1', got.resultId, 1, 0, 'mock:local');
  assert.equal(details.detail, 'a.ts');
  assert.match(details.documentation.value, /function helper\(\) \{\}/);
  assert.equal(details.extraEdits, undefined);
  await assert.rejects(mock.resolveCodeCompletion(c, 'q2', 'd1', got.resultId, 5, 0, 'mock:local'), e => codeDiagnosticsError(e).code === 'invalidArgument');
  // At the current version, a name declared in another document imports it.
  const top = { line: 0, character: 0 };
  const helping = await mock.resolveCodeCompletion(c, 'q2', 'd1', got.resultId, 2, 1, 'mock:local');
  assert.deepEqual([helping.version, helping.extraEdits], [1, [{ range: { start: top, end: top }, newText: "import { helping } from './b';\n" }]]);
  assert.equal((await mock.resolveCodeCompletion(c, 'q2', 'd1', got.resultId, 1, 1, 'mock:local')).extraEdits, undefined, 'declared here: no import');
  assert.equal((await mock.resolveCodeCompletion(c, 'q2', 'd1', got.resultId, 2, 0, 'mock:local')).extraEdits, undefined, 'version 0: details only');

  // A newer answer replaces it; a change keeps it, at its new version; a close ends it.
  const next = await mock.completeCode(c, 'q1', 'd1', 1, end, invoked, 'mock:local');
  await assert.rejects(mock.resolveCodeCompletion(c, 'q2', 'd1', got.resultId, 0, 0, 'mock:local'), e => codeDiagnosticsError(e).code === 'staleVersion');
  await mock.changeCodeDiagnosticDocument(c, 'd1', 2, 'function helper() {}\nhelp\n', 'mock:local');
  assert.equal((await mock.resolveCodeCompletion(c, 'q2', 'd1', next.resultId, 1, 0, 'mock:local')).detail, 'a.ts');
  await assert.rejects(mock.resolveCodeCompletion(c, 'q2', 'd1', next.resultId, 2, 1, 'mock:local'), e => codeDiagnosticsError(e).code === 'staleVersion');
  assert.equal((await mock.resolveCodeCompletion(c, 'q2', 'd1', next.resultId, 2, 2, 'mock:local')).version, 2);
  await assert.rejects(mock.completeCode(c, 'q1', 'd1', 1, end, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'staleVersion');
  // Positions as a server checks them: whole, not negative, inside the text.
  for (const bad of [{ line: 9, character: 0 }, { line: 0, character: 21 }, { line: 0, character: -1 }, { line: -1, character: 0 }, { line: 0, character: 1.5 }, { line: 0.5, character: 0 }, { line: 0, character: NaN }]) {
    await assert.rejects(mock.completeCode(c, 'q1', 'd1', 2, bad, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'invalidArgument', JSON.stringify(bad));
  }
  assert.equal((await mock.completeCode(c, 'q1', 'd1', 2, { line: 0, character: 20 }, invoked, 'mock:local')).version, 2, 'the end of a line is inside');
  await mock.closeCodeDiagnosticDocument(c, 'd1', 'mock:local');
  await assert.rejects(mock.completeCode(c, 'q1', 'd1', 2, end, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'notFound');
  await mock.cancelCodeCompletion('q1');
  assert.equal(mock.answers.size, 0, 'a close drops the answer');
  // A detach drops the answers of the documents still open.
  await mock.completeCode(c, 'q2', 'd2', 1, { line: 0, character: 4 }, invoked, 'mock:local');
  assert.equal(mock.answers.size, 1);
  await mock.detachCodeDiagnostics(c, 'mock:local');
  assert.equal(mock.answers.size, 0);
});

test('the mock without live documents is unsupported', async () => {
  const mock = new MockBackendAPI();
  const { context: c } = await mock.attachCodeDiagnostics('/p', 1, 'mock:local');
  await assert.rejects(mock.completeCode(c, 'q1', 'd1', 1, at, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'unsupported');
});

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
  await assert.rejects(api.resolveCodeCompletion(context, 'd1', 'r1', 0, 'host'), e => e.code === 'unsupported');
  await api.cancelCodeCompletion('q1'); // an old binding has nothing to cancel

  let args;
  binding.CompleteCode = async (...a) => { args = a; return { documentId: 'd1', version: 3, resultId: 'r1', items: null }; };
  const dot = { kind: 'character', character: '.' };
  const got = await api.completeCode(context, 'q1', 'd1', 3, at, dot, 'host');
  assert.deepEqual(args, [context, 'q1', 'd1', 3, at, dot, 'host']);
  assert.deepEqual(got.items, [], 'a Go nil list is an empty list');
  binding.ResolveCodeCompletion = async (...a) => { args = a; return { documentId: 'd1', resultId: 'r1', index: 2, detail: 'func f()' }; };
  assert.equal((await api.resolveCodeCompletion(context, 'd1', 'r1', 2, 'host')).detail, 'func f()');
  assert.deepEqual(args, [context, 'd1', 'r1', 2, 'host']);
  binding.CancelCodeCompletion = async (...a) => { args = a; };
  await api.cancelCodeCompletion('q7');
  assert.deepEqual(args, ['q7']);

  for (const code of ['canceled', 'staleVersion', 'timeout', 'notRunning']) {
    binding.CompleteCode = async () => { throw `${code}: something`; };
    await assert.rejects(api.completeCode(context, 'q1', 'd1', 3, at, invoked, 'host'), e => e.code === code);
  }
  binding.ResolveCodeCompletion = async () => { throw 'staleVersion: The completion answer is no longer kept'; };
  await assert.rejects(api.resolveCodeCompletion(context, 'd1', 'r1', 0, 'host'), e => e.code === 'staleVersion');
});

test('the mock suggests words of its open documents and resolves its last answer', async () => {
  const mock = new MockBackendAPI();
  mock.liveDocuments = true;
  const { context: c, snapshot } = await mock.attachCodeDiagnostics('/p', 1, 'mock:local');
  await mock.approveCodeDiagnostics(c, 'session', snapshot.proposal.fingerprint, 'mock:local');
  await mock.openCodeDiagnosticDocument(c, 'a.ts', 'd1', 1, 'function helper() {}\nhel\n', undefined, 'mock:local');
  await mock.openCodeDiagnosticDocument(c, 'b.ts', 'd2', 1, 'const help = 1;\n', undefined, 'mock:local');
  const end = { line: 1, character: 3 };
  const got = await mock.completeCode(c, 'q1', 'd1', 1, end, invoked, 'mock:local');
  assert.deepEqual(got.items.map(i => i.label), ['help', 'helper']);
  assert.deepEqual(got.items[1].edit, { range: { start: { line: 1, character: 0 }, end }, newText: 'helper' });
  const details = await mock.resolveCodeCompletion(c, 'd1', got.resultId, 1, 'mock:local');
  assert.equal(details.detail, 'a.ts');
  assert.match(details.documentation.value, /function helper\(\) \{\}/);
  await assert.rejects(mock.resolveCodeCompletion(c, 'd1', got.resultId, 5, 'mock:local'), e => codeDiagnosticsError(e).code === 'invalidArgument');

  // A newer answer replaces it; a change or close ends it.
  const next = await mock.completeCode(c, 'q1', 'd1', 1, end, invoked, 'mock:local');
  await assert.rejects(mock.resolveCodeCompletion(c, 'd1', got.resultId, 0, 'mock:local'), e => codeDiagnosticsError(e).code === 'staleVersion');
  await mock.changeCodeDiagnosticDocument(c, 'd1', 2, 'function helper() {}\nhelp\n', 'mock:local');
  await assert.rejects(mock.resolveCodeCompletion(c, 'd1', next.resultId, 0, 'mock:local'), e => codeDiagnosticsError(e).code === 'staleVersion');
  await assert.rejects(mock.completeCode(c, 'q1', 'd1', 1, end, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'staleVersion');
  await assert.rejects(mock.completeCode(c, 'q1', 'd1', 2, { line: 9, character: 0 }, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'invalidArgument');
  await mock.closeCodeDiagnosticDocument(c, 'd1', 'mock:local');
  await assert.rejects(mock.completeCode(c, 'q1', 'd1', 2, end, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'notFound');
  await mock.cancelCodeCompletion('q1');
});

test('the mock without live documents is unsupported', async () => {
  const mock = new MockBackendAPI();
  const { context: c } = await mock.attachCodeDiagnostics('/p', 1, 'mock:local');
  await assert.rejects(mock.completeCode(c, 'q1', 'd1', 1, at, invoked, 'mock:local'), e => codeDiagnosticsError(e).code === 'unsupported');
});

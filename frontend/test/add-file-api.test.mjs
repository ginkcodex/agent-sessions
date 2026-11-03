import test from 'node:test';
import assert from 'node:assert/strict';
import { withDesktopApp } from './desktop-app.mjs';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

const withApp = withDesktopApp;

test('addFile forwards the destination and captured host, preserving success and cancellation', async () => {
  const calls = [];
  const App = { async AddFile(...args) { calls.push(args); return true; } };
  await withApp(App, async (api) => {
    assert.equal(await api.addFile('/repo', 'assets', 'dev-box'), true);
    App.AddFile = async (...args) => { calls.push(args); return false; };
    assert.equal(await api.addFile('/repo', '', ''), false);
  });
  assert.deepEqual(calls, [['/repo', 'assets', 'dev-box'], ['/repo', '', '']]);
});

test('addFile normalizes native errors', async () => {
  await withApp({ AddFile: async () => { throw 'file already exists'; } }, async (api) => {
    await assert.rejects(api.addFile('/repo', 'assets', ''),
      (err) => err instanceof Error && err.message === 'file already exists');
  });
});

test('mock addFile simulates cancellation rather than reporting a copy it did not perform', async () => {
  assert.equal(await new MockBackendAPI().addFile('/repo', 'assets', ''), false);
});

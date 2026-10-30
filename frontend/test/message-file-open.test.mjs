import test from 'node:test';
import assert from 'node:assert/strict';
import { messageFileRecovery, matchingMessageFiles, messageFileProbePaths } from '../src/lib/messageFileOpen.ts';
import { createTerminalFileOpener } from '../src/lib/terminalFileOpen.ts';
import { loadServerModule } from './helpers.mjs';
const { ToastStore } = await loadServerModule(new URL('../src/lib/stores/toast.svelte.ts', import.meta.url), []);

function setup(requested = 'file.txt') {
  const toast = new ToastStore(), calls = [], errors = [];
  let valid = true, version = 0;
  const deps = {
    toast,
    async probeFiles(...args) { calls.push(['probe', ...args]); return { observations: [{ path: 'file.txt', state: 'absent' }] }; },
    async listFiles(dir) { calls.push(['list', dir]); return { files: ['b/file.txt', 'a/file.txt'], truncated: true }; },
  };
  const files = { dir: null, listError: null, async show(dir) { this.dir = dir; calls.push(['show', dir]); }, openFile(...args) { calls.push(['open', ...args]); } };
  const openerDeps = {
    link: () => ({ phase: 'local', stale: false, locked: false }), epoch: () => version,
    async resolve(dir, path) { calls.push(['resolve', dir, path]); if (path === requested) throw new Error('missing'); return { dir, path }; },
    files, report: error => errors.push(error), isStale: () => false,
  };
  const opener = createTerminalFileOpener(openerDeps);
  const open = () => opener.open('', requested, '/repo', {
    valid: () => valid,
    recover: messageFileRecovery(deps, '', '/repo', requested),
    opened: target => calls.push(['reveal', target.path, 42]),
  });
  return { deps, toast, calls, errors, opener, openerDeps, open, invalidate: () => { valid = false; }, bump: () => version++ };
}
const flush = async () => { for (let i = 0; i < 8; i++) await Promise.resolve(); };

test('exact basename matches are deduplicated, sorted and restricted to real relative file entries', () => {
  assert.deepEqual(matchingMessageFiles({ files: ['b/file.txt', 'a/file.txt', '/file.txt', '../file.txt', 'x/../file.txt', 'dir/file.txt', 'gone/file.txt', 'a/file.txt', 'x/File.txt', 'x/file.txt/'], ignored: ['ignored/file.txt', 'a/file.txt'], directories: ['dir/file.txt'], status: { 'gone/file.txt': 'D' } }, 'file.txt'), ['a/file.txt', 'b/file.txt', 'ignored/file.txt']);
  for (const path of ['', '.', '..', '../file.txt', '/file.txt', './file.txt', 'a//file.txt', 'a/../file.txt', 'a\\file.txt', 'C:file.txt', 'file.txt\n']) assert.deepEqual(messageFileProbePaths(path), [], path);
});

test('missing basename offers persistent full paths and re-resolves chosen file before kept open and reveal', async () => {
  const s = setup(), pending = s.open();
  await flush();
  const message = s.toast.current;
  assert.equal(message.persistent, true);
  assert.match(message.body, /\/repo.*truncated/);
  assert.deepEqual(message.actions.map(a => a.label), ['a/file.txt', 'b/file.txt']);
  assert.equal(s.calls.some(c => c[0] === 'show'), false);
  s.toast.invoke(message.id, 1);
  await pending;
  assert.deepEqual(s.calls.slice(-4), [['resolve', '/repo', 'b/file.txt'], ['show', '/repo'], ['open', 'b/file.txt', { keep: true }], ['reveal', 'b/file.txt', 42]]);
  assert.deepEqual(s.errors, []);
  assert.equal(s.toast.current, null);
});

test('direct success bypasses recovery; unknown and present probes preserve original failure', async () => {
  const direct = setup();
  await direct.opener.open('', 'a/file.txt', '/repo', { recover: messageFileRecovery(direct.deps, '', '/repo', 'file.txt') });
  assert.equal(direct.calls.some(c => c[0] === 'probe'), false);
  for (const state of ['unknown', 'present']) {
    const s = setup();
    s.deps.probeFiles = async () => ({ observations: [{ path: 'file.txt', state }] });
    await s.open();
    assert.deepEqual(s.errors, ['missing']);
    assert.equal(s.calls.some(c => c[0] === 'list'), false);
  }
});

test('single matches still require selection and no matches disclose listing limits', async () => {
  const s = setup();
  s.deps.listFiles = async () => ({ files: ['only/file.txt'] });
  const pending = s.open(); await flush();
  assert.equal(s.toast.current.actions.length, 1);
  assert.equal(s.calls.some(c => c[0] === 'open'), false);
  s.toast.dismiss(); await pending;
  s.deps.listFiles = async () => ({ files: [], truncated: true });
  await s.open();
  assert.match(s.toast.current.body, /No matching.*available listing.*truncated/);
  s.toast.dismiss();
});

test('dismissal, replacement, invalid ownership, epoch changes and disposal never open choices', async () => {
  for (const mode of ['dismiss', 'replace', 'invalid', 'epoch', 'dispose', 'newer']) {
    const s = setup(), pending = s.open(); await flush();
    const old = s.toast.current;
    if (mode === 'dismiss') s.toast.dismiss();
    if (mode === 'replace') s.toast.show({ title: 'New', body: 'Unrelated', persistent: true });
    if (mode === 'invalid') { s.invalidate(); s.toast.invoke(old.id, 0); }
    if (mode === 'epoch') { s.bump(); s.toast.invoke(old.id, 0); }
    if (mode === 'dispose') s.opener.dispose();
    if (mode === 'newer') await s.opener.open('', 'other.txt', '/repo');
    old.actions[0].onSelect();
    await pending;
    assert.equal(s.calls.some(c => c[0] === 'open' && c[1] === 'a/file.txt'), false, mode);
    if (mode === 'replace') assert.equal(s.toast.current.title, 'New');
    s.toast.dismiss();
  }
});

test('ownership changes during probe or listing suppress the chooser', async () => {
  for (const operation of ['probeFiles', 'listFiles']) {
    const s = setup(); let finish;
    s.deps[operation] = () => new Promise(resolve => { finish = resolve; });
    const pending = s.open(); await flush();
    s.invalidate();
    finish(operation === 'probeFiles' ? { observations: [{ path: 'file.txt', state: 'absent' }] } : { files: ['a/file.txt'] });
    await pending;
    assert.equal(s.toast.current, null);
    assert.deepEqual(s.errors, []);
  }
});

test('a failed probe, such as an unknown terminal cwd, preserves the original failure', async () => {
  const s = setup();
  s.deps.probeFiles = async () => { throw new Error('unknown directory'); };
  await s.open();
  assert.deepEqual(s.errors, ['missing']);
  assert.equal(s.calls.some(c => c[0] === 'list'), false);
  assert.equal(s.toast.current, null);
});

test('terminal recovery names its source when nothing matches', async () => {
  const s = setup();
  s.deps.source = 'terminal';
  s.deps.listFiles = async () => ({ files: [] });
  await s.open();
  assert.equal(s.toast.current.title, 'Could not find terminal file');
  s.toast.dismiss();
});

test('selection resolution failures and listing failures do not retry recovery', async () => {
  const s = setup();
  s.openerDeps.resolve = async () => { throw new Error('denied'); };
  const pending = s.open(); await flush();
  s.toast.invoke(s.toast.current.id, 0); await pending;
  assert.equal(s.calls.filter(c => c[0] === 'probe').length, 1);
  assert.deepEqual(s.errors, ['denied']);
  const t = setup();
  t.deps.listFiles = async () => { throw new Error('listing failed'); };
  await t.open();
  assert.deepEqual(t.errors, ['listing failed']);
});


test('partial paths match the complete suffix at directory boundaries', () => {
  assert.deepEqual(matchingMessageFiles({ files: ['frontend/src/main.ts', 'packages/web/src/main.ts', 'frontend/main.ts', 'frontend/not-src/main.ts', 'frontend/src/other/main.ts', 'frontend/src/Main.ts'], ignored: ['ignored/src/main.ts'] }, 'src/main.ts'), ['frontend/src/main.ts', 'ignored/src/main.ts', 'packages/web/src/main.ts']);
  assert.deepEqual(messageFileProbePaths('src/main.ts'), ['src', 'src/main.ts']);
  assert.equal(messageFileProbePaths(Array(32).fill('a').join('/')).length, 32);
  assert.deepEqual(messageFileProbePaths(Array(33).fill('a').join('/')), []);
  assert.equal(messageFileProbePaths('é'.repeat(2048)).length, 1);
  assert.deepEqual(messageFileProbePaths('é'.repeat(2049)), []);
});

test('missing parent or leaf offers full suffix choices and preserves source reveal', async () => {
  for (const absent of ['src', 'src/main.ts']) {
    const s = setup('src/main.ts');
    s.deps.probeFiles = async (cwd, paths, host) => {
      assert.deepEqual([cwd, paths, host], ['/repo', ['src', 'src/main.ts'], '']);
      return { observations: paths.map(path => ({ path, state: path === absent ? 'absent' : 'unknown' })) };
    };
    s.deps.listFiles = async () => ({ files: ['frontend/src/main.ts', 'other/main.ts'] });
    const pending = s.open(); await flush();
    assert.match(s.toast.current.title, /src\/main.ts/);
    assert.deepEqual(s.toast.current.actions.map(a => a.label), ['frontend/src/main.ts']);
    s.toast.invoke(s.toast.current.id, 0); await pending;
    assert.deepEqual(s.calls.slice(-4), [['resolve', '/repo', 'frontend/src/main.ts'], ['show', '/repo'], ['open', 'frontend/src/main.ts', { keep: true }], ['reveal', 'frontend/src/main.ts', 42]]);
  }
});

test('missing intermediate prefix is explicit evidence but unrelated or unknown observations are not', async () => {
  for (const observations of [[], [{ path: 'unrelated', state: 'absent' }], [{ path: 'src', state: 'unknown', reason: 'unsupported' }], [{ path: 'src', state: 'present' }, { path: 'src/main.ts', state: 'unknown' }]]) {
    const s = setup('src/main.ts');
    s.deps.probeFiles = async () => ({ observations });
    await s.open();
    assert.deepEqual(s.errors, ['missing']);
    assert.equal(s.calls.some(c => c[0] === 'list'), false);
  }
  const s = setup('packages/src/main.ts');
  s.deps.probeFiles = async () => ({ observations: [{ path: 'packages/src', state: 'absent' }] });
  s.deps.listFiles = async () => ({ files: ['web/packages/src/main.ts'] });
  const pending = s.open(); await flush();
  assert.equal(s.toast.current.actions[0].label, 'web/packages/src/main.ts');
  s.toast.dismiss(); await pending;
});

import test from 'node:test';
import assert from 'node:assert/strict';
import { copyToClipboard } from '../src/lib/portable.ts';
import { splitHighlightedLines, splitLines } from '../src/lib/codeHighlight.ts';
import {
  buildFileTree,
  diffRows,
  diffChangeStarts,
  stepDiffChange,
  fileChanges,
  fileLanguage,
  filePreview,
  filterFiles,
  isImageFile,
  folderStatus,
  ignoredByGit,
  inGitRepository,
  lineMarks,
  matchScore,
  parentFolders,
  searchFiles,
  withDeleted,
} from '../src/lib/files.ts';
import { fixtureStore, loadServerModule } from './helpers.mjs';

test('buildFileTree nests folders, folders before files', () => {
  const tree = buildFileTree(['main.go', 'src/b.ts', 'src/a.ts', 'src/lib/x.ts', 'README.md']);
  assert.deepEqual(tree.map((n) => n.name), ['src', 'main.go', 'README.md']);
  const src = tree[0];
  assert.deepEqual(src.children.map((n) => n.path), ['src/lib', 'src/a.ts', 'src/b.ts']);
  assert.deepEqual(src.children[0].children.map((n) => n.path), ['src/lib/x.ts']);
  assert.equal(tree[1].children, undefined);
});

test('diff change navigation groups displayed rows, including deletion-only changes', () => {
  assert.deepEqual(diffChangeStarts(diffRows([], 3)), []);
  const rows = diffRows([
    { kind: 'modified', start: 2, count: 1, removed: ['old two'] },
    { kind: 'added', start: 4, count: 1 },
    { kind: 'deleted', start: 6, count: 0, removed: ['gone'] },
    { kind: 'deleted', start: 7, count: 0, removed: ['tail'] },
  ], 7);
  // 0 line, 1 removed + 2 replacement, 3 line, 4 added, 5–6 lines, 7 removed, 8 line, 9 EOF removed.
  assert.deepEqual(diffChangeStarts(rows), [1, 4, 7, 9]);
  assert.deepEqual(diffChangeStarts(diffRows([{ kind: 'deleted', start: 0, count: 0, removed: ['a', 'b'] }], 0)), [0]);

  const starts = [1, 4, 7, 9];
  assert.equal(stepDiffChange(starts, -1, 1, 0), 0);
  assert.equal(stepDiffChange(starts, -1, 1, 5), 2);
  assert.equal(stepDiffChange(starts, -1, 1, 10), 0);
  assert.equal(stepDiffChange(starts, -1, -1, 5), 1);
  assert.equal(stepDiffChange(starts, -1, -1, 0), 3);
  assert.equal(stepDiffChange(starts, 3, 1, 0), 0);
  assert.equal(stepDiffChange(starts, 0, -1, 0), 3);
  assert.equal(stepDiffChange([], -1, 1, 0), -1);
});

test('filterFiles matches characters in order and ranks file names first', () => {
  const paths = ['src/lib/apps/x.ts', 'src/app.ts', 'docs/a-p-p.md', 'main.go'];
  assert.deepEqual(filterFiles(paths, 'app'), ['src/app.ts', 'docs/a-p-p.md', 'src/lib/apps/x.ts']);
  assert.deepEqual(filterFiles(paths, 'src app'), ['src/app.ts', 'src/lib/apps/x.ts']);
  assert.equal(matchScore('main.go', 'xyz'), -1);
  assert.deepEqual(filterFiles(paths, ''), paths);
  assert.equal(filterFiles(paths, '', 2).length, 2);
});

test('fileLanguage supports source and text files only', () => {
  assert.equal(fileLanguage('internal/app.go'), 'go');
  assert.equal(fileLanguage('src/App.svelte'), 'xml');
  assert.equal(fileLanguage('x.TSX'), 'typescript');
  assert.equal(fileLanguage('README.md'), 'markdown');
  assert.equal(fileLanguage('Makefile'), 'makefile');
  assert.equal(fileLanguage('Cargo.lock'), '');
  assert.equal(fileLanguage('.env.local'), '');
  assert.equal(fileLanguage('assets/logo.png'), null);
  assert.equal(fileLanguage('app.exe'), null);
  assert.equal(fileLanguage('noext'), null);
});

test('filePreview offers a rendered view for Markdown and SVG files', () => {
  assert.equal(filePreview('README.md'), 'markdown');
  assert.equal(filePreview('docs/guide.MDX'), 'markdown');
  assert.equal(filePreview('assets/logo.svg'), 'svg');
  assert.equal(filePreview('assets/Logo.SVG'), 'svg');
  assert.equal(filePreview('src/app.ts'), null);
  assert.equal(filePreview('assets/logo.png'), null);
});

test('isImageFile knows the image types the backend sends as data', () => {
  for (const path of ['a.png', 'b/c.JPG', 'd.jpeg', 'e.gif', 'f.webp', 'g.bmp', 'favicon.ico', 'h.avif']) {
    assert.equal(isImageFile(path), true, path);
  }
  for (const path of ['logo.svg', 'notes.md', 'png', 'archive.png.zip', 'app.exe']) {
    assert.equal(isImageFile(path), false, path);
  }
});

test('parentFolders lists the folders above a file', () => {
  assert.deepEqual(parentFolders('a/b/c.ts'), ['a', 'a/b']);
  assert.deepEqual(parentFolders('c.ts'), []);
});

test('splitHighlightedLines keeps every line complete HTML', () => {
  const html = 'a <span class="hljs-comment">/* one\ntwo <span class="hljs-doctag">@x\ny</span> three\n*/</span> b\n';
  assert.deepEqual(splitHighlightedLines(html), [
    'a <span class="hljs-comment">/* one</span>',
    '<span class="hljs-comment">two <span class="hljs-doctag">@x</span></span>',
    '<span class="hljs-comment"><span class="hljs-doctag">y</span> three</span>',
    '<span class="hljs-comment">*/</span> b',
    '',
  ]);
  assert.deepEqual(splitHighlightedLines('x\n\ny'), ['x', '', 'y']);
  assert.deepEqual(splitHighlightedLines(''), ['']);
  assert.deepEqual(splitLines('a\nb\n'), ['a', 'b', '']);
  assert.deepEqual(splitLines('a\n\n'), ['a', '', '']);
  assert.deepEqual(splitHighlightedLines('a\n\n'), ['a', '', '']);
  assert.deepEqual(splitLines(''), ['']);
});

test('copyToClipboard writes natively in the desktop app, falling back to the web clipboard', async () => {
  const written = [];
  const web = { writeText: async (t) => { written.push(['web', t]); } };
  const saved = Object.getOwnPropertyDescriptor(globalThis, 'navigator');
  Object.defineProperty(globalThis, 'navigator', { value: { clipboard: web }, configurable: true });
  try {
    globalThis.runtime = { ClipboardSetText: async (t) => { written.push(['native', t]); return true; } };
    assert.equal(await copyToClipboard('a'), true);
    globalThis.runtime = { ClipboardSetText: async () => false };
    assert.equal(await copyToClipboard('b'), true);
    delete globalThis.runtime;
    web.writeText = async () => { throw new Error('NotAllowedError'); };
    assert.equal(await copyToClipboard('c'), false);
    assert.deepEqual(written, [['native', 'a'], ['web', 'b']]);
  } finally {
    delete globalThis.runtime;
    if (saved) Object.defineProperty(globalThis, 'navigator', saved);
    else delete globalThis.navigator;
  }
});

test('git marks: deleted files listed, folders take their most important change', () => {
  const status = { 'src/a.ts': 'U', 'src/lib/b.ts': 'M', 'gone.txt': 'D', 'docs/new.md': 'A' };
  assert.deepEqual(withDeleted(['src/a.ts', 'src/lib/b.ts'], status), ['gone.txt', 'src/a.ts', 'src/lib/b.ts']);
  assert.deepEqual(withDeleted(['x'], undefined), ['x']);
  assert.deepEqual(Object.fromEntries(folderStatus(status)), { src: 'M', 'src/lib': 'M', docs: 'A' });
  assert.equal(folderStatus(undefined).size, 0);
});

test('lineMarks places gutter bars for changed lines and markers for removed ones', () => {
  const marks = lineMarks(
    [
      { kind: 'modified', start: 2, count: 1 },
      { kind: 'added', start: 4, count: 2 },
      { kind: 'deleted', start: 0, count: 0 },
      { kind: 'deleted', start: 6, count: 0 },
    ],
    6
  );
  assert.deepEqual(marks, ['deleted-above', 'modified', undefined, 'added', 'added', 'deleted-below']);
  assert.deepEqual(lineMarks(undefined, 2), [undefined, undefined]);
  assert.deepEqual(lineMarks([{ kind: 'deleted', start: 0, count: 0 }], 0), []);
});

test('diffRows puts removed lines above what replaced them', () => {
  const changes = [
    { kind: 'deleted', start: 0, count: 0, removed: ['first'] },
    { kind: 'modified', start: 2, count: 1, removed: ['old two'] },
    { kind: 'added', start: 3, count: 1 },
    { kind: 'deleted', start: 3, count: 0, removed: ['old end'] },
  ];
  const rows = diffRows(changes, 3).map((r) => (r.kind === 'removed' ? `-${r.text}` : `${r.index + 1}${r.mark ? ':' + r.mark : ''}`));
  assert.deepEqual(rows, ['-first', '1', '-old two', '2:modified', '3:added', '-old end']);
  assert.deepEqual(diffRows(undefined, 2).map((r) => r.index), [0, 1]);
});

test('fileChanges marks an untracked file as all added', () => {
  assert.deepEqual(fileChanges('U', undefined, 3), [{ kind: 'added', start: 1, count: 3 }]);
  assert.equal(fileChanges('M', undefined, 3), undefined);
  const c = [{ kind: 'added', start: 1, count: 1 }];
  assert.equal(fileChanges('M', c, 3), c);
});

test('searchFiles finds ignored files, below listed ones that match as well', () => {
  const paths = ['src/env.ts', 'README.md'];
  const ignored = ['.env', 'src/.env.local'];
  const got = searchFiles(paths, ignored, '.env');
  assert.ok(got.includes('.env') && got.includes('src/.env.local'));
  assert.ok(!got.includes('README.md'));
  assert.deepEqual(searchFiles(paths, ignored, 'env.ts'), ['src/env.ts']);
  assert.deepEqual(searchFiles(['a/x.ts'], ['b/x.ts'], 'x.ts'), ['a/x.ts', 'b/x.ts']);
  assert.deepEqual(searchFiles(paths, undefined, 'readme'), ['README.md']);
});

const FILES_STORE_URL = new URL('../src/lib/stores/files.svelte.ts', import.meta.url);

let filesStoreModule;

async function loadFilesStore() {
  const lib = (rel) => JSON.stringify(new URL(rel, FILES_STORE_URL).href);
  filesStoreModule ??= await loadServerModule(FILES_STORE_URL, [
    ["import { api } from '../api';", fixtureStore('api', '__storeFixture')],
    ["'../manage'", lib('../manage.ts')],
    ["'../link'", lib('../link.ts')],
    ["'../files'", lib('../files.ts')],
    ["'../fileEditing'", lib('../fileEditing.ts')],
    ["import { link } from './link.svelte';", "const link = { dataHost: undefined, blockedReason: null };"],
  ]);
  return filesStoreModule;
}

test('Git snapshots use their basis, keep deleted diffs readable and never become live documents or drafts', async () => {
  const { FilesStore } = await loadFilesStore();
  const calls = [], live = [];
  globalThis.__storeFixture = { api: {
    listFiles: async dir => ({ root: dir, files: ['a.ts'], status: { 'a.ts': 'D' } }),
    readFile: async (_dir, path) => ({ path, size: 4, text: 'disk' }),
    readGitDiff: async (_dir, repo, path, basis) => { calls.push(repo ? [repo, basis] : basis); return { content: { path, size: 0, text: '', changes: [{ kind: 'deleted', start: 0, count: 0, removed: ['old'] }] }, basis, deleted: true, readOnly: true }; },
  } };
  const store = new FilesStore();
  await store.show('/p');
  store.observe({ opened: () => live.push('opened'), removed: () => {}, replaced() {}, changed() {}, beforeSave() {}, saved() {}, saveFailed() {}, renamed() {}, uncertain() {}, restored() {}, cleared() {}, interrupted() {} });
  assert.equal(store.openGitDiff('a.ts', 'staged'), true);
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.deepEqual(calls, ['staged']);
  assert.equal(store.activeTab.gitDeleted, true);
  assert.equal(store.displayedDocuments().length, 0);
  assert.equal(store.startEditing('a.ts'), false);
  assert.deepEqual(live, []);
  store.openFile('a.ts');
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(store.activeTab.gitBasis, undefined);
  assert.equal(store.content.text, 'disk');
  assert.equal(store.displayedDocuments().length, 1);
  delete globalThis.__storeFixture;
});

test("a submodule's Git snapshot is marked so it offers no working file", async () => {
  const { FilesStore } = await loadFilesStore();
  globalThis.__storeFixture = { api: {
    listFiles: async dir => ({ root: dir, files: ['a.ts'], status: { ui: 'M' } }),
    readFile: async (_dir, path) => ({ path, size: 4, text: 'disk' }),
    readGitDiff: async (_dir, _repo, path, basis) => ({ content: { path, size: 24, text: 'Subproject commit new\n', changes: [{ kind: 'modified', start: 1, count: 1, removed: ['Subproject commit old'] }] }, basis, deleted: false, readOnly: true, submodule: basis === 'index' }),
  } };
  const store = new FilesStore();
  await store.show('/p');
  const settle = () => new Promise(resolve => setTimeout(resolve, 0));
  assert.equal(store.openGitDiff('ui', 'index'), true);
  await settle();
  assert.equal(store.activeTab.gitSubmodule, true);
  assert.equal(store.content.text, 'Subproject commit new\n');
  assert.equal(store.openGitDiff('a.ts', 'staged'), true);
  await settle();
  assert.equal(store.activeTab.gitSubmodule, undefined);
  delete globalThis.__storeFixture;
});

test('Git snapshots in a project that is in no repository read the repository holding the file', async () => {
  const { FilesStore } = await loadFilesStore();
  const calls = [];
  globalThis.__storeFixture = { api: {
    listFiles: async dir => ({ root: dir, files: ['lib/inner/x.ts', 'lib/y.ts'], repositories: ['lib', 'lib/inner'], status: { 'lib/inner/x.ts': 'M', 'lib/y.ts': 'M' } }),
    readFile: async (_dir, path) => ({ path, size: 4, text: 'disk' }),
    readGitDiff: async (_dir, repo, path, basis) => { calls.push([repo, path, basis]); return { content: { path, size: 0, text: '', changes: [] }, basis, deleted: false, readOnly: basis === 'staged' }; },
  } };
  const store = new FilesStore();
  await store.show('/p');
  const settle = () => new Promise(resolve => setTimeout(resolve, 0));
  // The innermost repository the listing found holds the file.
  assert.equal(store.openGitDiff('lib/inner/x.ts', 'index'), true);
  await settle();
  // Another basis keeps the tab's repository; the Changes pane names its own.
  assert.equal(store.openGitDiff('lib/inner/x.ts', 'staged'), true);
  await settle();
  assert.equal(store.openGitDiff('lib/y.ts', 'staged', 'lib'), true);
  await settle();
  assert.deepEqual(calls, [['lib/inner', 'lib/inner/x.ts', 'index'], ['lib/inner', 'lib/inner/x.ts', 'staged'], ['lib', 'lib/y.ts', 'staged']]);
  delete globalThis.__storeFixture;
});

test('FilesStore: one click previews, double click keeps a tab, folders are remembered', async () => {
  const { FilesStore } = await loadFilesStore();
  globalThis.__storeFixture = {
    api: {
      listFiles: async (dir) => ({ root: dir, files: ['a.ts', 'b.ts', 'c.ts', 'd.ts'] }),
      readFile: async (_dir, path) => ({ path, size: 1, text: path }),
    },
  };
  try {
    const store = new FilesStore();
    await store.show('/p');
    const tabs = () => store.tabs.map((t) => `${t.path}${t.preview ? '*' : ''}`);

    store.openFile('a.ts');
    store.openFile('b.ts');
    assert.deepEqual(tabs(), ['b.ts*'], 'a preview is replaced by the next file');
    store.openFile('b.ts', { keep: true });
    assert.deepEqual(tabs(), ['b.ts'], 'double click keeps the preview');
    store.openFile('c.ts');
    store.openFile('d.ts', { keep: true });
    assert.deepEqual(tabs(), ['b.ts', 'c.ts*', 'd.ts']);
    store.openFile('a.ts');
    assert.deepEqual(tabs(), ['b.ts', 'a.ts*', 'd.ts']);
    store.openFile('d.ts');
    assert.deepEqual(tabs(), ['b.ts', 'a.ts*', 'd.ts'], 'a file with a tab is just shown');
    assert.equal(store.selected, 'd.ts');
    await new Promise((r) => setTimeout(r, 0));
    assert.equal(store.content?.text, 'd.ts');

    store.closeTab('d.ts');
    assert.equal(store.selected, 'a.ts', 'closing the shown tab shows its neighbour');
    store.closeTab('b.ts');
    store.closeTab('a.ts');
    assert.equal(store.selected, null);

    store.openFile('a.ts', { keep: true });
    store.openFile('b.ts');
    store.setExpanded(new Set(['src']));
    store.close();
    assert.equal(store.expanded.size, 0);
    assert.equal(store.tabs.length, 0);
    await store.show('/p');
    assert.deepEqual([...store.expanded], ['src'], 'open folders come back with the directory');
    assert.deepEqual(tabs(), ['a.ts', 'b.ts*'], 'and so do the tabs');
    assert.equal(store.selected, 'b.ts');

    let listed = 0;
    globalThis.__storeFixture.api.listFiles = async (dir) => {
      listed++;
      return { root: dir, files: ['a.ts', 'b.ts', 'e.ts'] };
    };
    globalThis.__storeFixture.api.readFile = async (_dir, path) => ({ path, size: 2, text: path + '!' });
    await store.refresh();
    await new Promise((r) => setTimeout(r, 0));
    assert.equal(listed, 1);
    assert.deepEqual(store.listing.files, ['a.ts', 'b.ts', 'e.ts'], 'refresh lists the files again');
    assert.equal(store.content?.text, 'b.ts!', 'and reloads the open ones');
    assert.equal(store.refreshing, false);
    store.reset();
    await store.show('/p');
    assert.equal(store.expanded.size, 0, 'another host forgets them');

    store.setExpanded(new Set(['unrelated']));
    store.openFile('docs/nested/formats.md', { keep: true });
    assert.deepEqual([...store.expanded], ['unrelated', 'docs', 'docs/nested'], 'programmatic opens reveal every ancestor without collapsing other folders');
    assert.equal(store.selected, 'docs/nested/formats.md');
    store.setExpanded(new Set());
    store.openFile('docs/nested/formats.md', { keep: true });
    assert.deepEqual([...store.expanded], ['docs', 'docs/nested'], 'reopening an existing tab reveals its tree path');
    store.setExpanded(new Set());
    store.selectTab('a.ts');
    store.selectTab('docs/nested/formats.md');
    assert.deepEqual([...store.expanded], ['docs', 'docs/nested'], 'switching file tabs reveals the selected file');

    store.openFile('a.ts', { keep: true });
    assert.equal(store.selectNth(0), false, 'positions count from 1');
    assert.equal(store.selectNth(3), false, 'no tab at that position, no selection');
    assert.equal(store.selectNth(2), true, 'the second tab selects');
    assert.equal(store.selected, 'a.ts');
    assert.equal(store.selectNth(1), true, 'the first tab selects');
    assert.equal(store.selected, 'docs/nested/formats.md');
  } finally {
    delete globalThis.__storeFixture;
  }
});

test('moving a tab reorders only the shown layout and keeps a moved preview open', async () => {
  const { FilesStore, tabId } = await loadFilesStore();
  globalThis.__storeFixture = { api: {
    listFiles: async root => ({ root, files: ['a.ts', 'b.ts', 'c.ts'] }),
    readFile: async (_dir, path) => ({ path, text: path, size: 1 }),
  } };
  try {
    const store = new FilesStore();
    await store.show('/p');
    store.openFile('a.ts', { keep: true, read: false });
    store.openFile('src/a.ts', { id: 'revision:one', owner: 'history', read: false, keep: true });
    store.selectionOwner = 'working';
    store.openFile('b.ts', { keep: true, read: false });
    store.openFile('c.ts', { read: false });
    // History tabs share the list, here between working ones.
    const byId = new Map(store.tabs.map(t => [tabId(t), t]));
    store.tabs = ['a.ts', 'revision:one', 'b.ts', 'c.ts'].map(id => byId.get(id));
    assert.equal(store.activeTab.preview, true);

    assert.equal(store.moveTab('c.ts', 0), true);
    assert.deepEqual(store.tabs.map(tabId), ['c.ts', 'a.ts', 'revision:one', 'b.ts'], 'the history tab stays put');
    assert.equal(store.activeTab.preview, false, 'a moved preview is kept');
    store.openFile('docs.md', { read: false });
    assert.deepEqual(store.visibleTabs.map(tabId), ['c.ts', 'docs.md', 'a.ts', 'b.ts'], 'the next file opens beside it, not over it');

    assert.equal(store.moveTab('c.ts', 3), true);
    assert.deepEqual(store.tabs.map(tabId), ['docs.md', 'a.ts', 'revision:one', 'b.ts', 'c.ts'], 'to the end');
    assert.equal(store.moveTab('b.ts', 0), true);
    assert.deepEqual(store.tabs.map(tabId), ['b.ts', 'docs.md', 'a.ts', 'revision:one', 'c.ts']);

    assert.equal(store.moveTab('b.ts', 0), false, 'already there');
    assert.equal(store.moveTab('b.ts', -1), false, 'past the start');
    assert.equal(store.moveTab('b.ts', 4), false, 'past the end');
    assert.equal(store.moveTab('revision:one', 0), false, 'the other layout\'s tab');
    assert.equal(store.moveTab('missing', 0), false);
    const order = ['b.ts', 'docs.md', 'a.ts', 'c.ts'];
    assert.deepEqual(store.visibleTabs.map(tabId), order);

    store.close();
    await store.show('/q');
    await store.show('/p');
    assert.deepEqual(store.visibleTabs.map(tabId), order, 'the order is restored with the project');
  } finally {
    delete globalThis.__storeFixture;
  }
});

test('ignoredByGit tells files .gitignore leaves out from listed, changed and unknown ones', () => {
  const listing = { git: true, files: ['src/api.py', 'tracked.log'], ignored: ['.env'], status: { 'src/new.py': 'U' } };
  assert.equal(ignoredByGit(null, 'a.py'), false);
  assert.equal(ignoredByGit(listing, 'src/api.py'), false);
  assert.equal(ignoredByGit(listing, 'tracked.log'), false, 'a tracked file is listed even if .gitignore matches it');
  assert.equal(ignoredByGit(listing, 'src/new.py'), false, 'a changed file is not ignored');
  assert.equal(ignoredByGit(listing, '.env'), true);
  assert.equal(ignoredByGit(listing, '.venv/lib/python3.13/site-packages/fastapi/applications.py'), true, 'a file in an ignored folder');
  // Only a whole git listing tells by leaving a file out.
  assert.equal(ignoredByGit({ ...listing, truncated: true }, '.venv/lib/x.py'), false);
  assert.equal(ignoredByGit({ ...listing, truncated: true }, '.env'), true);
  assert.equal(ignoredByGit({ files: ['a.py'], repositories: ['api'] }, 'api/.venv/x.py'), false);
});

test('inGitRepository covers git listings and repositories nested below a plain directory', () => {
  assert.equal(inGitRepository(null, 'a.go'), false);
  assert.equal(inGitRepository({ git: true }, 'anything/a.go'), true);
  // Older servers send no repositories: a walked listing has no blame, as before.
  assert.equal(inGitRepository({}, 'api/main.go'), false);
  const walked = { repositories: ['api', 'apps/web'] };
  assert.equal(inGitRepository(walked, 'api/main.go'), true);
  assert.equal(inGitRepository(walked, 'apps/web/src/deep/x.ts'), true);
  assert.equal(inGitRepository(walked, 'notes.md'), false);
  assert.equal(inGitRepository(walked, 'api'), false, 'the directory itself is not a file in it');
  assert.equal(inGitRepository(walked, 'apis/x.go'), false, 'whole path segments only');
  assert.equal(inGitRepository(walked, 'apps/website/x.ts'), false);
  assert.equal(inGitRepository(walked, 'apps/x.ts'), false);
});


test('stable IDs isolate same-path revisions and layout selection across close and restore', async () => {
  const { FilesStore, tabId } = await loadFilesStore();
  const reads = [], events = [];
  globalThis.__storeFixture = { api: {
    listFiles: async root => ({ root, files: ['src/a.ts'] }),
    readFile: async (_dir, path) => { reads.push(path); return { path, text: 'disk', size: 4 }; },
    deleteFile: async (_dir, path) => { events.push(['delete', path]); },
  } };
  try {
    const store = new FilesStore();
    await store.show('/p');
    store.observe({ opened: (_dir, path) => events.push(['opened', path]), removed: (_dir, path) => events.push(['removed', path]) });
    store.openFile('src/a.ts', { keep: true });
    await new Promise(r => setImmediate(r));
    const working = store.activeTab;
    working.edit = { editing: true, baseline: 'disk', draft: 'unsaved', revision: 'r1', saving: false, version: 1 };
    for (const id of ['revision:one', 'revision:two']) {
      store.openFile('src/a.ts', { id, owner: 'history', read: false });
      store.activeTab.content = { path: 'src/a.ts', text: id, size: 12 };
      store.keepTab(id);
    }
    assert.deepEqual(store.tabs.map(tabId), ['src/a.ts', 'revision:one', 'revision:two']);
    assert.equal(store.workingSelected, 'src/a.ts');
    assert.equal(store.selectedPath, 'src/a.ts');
    assert.equal(store.selectNth(1), true);
    assert.equal(store.content.text, 'revision:one');
    assert.equal(store.closeTab('revision:one'), true);
    assert.equal(store.selected, 'revision:two');
    assert.equal(store.activeTab.preview, false);
    store.selectionOwner = 'working';
    assert.equal(store.activeTab, working);
    assert.equal(working.edit.draft, 'unsaved');
    assert.equal(store.closeTab('src/a.ts'), false);
    store.selectionOwner = 'history';
    await store.refresh();
    assert.equal(store.content.text, 'revision:two');
    assert.deepEqual(store.displayedDocuments().map(d => d.path), ['src/a.ts']);
    store.close();
    await store.show('/p');
    assert.equal(store.activeTab, working);
    store.selectionOwner = 'history';
    assert.equal(store.selected, 'revision:two');
    store.remapPaths('src', 'renamed');
    assert.equal(working.path, 'renamed/a.ts');
    assert.equal(store.workingSelected, 'renamed/a.ts');
    assert.equal(store.activeTab.path, 'src/a.ts');
    await store.delete('renamed/a.ts');
    assert.equal(store.workingSelected, null);
    assert.equal(store.selected, 'revision:two');
    assert.equal(store.closeTab('revision:two'), true);
    assert.equal(store.selected, null);
    assert.ok(reads.every(path => !path.startsWith('revision:')));
    assert.ok(events.every(([, path]) => !path.startsWith('revision:')));
    assert.equal(events.filter(([kind]) => kind === 'removed').length, 1);
  } finally { delete globalThis.__storeFixture; }
});

test('explicit live tab IDs keep read replies, observer paths and renames separate', async () => {
  const { FilesStore, tabId } = await loadFilesStore();
  const pending = [], opened = [];
  globalThis.__storeFixture = { api: {
    listFiles: async root => ({ root, files: [] }),
    readFile: (_dir, path) => new Promise(resolve => pending.push({ path, resolve })),
  } };
  try {
    const store = new FilesStore();
    await store.show('/p');
    store.observe({ opened: (_dir, path, text) => opened.push([path, text]), removed() {} });
    store.openFile('same.ts', { id: 'first', keep: true });
    store.openFile('same.ts', { id: 'second', keep: true });
    assert.equal(store.closeTab('first'), true);
    pending[0].resolve({ path: 'same.ts', text: 'late', size: 4 });
    pending[1].resolve({ path: 'same.ts', text: 'second', size: 6 });
    await new Promise(r => setImmediate(r));
    assert.equal(store.content.text, 'second');
    assert.deepEqual(opened, [['same.ts', 'second']]);
    assert.deepEqual(pending.map(p => p.path), ['same.ts', 'same.ts']);
    store.remapPaths('same.ts', 'next.ts');
    assert.equal(store.selected, 'second');
    assert.equal(store.selectedPath, 'next.ts');
    assert.equal(tabId(store.activeTab), 'second');
  } finally { delete globalThis.__storeFixture; }
});


test('immutable history reads coexist with dirty working files and reject late/foreign replies', async () => {
  const { FilesStore, historyTabId } = await loadFilesStore();
  const pending = [], calls = [], events = [];
  const source = commit => ({ dir: '/p', repo: 'nested', tip: 'tip', commit, base: 'parent', path: 'src/a.ts', oldPath: 'old.ts' });
  const reply = (commit, kind = 'text') => ({ commit, base: 'parent', change: { path: 'src/a.ts', oldPath: 'old.ts' }, kind, deleted: true,
    content: { path: 'src/a.ts', text: '', size: 0, changes: [{ kind: 'deleted', start: 0, removed: ['old'] }] } });
  globalThis.__storeFixture = { api: {
    listFiles: async root => ({ root, files: ['src/a.ts'], status: { 'src/a.ts': 'D' } }),
    readFile: async (_dir, path) => { calls.push('disk'); return { path, text: 'disk', size: 4 }; },
    readGitHistoryDiff: req => { calls.push(req); return new Promise((resolve, reject) => pending.push({ resolve, reject })); },
    saveFile: async () => { throw new Error('History must never save'); },
  } };
  try {
    const store = new FilesStore();
    await store.show('/p');
    store.openFile('src/a.ts', { keep: true });
    await new Promise(r => setImmediate(r));
    const working = store.activeTab;
    working.edit = { editing: true, baseline: 'disk', draft: 'unsaved', revision: 'r1', saving: false, version: 1 };
    store.observe({ opened: () => events.push('opened'), removed: () => events.push('removed'), replaced: () => events.push('replaced') });
    const one = source('one'), two = source('two');
    store.openHistoryDiff(one, { keep: true });
    const first = store.activeTab;
    store.openHistoryDiff(two, { keep: true });
    const second = store.activeTab;
    assert.equal(store.tabs.length, 3);
    assert.notEqual(historyTabId(one), historyTabId(two));
    assert.equal(store.workingSelected, 'src/a.ts');
    assert.equal(Object.isFrozen(first.history), true);
    assert.equal(store.startEditing(historyTabId(two)), false);
    assert.equal(await store.saveEdited(historyTabId(two)), false);
    assert.equal(await store.discardEdited(historyTabId(two)), false);
    pending[1].resolve(reply('two', 'submodule'));
    pending[0].resolve(reply('one'));
    await new Promise(r => setImmediate(r));
    assert.equal(store.activeTab, second);
    assert.equal(first.history.commit, 'one');
    assert.equal(first.content.changes[0].removed[0], 'old');
    assert.equal(second.historyKind, 'submodule');
    assert.equal(second.gitDeleted, true);
    assert.deepEqual(store.displayedDocuments().map(d => d.text), ['unsaved']);
    await store.refresh();
    assert.equal(calls.filter(c => typeof c !== 'string').length, 2, 'refresh does not read history or disk for snapshots');
    assert.deepEqual(events, []);
    assert.equal(working.edit.draft, 'unsaved');
    store.openHistoryDiff(source('late'));
    const late = store.activeTab;
    store.openHistoryDiff(source('replacement'));
    pending[2].resolve(reply('late'));
    pending[3].resolve(reply('wrong'));
    await new Promise(r => setImmediate(r));
    assert.equal(late.content, null, 'replaced preview cannot accept a late reply');
    assert.match(store.fileError, /does not match/);
    assert.equal(store.content, null);
    store.openHistoryDiff(source('disconnect'));
    const disconnected = store.activeTab;
    const reconnect = store.connectionChanged();
    pending[4].resolve(reply('disconnect'));
    await reconnect;
    assert.equal(disconnected.content, null, 'connection transition invalidates the immutable read too');
    assert.equal(disconnected.loading, false);
  } finally { delete globalThis.__storeFixture; }
});

test('mutable comparison tabs isolate drafts/history and refresh retires pending disk reads', async () => {
  const { FilesStore, comparisonTabId } = await loadFilesStore();
  const pending = [];
  globalThis.__storeFixture = { api: {
    listFiles: async root => ({ root, files: ['a.ts'] }),
    readFile: async (_dir, path) => ({ path, text: 'disk', size: 4 }),
    readGitHistoryDiff: async req => ({ commit: req.commit, base: '', change: { path: req.path }, kind: 'text', content: { path: req.path, text: 'commit', size: 6 } }),
    readGitHistoryCompareDiff: req => new Promise(resolve => pending.push({ req, resolve })),
  } };
  try {
    const store = new FilesStore(); await store.show('/p');
    store.openFile('a.ts', { keep: true }); await new Promise(r => setImmediate(r));
    const working = store.activeTab;
    working.edit = { editing: true, baseline: 'disk', draft: 'draft', revision: 'r', version: 1 };
    store.openHistoryDiff({ dir: '/p', repo: '.', tip: 'tip', commit: 'tip', path: 'a.ts' });
    const history = store.activeTab;
    const source = { dir: '/p', repo: '.', ref: 'HEAD', expectedOID: 'tip', commit: 'tip', token: 't', path: 'a.ts', label: 'HEAD → working tree' };
    store.openComparisonDiff(source); const comparison = store.activeTab;
    assert.equal(store.tabs.length, 3, 'comparison preview must not replace immutable history preview');
    assert.equal(comparison.history, undefined); assert.equal(store.startEditing(comparisonTabId(source)), false);
    store.invalidateComparisons();
    pending[0].resolve({ ...source, oid: 'tip', content: { path: 'a.ts', text: 'late', size: 4 }, kind: 'text', change: { path: 'a.ts' } });
    await new Promise(r => setImmediate(r));
    assert.equal(comparison.content, null); assert.ok(store.tabs.includes(history));
    assert.equal(working.edit.draft, 'draft');
    store.openComparisonDiff({ ...source, token: 'new' });
    const fresh = store.activeTab;
    pending[1].resolve({ ...source, token: 'new', oid: 'tip', content: { path: 'a.ts', text: 'saved', size: 5, changes: [] }, kind: 'text', change: { path: 'a.ts' } });
    await new Promise(r => setImmediate(r));
    assert.equal(fresh.content.text, 'saved'); assert.equal(fresh.historyKind, 'text');
    store.openComparisonDiff({ ...source, token: 'foreign' }); const foreign = store.activeTab;
    await store.show('/other');
    pending[2].resolve({ ...source, token: 'foreign', oid: 'tip', content: { path: 'a.ts', text: 'wrong context', size: 13 }, kind: 'text', change: { path: 'a.ts' } });
    await new Promise(r => setImmediate(r)); assert.equal(foreign.content, null);
  } finally { delete globalThis.__storeFixture; }
});

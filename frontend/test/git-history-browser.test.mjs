// History browser navigation: repository choice, refs, search and commit list.
// Server-rendered markup only; not native WebView layout, focus or paint QA.
import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerComponent, renderWithFixture, renderedButtons } from './helpers.mjs';

const base = new URL('../src/lib/components/common/go-to-file/', import.meta.url);
const store = ["import { gitHistory } from '../../../stores/gitHistory.svelte';", fixtureStore('gitHistory')];
const branches = await loadServerComponent(new URL('HistoryBranches.svelte', base), [store,
  ["import { gitHistoryActions } from '../../../stores/gitHistoryActions.svelte';", "const gitHistoryActions = { error: null, message: '', currentBranch: 'main', blocker() { return null; }, run() {} };"],
  ["import ContextMenu from '../ContextMenu.svelte';", 'const ContextMenu = () => {};'],
]);
globalThis.__historyBranches = branches.default;
const toolbar = await loadServerComponent(new URL('HistoryToolbar.svelte', base), [
  ["import { gitHistory, HISTORY_QUERY_DELAY_MS } from '../../../stores/gitHistory.svelte';", `${fixtureStore('gitHistory')} const HISTORY_QUERY_DELAY_MS = 250;`],
  ["import HistoryBranches from './HistoryBranches.svelte';", 'const HistoryBranches = globalThis.__historyBranches;'],
]);
const commits = await loadServerComponent(new URL('HistoryCommits.svelte', base), [store]);

// First opening tag of the element that contains the given attribute text.
const tag = (html, name, attr) => [...html.matchAll(new RegExp(`<${name}\\b(?:[^>"]|"[^"]*")*>`, 'g'))].map(m => m[0]).find(t => t.includes(attr)) ?? '';
const author = { name: 'Ada <Lovelace>', email: 'ada@example.test', time: 1700000000 };
const oid = (c) => c.repeat(40);
const refs = [
  { ref: 'refs/heads/main', name: 'main', remote: false, current: true, oid: oid('a') },
  { ref: 'refs/heads/feature/x', name: 'feature/x', remote: false, current: false, oid: oid('b') },
  { ref: 'refs/remotes/origin/main', name: 'origin/main', remote: true, current: false, oid: oid('a') },
  { ref: 'refs/remotes/upstream/main', name: 'upstream/main', remote: true, current: false, oid: oid('c') },
];
const state = (extra = {}) => ({ gitHistory: {
  blockedReason: null, repositories: [{ path: '.', name: 'repo', scope: '' }], selectedRepository: '.', repository: { path: '.', name: 'repo', scope: '' },
  repositoriesTruncated: false, refs, refsTruncated: false, selectedRef: 'HEAD', detached: false, unborn: false, query: '',
  commits: [], cursor: '', limited: false, shallow: false, retainedLimit: false, selectedCommit: null, canLoadMore: false,
  repositoriesLoading: false, refsLoading: false, pagesLoading: false, loading: false, requiresRepositorySelection: false,
  discoveryError: null, refsError: null, pageError: null, contextKey: 'k', ...extra,
} });

test('multiple repositories require an explicit choice before any history is shown', async () => {
  const repositories = [{ path: 'api', name: 'api', scope: '' }, { path: 'web/app', name: 'app', scope: '' }];
  const fixture = state({ repositories, selectedRepository: null, repository: null, requiresRepositorySelection: true, refs: [] });
  const bar = await renderWithFixture(toolbar, fixture);
  assert.match(bar, /<option value="" disabled=""[^>]*selected=""[^>]*>Choose a repository…<\/option>/);
  assert.match(bar, /<option value="web\/app"[^>]*>web\/app<\/option>/);
  assert.match(tag(bar, 'input', 'aria-label="Search commit message, author or OID"'), /disabled/);
  const list = await renderWithFixture(commits, fixture);
  assert.match(list, /contains 2 repositories\. Choose one/);
  assert.deepEqual(renderedButtons(list).map(b => b.text), ['api', 'web/app']);
  assert.doesNotMatch(list, /aria-label="Commits"/);
});

test('toolbar labels the root repository and scoped history and never offers write actions', async () => {
  const bar = await renderWithFixture(toolbar, state({ repository: { path: '.', name: 'repo', scope: 'src/deep' } }));
  assert.match(bar, /<option value="\."[^>]*>repo<\/option>/);
  assert.match(bar, /Scoped history: src\/deep/);
  assert.match(bar, /placeholder="Message, author or hash…" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false"/);
  assert.match(bar, /aria-label="Refresh Git history"/);
  assert.doesNotMatch(bar, /Checkout|Switch|Fetch|Pull|Push/);
});

test('branch tree groups HEAD, local and each remote; the narrow selector mirrors it', async () => {
  const tree = (await renderWithFixture(branches, state({ selectedRef: 'refs/heads/feature/x', detached: true }))).replace(/<!--.*?-->/g, '');
  assert.match(tree, /aria-label="History refs"/);
  assert.match(tree, />HEAD \(detached\)</);
  assert.match(tree, /aria-label="Filter branches" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false"/);
  // Disclosures show a label and a count of the branches listed under them.
  const summary = (label, count) => new RegExp(`<summary[^>]*>[\\s\\S]*?>${label}</span><span class="count[^"]*">${count}</span></summary>`);
  assert.match(tree, summary('Local', 2));
  assert.match(tree, summary('Remote', 2));
  assert.match(tree, />main<\/span><span class="star[^"]*" role="img" aria-label="Current branch">★<\/span><\/button>[\s\S]*>feature\/x</);
  assert.doesNotMatch(tree, /\(current\)</);
  assert.match(tree, /aria-label="Current branch"/);
  assert.equal(tree.match(/aria-label="Current branch"/g).length, 1);
  assert.match(tree, new RegExp(`${summary('origin', 1).source}[\\s\\S]*>main<[\\s\\S]*${summary('upstream', 1).source}[\\s\\S]*>main<`));
  assert.match(tag(tree, 'button', 'title="refs/heads/feature/x"'), /aria-current="true"/);
  assert.match(tag(tree, 'button', 'title="refs/heads/feature/x"'), /class="[^"]*\bselected\b/);
  assert.doesNotMatch(tag(tree, 'button', 'title="refs/heads/main (current branch)"'), /aria-current|\bselected\b/);
  const filtered = await renderWithFixture(branches, state({ refs: refs.filter(ref => ref.name.includes('feature')) }));
  assert.doesNotMatch(filtered, /<summary[^>]*>[\s\S]*?>Remote</);
  assert.match(tree, /never switches or fetches/);
  const narrow = await renderWithFixture(branches, state(), { compact: true });
  assert.match(narrow, /aria-label="History branch"/);
  assert.match(narrow, /<optgroup label="Local"/);
  assert.match(narrow, /<optgroup label="Remote · origin"[^>]*>[\s\S]*value="refs\/remotes\/origin\/main"/);
  assert.match(narrow, /<optgroup label="Remote · upstream"/);
  const toolbarHtml = await renderWithFixture(toolbar, state());
  assert.match(toolbarHtml, /class="narrow-branches[^"]*"[\s\S]*aria-label="History branch"/);
});

test('commit rows show escaped subject, author, date and short OID with Load more', async () => {
  const rows = [
    { oid: oid('a'), subject: 'fix: <script>very long subject</script>'.repeat(5), author, parents: [] },
    { oid: oid('b'), subject: '', author, parents: [] },
  ];
  const html = await renderWithFixture(commits, state({ commits: rows, selectedCommit: oid('b'), cursor: 'next', canLoadMore: true }));
  assert.match(html, /aria-label="Commits"/);
  assert.match(html, /&lt;script>very long subject&lt;\/script>/);
  assert.doesNotMatch(html, /<script>very/);
  assert.match(html, /Ada &lt;Lovelace>/);
  assert.match(html, /<time datetime="2023-11-14T22:13:20.000Z"/);
  assert.match(html, /<code[^>]*>aaaaaaaa<\/code>/);
  assert.match(html, /\(No subject\)/);
  assert.match(tag(html, 'button', `data-commit="${oid('b')}"`), /aria-current="true" tabindex="0"[\s\S]*class="[^"]*\bselected\b/);
  assert.match(tag(html, 'button', `data-commit="${oid('a')}"`), /tabindex="-1"/);
  assert.doesNotMatch(tag(html, 'button', 'class="more'), /disabled/);
  const busy = await renderWithFixture(commits, state({ commits: rows, cursor: 'next', pagesLoading: true, loading: true }));
  assert.match(busy, /Loading more commits…/);
  assert.match(tag(busy, 'button', 'class="more'), /disabled/);
});

test('commit pane reports empty, loading, unborn, error, shallow, truncation and unsupported states', async () => {
  const cases = [
    [{ repositoriesLoading: true }, /Discovering repositories…/],
    [{ repositories: [], repository: null, selectedRepository: null }, /No Git repositories found/],
    [{ refsLoading: true }, /Loading branches…/],
    [{ pagesLoading: true }, /Loading commits…/],
    [{}, /No commits in this history\./],
    [{ query: 'zzz' }, /No commits match this search\./],
    [{ unborn: true }, /no commits yet \(unborn HEAD\)/],
    [{ pageError: 'bad <cursor>' }, /role="alert"[^>]*>bad &lt;cursor></],
    [{ refsError: 'refs failed' }, /role="alert"[^>]*>refs failed</],
    [{ commits: [{ oid: oid('a'), subject: 's', author, parents: [] }], shallow: true, limited: true, retainedLimit: true, cursor: 'x', repositoriesTruncated: true, refsTruncated: true },
      /Shallow repository[\s\S]*traversal limit[\s\S]*Display limit reached[\s\S]*discovery was truncated[\s\S]*Branch listing was truncated/],
  ];
  for (const [extra, pattern] of cases) assert.match(await renderWithFixture(commits, state(extra)), pattern, JSON.stringify(extra));
  const searching = await renderWithFixture(commits, state({ query: 'zzz' }), { searching: true });
  assert.match(searching, /Loading commits…/);
  assert.doesNotMatch(searching, /No commits match/);
  const errored = await renderWithFixture(commits, state({ pageError: 'failed' }));
  assert.doesNotMatch(errored, /No commits/);
  const limited = await renderWithFixture(commits, state({ commits: [{ oid: oid('a'), subject: 's', author, parents: [] }], retainedLimit: true, cursor: 'x' }));
  assert.doesNotMatch(limited, /Load more/);
  const unsupported = await renderWithFixture(commits, state({ blockedReason: 'Update the server to browse Git history.' }));
  assert.match(unsupported, /role="status"[^>]*>Update the server to browse Git history\./);
  assert.doesNotMatch(unsupported, /aria-label="Commits"|Load more/);
  assert.match(await renderWithFixture(toolbar, state({ blockedReason: 'offline' })), /aria-label="Refresh Git history"[^>]*disabled/);
});

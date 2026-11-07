import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerComponent, loadServerModule, renderWithFixture, renderedButtons } from './helpers.mjs';
import { CODE_LANGUAGES, codeLanguageIds, languageList, languageOfPath, languageOfProvider } from '../src/lib/codeLanguages.ts';
import { liveLanguages, providerMode, providerStatus } from '../src/lib/codeCheckLabels.ts';
import { navigationBlocker } from '../src/lib/codeNavigation.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

const { PreferencesStore } = await loadServerModule(new URL('../src/lib/stores/preferences.svelte.ts', import.meta.url), [
  ["'../terminalFont'", JSON.stringify(new URL('../src/lib/terminalFont.ts', import.meta.url).href)],
  ["'../transcript'", JSON.stringify(new URL('../src/lib/transcript.ts', import.meta.url).href)],
  ["'../date'", JSON.stringify(new URL('../src/lib/date.ts', import.meta.url).href)],
]);
const { CodeDiagnosticsStore } = await loadServerModule(new URL('../src/lib/stores/codeDiagnostics.svelte.ts', import.meta.url), [
  ["import { api } from '../api';", 'const api = {};'],
  ["import { epoch } from '../link';", 'const epoch = () => 0;'],
  ["'../codeDiagnostics'", JSON.stringify(new URL('../src/lib/codeDiagnostics.ts', import.meta.url).href)],
  ["'../codeDiagnosticCounts'", JSON.stringify(new URL('../src/lib/codeDiagnosticCounts.ts', import.meta.url).href)],
  ["'../codeNavigation'", JSON.stringify(new URL('../src/lib/codeNavigation.ts', import.meta.url).href)],
]);
const panel = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/CodeChecks.svelte', import.meta.url), [
  ["import { codeDiagnostics as checks } from '../../../stores/codeDiagnostics.svelte';", fixtureStore('checks')],
]);

test('the language table matches the backend IDs and maps providers and files', () => {
  assert.deepEqual(CODE_LANGUAGES.map(l => l.id), ['go', 'typescript', 'svelte', 'python', 'rust', 'ruby']);
  assert.deepEqual(codeLanguageIds(['ruby', 'go']), ['go', 'ruby']);
  assert.equal(languageOfProvider('rubocop')?.id, 'ruby');
  assert.deepEqual(codeLanguageIds(['rust', 'nonsense', 'go', 'rust', 7]), ['go', 'rust']);
  assert.equal(languageOfProvider('go-vet-2')?.id, 'go');
  assert.equal(languageOfProvider('pyright')?.id, 'python');
  assert.equal(languageOfProvider('mock-checker'), undefined);
  for (const [path, id] of [['a/B.PY', 'python'], ['x.pyi', 'python'], ['m.mts', 'typescript'], ['App.svelte', 'svelte'], ['lib.rs', 'rust'], ['main.go', 'go'], ['app/User.RB', 'ruby'], ['lib/tasks/db.rake', 'ruby'], ['x.gemspec', 'ruby'], ['config.ru', 'ruby']]) assert.equal(languageOfPath(path)?.id, id, path);
  // Extensionless Ruby files and templates are not owned, as on the backend.
  for (const path of ['README.md', 'Gemfile', 'Rakefile', 'show.html.erb']) assert.equal(languageOfPath(path), undefined, path);
  assert.equal(languageList(['Go', 'Python', 'Rust']), 'Go, Python and Rust');
  // The shared table keeps the existing labels.
  assert.equal(providerMode({ providerId: 'go-vet-1', coverage: { kind: 'savedProject', externalChanges: 'unwatched' } }), 'saved Go files · on demand');
  assert.equal(liveLanguages([{ providerId: 'gopls', availability: 'available', coverage: { kind: 'openedDocuments' } }, { providerId: 'ruff', availability: 'available', coverage: { kind: 'openedDocuments' } }]), 'Go and Python');
});

test('disabled languages persist, default to none and survive unavailable storage', () => {
  const previous = globalThis.localStorage;
  const values = new Map();
  globalThis.localStorage = { getItem: k => values.get(k) ?? null, setItem: (k, v) => values.set(k, v) };
  try {
    const fresh = new PreferencesStore();
    fresh.init();
    assert.deepEqual(fresh.disabledLanguages, []);
    fresh.setLanguageEnabled('rust', false);
    fresh.setLanguageEnabled('python', false);
    fresh.setLanguageEnabled('python', false);
    assert.deepEqual(fresh.disabledLanguages, ['python', 'rust']);
    const restored = new PreferencesStore();
    restored.init();
    assert.deepEqual(restored.disabledLanguages, ['python', 'rust']);
    restored.setLanguageEnabled('rust', true);
    assert.deepEqual(new PreferencesStore().disabledLanguages, []);
    const again = new PreferencesStore();
    again.init();
    assert.deepEqual(again.disabledLanguages, ['python']);
    // Unknown or malformed saved values leave every language on.
    values.set('agent-sessions:disabled-code-languages', '["cobol","go"]');
    const filtered = new PreferencesStore(); filtered.init();
    assert.deepEqual(filtered.disabledLanguages, ['go']);
    values.set('agent-sessions:disabled-code-languages', '{not json');
    const broken = new PreferencesStore(); broken.init();
    assert.deepEqual(broken.disabledLanguages, []);
    globalThis.localStorage.setItem = () => { throw new Error('disabled'); };
    broken.setLanguageEnabled('svelte', false);
    assert.deepEqual(broken.disabledLanguages, ['svelte']);
  } finally { globalThis.localStorage = previous; }
});

const context = { connectionId: 'c', attachmentId: 'a', generation: 'g' };
const live = (providerId, documentExtensions, availability = 'available') => ({ providerId, availability, phase: 'idle', freshness: 'uncertain', truncated: false, documentExtensions, coverage: { kind: 'openedDocuments', roots: availability === 'disabled' ? [] : [''], partial: true, externalChanges: 'unwatched' } });
function harness(capabilities = { codeDiagnostics: true, liveCodeDiagnostics: true, codeNavigation: true, codeDiagnosticsLanguages: true }) {
  const attaches = [];
  let off = ['python'];
  const connection = { phase: 'local', generation: 0, hostId: 'host', capabilities };
  const backend = {
    onEvent() { return () => {}; },
    async connectionState() { return connection; },
    async attachCodeDiagnostics(dir, version, host, disabled) {
      attaches.push(disabled);
      const providers = [live('gopls', ['.go']), ...(disabled.includes('python') ? [live('ruff', undefined, 'disabled'), live('pyright', undefined, 'disabled')] : [live('pyright', ['.py', '.pyi'])])];
      return { project: { hostId: host, root: dir }, context, snapshot: { contractVersion: 1, context, sequence: 0, approval: { state: 'approved', mode: 'session', fingerprint: 'fp' }, providers, resultSets: [] } };
    },
    async detachCodeDiagnostics() { return { context }; },
  };
  const setting = { get off() { return off; }, set(id, on) { off = on ? off.filter(l => l !== id) : [...off, id]; } };
  const store = new CodeDiagnosticsStore(backend, () => 0, undefined, setting);
  return { store, attaches, connection, setOff: next => { off = next; } };
}

test('attach sends the languages that are off; changing one rediscovers the project', async () => {
  const h = harness();
  await h.store.attach('/p', h.connection);
  assert.deepEqual(h.attaches, [['python']]);
  assert.equal(h.store.liveSupports('main.py'), false);
  assert.equal(h.store.liveSupports('main.go'), true);
  assert.match(h.store.navigationBlocker('main.py'), /^Python is off in Settings/);
  assert.equal(h.store.navigationBlocker('main.go'), 'The language server is still opening this file.');
  assert.equal(h.store.approvalBlocker, undefined);
  assert.equal(h.store.languagesChanged, false);
  await h.store.setLanguageEnabled('python', true);
  assert.deepEqual(h.attaches, [['python'], []]);
  assert.equal(h.store.liveSupports('main.py'), true);
  assert.equal(h.store.navigationBlocker('main.py'), 'The language server is still opening this file.');
  // A change the store could not apply yet is shown until checkers are rediscovered.
  h.setOff(['rust']);
  assert.equal(h.store.languagesChanged, true);
  await h.store.rediscover();
  assert.deepEqual(h.attaches.at(-1), ['rust']);
  assert.equal(h.store.languagesChanged, false);
  await h.store.detach();
});

test('an older host that ignores the list keeps every language and says so', async () => {
  const h = harness({ codeDiagnostics: true, liveCodeDiagnostics: true, codeNavigation: true });
  await h.store.attach('/p', h.connection);
  assert.deepEqual(h.attaches, [['python']]);
  assert.deepEqual(h.store.languagesIgnored.map(l => l.name), ['Python']);
  // Nothing is held back locally: the old host decides, as before.
  assert.doesNotMatch(h.store.navigationBlocker('main.py') ?? '', /off in Settings/);
  h.setOff([]);
  await h.store.rediscover();
  assert.deepEqual(h.store.languagesIgnored, []);
  await h.store.detach();
});

test('labels: a language that is off reads "Off in Settings" and navigation says how to turn it on', () => {
  assert.deepEqual(providerStatus({ availability: 'disabled', phase: 'idle' }), { text: 'Off in Settings', tone: 'muted' });
  const reason = navigationBlocker({ status: 'ready', off: 'Rust', live: true, navigation: true, approved: true, servers: [] });
  assert.equal(reason, 'Rust is off in Settings, so code navigation is not offered for this file. Turn it on under Settings › Code checks.');
  assert.equal(navigationBlocker({ status: 'disconnected', off: 'Rust', live: true, navigation: true, approved: true, servers: [] }), 'The host is disconnected. Reconnect to use code navigation.');
});

const saved = { providerId: 'go-vet', availability: 'available', phase: 'idle', freshness: 'current', coverage: { kind: 'savedProject', roots: [''], externalChanges: 'unwatched', partial: false }, truncated: false };
function render(overrides = {}) {
  return renderWithFixture(panel, { checks: { status: 'ready', liveEnabled: true, project: { hostId: 'local:actual', root: '/p' }, snapshot: { approval: { state: 'approved', mode: 'session' } }, providers: [saved], approved: true, canRun: true, canRetry: () => true, counts: { error: 0, warning: 0 }, languagesOff: [], languagesIgnored: [], managedInstalls: {}, ...overrides } });
}

test('the panel names only the languages that are on and explains off and ignored ones', async () => {
  const all = await render();
  assert.match(all, /For Go, TypeScript\/JavaScript, Svelte, Python and Rust, a missing supported server/);
  assert.match(all, /class="mono[^"]*">gem install rubocop<\/span>/);
  assert.match(all, /npm install -g pyright/);
  assert.doesNotMatch(all, /off in Settings/);
  const off = await render({ languagesOff: ['python', 'rust'], providers: [saved, live('ruff', undefined, 'disabled'), { ...saved, providerId: 'cargo-check', availability: 'disabled', notice: 'Off in Settings', coverage: { ...saved.coverage, roots: [] } }] });
  assert.match(off, /For Go, TypeScript\/JavaScript and Svelte, a missing supported server/);
  assert.doesNotMatch(off, /Ruff plus Pyright|npm install -g pyright|uv tool install ruff/);
  assert.match(off, /Python and Rust are off in Settings and get no code checks, navigation or downloads/);
  assert.match(off, /Off in Settings<\/span>/);
  assert.match(off, /Turn this language on under Settings › Code checks/);
  // Rust guidance is for Rust projects that are checked, not for a muted row.
  assert.doesNotMatch(off, /Rust approval permits|pinned managed rust-analyzer/);
  const old = await render({ languagesIgnored: [{ id: 'python', name: 'Python' }], languagesOff: ['python'] });
  assert.match(old, /older and still checks Python, though it is off in Settings/);
  const changed = await render({ languagesChanged: true });
  assert.match(changed, /Language settings changed since these checkers were found/);
});

const rubocop = (availability, notice) => ({ ...live('rubocop', ['.rb', '.rake', '.gemspec', '.ru'], availability), ...(notice ? { notice } : {}) });
test('Ruby: RuboCop setup text and install hints, never an app download or a RuboCop navigation claim', async () => {
  const missing = await render({ providers: [saved, rubocop('missingTool', 'RuboCop is not installed for Ruby 4.0.9.')], managed: { hostId: 'local:actual', state: 'idle' }, snapshot: { approval: { state: 'required' }, proposal: { tools: [] } } });
  assert.match(missing, /Ruby linter \(RuboCop\)/);
  assert.match(missing, /RuboCop is not installed for Ruby 4\.0\.9\./);
  assert.match(missing, /class="mono[^"]*">bundle add rubocop --group development<\/span>(<!--[^>]*-->)*, (<!--[^>]*-->)*<span class="mono[^"]*">gem install rubocop<\/span>(<!--[^>]*-->)* or (<!--[^>]*-->)*<span class="mono[^"]*">gem install ruby-lsp<\/span>/);
  assert.match(missing, /RuboCop is never downloaded by this app\. It runs with the Ruby the project asks for/);
  assert.match(missing, /RuboCop plus ruby-lsp for Ruby \(lint\/syntax problems and navigation, respectively\)/);
  // Ruby is not among the languages with app downloads, and no Install button is offered.
  assert.match(missing, /For Go, TypeScript\/JavaScript, Svelte, Python and Rust, a missing supported server/);
  assert.doesNotMatch(missing, /Python, Rust and Ruby, a missing|Install rubocop/);
  assert.ok(!renderedButtons(missing).some(b => /^Install /.test(b.text)));
  // Approval names RuboCop as giving no navigation, unless Ruby is off.
  const approval = await render({ providers: [live('gopls', ['.go'], 'approvalRequired'), rubocop('approvalRequired')], approved: false, snapshot: { approval: { state: 'required' }, proposal: { fingerprint: 'fp', tools: [{ providerId: 'rubocop', executable: '/r/bin/ruby', configPaths: ['Gemfile'] }] } } });
  assert.match(approval, /Ruff alone does not provide navigation\. RuboCop alone does not provide navigation for Ruby\./);
  assert.doesNotMatch(/hover with available language servers: ([^<]*?)\. Ruff/.exec(approval)?.[1] ?? 'missing RuboCop', /RuboCop/);
  const rubyOff = await render({ languagesOff: ['ruby'], providers: [live('gopls', ['.go'], 'approvalRequired'), rubocop('disabled')], approved: false, snapshot: { approval: { state: 'required' }, proposal: { fingerprint: 'fp', tools: [] } } });
  assert.doesNotMatch(rubyOff, /RuboCop alone does not provide navigation|RuboCop is never downloaded|gem install rubocop|ruby-lsp/);
  assert.match(rubyOff, /Ruby is off in Settings/);
  // A folder without Ruby files has only the muted row and no Ruby setup text.
  const other = await render({ providers: [saved, { ...rubocop('unsupported', 'No Ruby file was found in this project.'), coverage: { ...rubocop().coverage, roots: [] } }] });
  assert.doesNotMatch(other, /RuboCop is never downloaded/);
});

const rubyLsp = (availability, notice) => ({ ...live('ruby-lsp', ['.rb', '.rake', '.gemspec', '.ru'], availability), ...(notice ? { notice } : {}) });
test('Ruby: ruby-lsp row, setup text and install hint; navigation names it, never a download', async () => {
  assert.equal(languageOfProvider('ruby-lsp')?.id, 'ruby');
  const missing = await render({ providers: [saved, rubocop('available'), rubyLsp('missingTool', 'ruby-lsp is not installed for Ruby 4.0.9. Run gem install ruby-lsp, then refresh.')], managed: { hostId: 'local:actual', state: 'idle' } });
  assert.match(missing, /<span class="name[^"]*">Ruby language server \(ruby-lsp\)<\/span> <span class="pill tone-bad[^"]*">Not installed<\/span>/);
  assert.match(missing, /ruby-lsp is not installed for Ruby 4\.0\.9\. Run gem install ruby-lsp, then refresh\./);
  assert.match(missing, /ruby-lsp, also never downloaded, answers go to definition, references and hover in opened Ruby files with the same Ruby\./);
  assert.match(missing, /without add-ons; it reads the project's Ruby files, Ruby's standard library and installed gems, evaluates the project's Gemfile, and reports no problems\. Right after it starts it may answer nothing while it indexes the project\./);
  assert.doesNotMatch(missing, /Install ruby-lsp|Ruby, a missing/);
  assert.ok(!renderedButtons(missing).some(b => /^Install /.test(b.text)));
  // The version floor and incompatible gems come as the backend's notice.
  const old = await render({ providers: [saved, rubyLsp('unsupported', 'ruby-lsp 0.25.0 is older than 0.26; run gem update ruby-lsp, then refresh.')] });
  assert.match(old, /Ruby language server \(ruby-lsp\)<\/span> <span class="pill tone-warn[^"]*">Unsupported/);
  assert.match(old, /ruby-lsp 0\.25\.0 is older than 0\.26; run gem update ruby-lsp, then refresh\./);
  // A Ruby project with only ruby-lsp registered still gets the Ruby setup text.
  assert.match(old, /ruby-lsp, also never downloaded/);
  // Approval lists ruby-lsp among the navigation servers, and its approval notice.
  const notice = "ruby-lsp reads the project's Ruby files, Ruby's standard library and the gems in Gemfile.lock to answer Go to Definition, references and hover; it reports no problems. Add-ons are not loaded. It evaluates the project's Gemfile. Ruby navigation covers opened documents only.";
  const approval = await render({ providers: [rubocop('approvalRequired'), rubyLsp('approvalRequired', notice)], approved: false, snapshot: { approval: { state: 'required' }, proposal: { fingerprint: 'fp', tools: [{ providerId: 'ruby-lsp', executable: '/r/bin/ruby', configPaths: ['Gemfile', 'Gemfile.lock'] }] } } });
  assert.match(/hover with available language servers: ([^<]*?)\. Ruff/.exec(approval)?.[1] ?? '', /and ruby-lsp for Ruby$/);
  assert.match(approval, /Ruby language server \(ruby-lsp\)/);
  assert.ok(approval.includes(notice.replaceAll("'", '&#39;')) || approval.includes(notice), 'approval notice');
});

test('the panel links to Settings for languages and downloads instead of its own switch', async () => {
  const on = await render({ autoInstall: true });
  assert.doesNotMatch(on, /type="checkbox"|Download missing language servers automatically/);
  assert.match(on, /is downloaded into this app's folder automatically, at most once per app session\./);
  assert.match(on, /Languages and automatic downloads are chosen in <button type="button" class="link[^"]*">Settings › Code checks<\/button>/);
  assert.match(await render({ autoInstall: false }), /is offered for download into this app's folder, for you to review\./);
  const { readFileSync } = await import('node:fs');
  const source = readFileSync(new URL('../src/lib/components/common/go-to-file/CodeChecks.svelte', import.meta.url), 'utf8');
  assert.match(source, /class="link" onclick=\{\(\) => manage\.openSettings\('checks'\)\}>Settings › Code checks</);
  assert.match(source, /Turn this off under Settings › Code checks\./);
});

test('the mock lists the languages an attach turned off', async () => {
  const mock = new MockBackendAPI();
  assert.equal((await mock.connectionState()).capabilities.codeDiagnosticsLanguages, true);
  const a = await mock.attachCodeDiagnostics('/p', 1, 'mock:local', ['rust']);
  assert.deepEqual(a.snapshot.providers.filter(p => p.availability === 'disabled').map(p => [p.providerId, p.notice]), [['rust-analyzer', 'Off in Settings']]);
});

test('a project whose languages are all off says so instead of "no checker found"', async () => {
  const { codeCheckSummary } = await import('../src/lib/codeCheckSummary.ts');
  const providers = [live('ruff', undefined, 'disabled'), live('pyright', undefined, 'disabled')];
  const summary = codeCheckSummary({ status: 'ready', findings: [], providers, snapshot: { approval: { state: 'required' } } });
  assert.equal(summary.text, 'Code checks: languages off');
  assert.match(summary.title, /languages are off in Settings/);
  const h = harness();
  h.setOff(['python', 'go']);
  const store = h.store;
  await store.attach('/p', h.connection);
  store.snapshot = { ...store.snapshot, approval: { state: 'required' }, providers };
  assert.equal(store.approvalBlocker, "Nothing to approve: this project's languages are off in Settings.");
  await store.detach();
});

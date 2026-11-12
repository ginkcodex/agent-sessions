// Shared by the test files: compiling real components and stores for
// server rendering with fixture stores, and small fixtures.
import assert from 'node:assert/strict';

// Store imports resolve per render to the fixture set for that render.
export function fixtureStore(name, fixture = '__componentFixture') {
  return `const ${name} = new Proxy({}, { get: (_, key) => {
    const store = globalThis.${fixture}.${name};
    if (${JSON.stringify(name)} === 'files' && !(key in store)) {
      if (key === 'selectedPath') return store.activeTab?.path ?? store.selected ?? null;
      if (key === 'selectionOwner') return 'working';
      if (key === 'visibleTabs') return store.tabs ?? [];
      if (key === 'lazyFolders') return new Map();
    }
    return store[key];
  } });`;
}

// Compiles a real component for server rendering. Only the listed imports
// are replaced: store imports with fixtures, helpers with the real modules.
export async function loadServerComponent(url, replacements) {
  const [{ readFileSync }, { fileURLToPath }, { compile }] = await Promise.all([
    import('node:fs'),
    import('node:url'),
    import('svelte/compiler'),
  ]);
  const filename = fileURLToPath(url);
  const code = compile(readFileSync(filename, 'utf8'), { generate: 'server', filename }).js.code;
  return importCompiled(filename, code, replacements);
}

// Compiles a real .svelte.ts store module for the server the same way: types
// are stripped, runes compiled, and only the listed imports replaced.
export async function loadServerModule(url, replacements) {
  const [{ readFileSync }, { fileURLToPath }, { compileModule }, { default: ts }] = await Promise.all([
    import('node:fs'),
    import('node:url'),
    import('svelte/compiler'),
    import('typescript'),
  ]);
  const filename = fileURLToPath(url);
  const js = ts.transpileModule(readFileSync(filename, 'utf8'), {
    fileName: filename,
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, useDefineForClassFields: true },
  }).outputText;
  const code = compileModule(js, { generate: 'server', filename: filename.replace(/\.ts$/, '.js') }).js.code;
  return importCompiled(filename, code, replacements);
}

let dialogCloseModule;
let iconModule;
let dropdownModule;
let monoFontModule;
let agentIconModule;
const goToFileModules = {};
const managedInstallModules = {};

async function importCompiled(filename, code, replacements) {
  replacements = [
    ["'svelte/internal/server'", JSON.stringify(import.meta.resolve('svelte/internal/server'))],
    ...replacements,
  ];
  const historyIdentityImport = "import { tabId, tabOwner } from '../../../stores/files.svelte';";
  if (code.includes(historyIdentityImport) && !replacements.some(([from]) => from === historyIdentityImport)) {
    replacements.push([historyIdentityImport, "const tabId = t => t.id ?? t.path; const tabOwner = t => t.owner ?? 'working';"]);
  }
  // The files store keeps its back/forward history in a plain module.
  const editorHistoryImport = "'../editorHistory'";
  if (code.includes(editorHistoryImport) && !replacements.some(([from]) => from === editorHistoryImport)) {
    replacements.push([editorHistoryImport, JSON.stringify(new URL('../src/lib/editorHistory.ts', import.meta.url).href)]);
  }
  const tabIdImport = "import { tabId } from '../../../stores/files.svelte';";
  if (code.includes(tabIdImport)) replacements.push([tabIdImport, 'const tabId = t => t.id ?? t.path;']);
  // Dialog fixtures render the real shared close control, not a markup stub.
  const closeButton = "import DialogCloseButton from './DialogCloseButton.svelte';";
  if (code.includes(closeButton) && !replacements.some(([from]) => from === closeButton)) {
    dialogCloseModule ??= loadServerComponent(new URL('../src/lib/components/common/DialogCloseButton.svelte', import.meta.url), []);
    globalThis.__dialogCloseButton = (await dialogCloseModule).default;
    replacements.push([closeButton, 'const DialogCloseButton = globalThis.__dialogCloseButton;']);
  }
  // Components render the real shared icon, wherever they import it from.
  const iconImport = code.match(/import Icon from '[./]*(?:common\/)?Icon\.svelte';/)?.[0];
  if (iconImport && !replacements.some(([from]) => from === iconImport)) {
    iconModule ??= loadServerComponent(new URL('../src/lib/components/common/Icon.svelte', import.meta.url), [
      ["'../../icons'", JSON.stringify(new URL('../src/lib/icons.ts', import.meta.url).href)],
      ["'../../animationVisibility'", JSON.stringify(new URL('../src/lib/animationVisibility.ts', import.meta.url).href)],
    ]);
    globalThis.__icon = (await iconModule).default;
    replacements.push([iconImport, 'const Icon = globalThis.__icon;']);
  }
  // Components render the real shared dropdown, wherever they import it from.
  const dropdownImport = code.match(/import Dropdown from '[./]*(?:common\/)?Dropdown\.svelte';/)?.[0];
  if (dropdownImport && !replacements.some(([from]) => from === dropdownImport)) {
    dropdownModule ??= loadServerComponent(new URL('../src/lib/components/common/Dropdown.svelte', import.meta.url), []);
    globalThis.__sharedDropdown = (await dropdownModule).default;
    replacements.push([dropdownImport, 'const Dropdown = globalThis.__sharedDropdown;']);
  }
  // Font settings render the real shared picker. Its installed-font list comes
  // from the fixture's `installedFonts`, or is still loading without one.
  const monoFontImport = "import MonoFontSetting from './MonoFontSetting.svelte';";
  if (code.includes(monoFontImport) && !replacements.some(([from]) => from === monoFontImport)) {
    monoFontModule ??= loadServerComponent(new URL('../src/lib/components/common/MonoFontSetting.svelte', import.meta.url), [
      ["'svelte'", JSON.stringify(import.meta.resolve('svelte'))],
      ["'../../terminalFont'", JSON.stringify(new URL('../src/lib/terminalFont.ts', import.meta.url).href)],
      ["import { installedFonts } from '../../stores/installedFonts.svelte';", "const installedFonts = new Proxy({}, { get: (_, key) => (globalThis.__componentFixture?.installedFonts ?? { fonts: [], loading: true, error: '', ensure() {}, refresh() {} })[key] });"],
    ]);
    globalThis.__monoFontSetting = (await monoFontModule).default;
    replacements.push([monoFontImport, 'const MonoFontSetting = globalThis.__monoFontSetting;']);
  }
  // Go to File renders the real problem ruler and the unplaced-problems line.
  for (const [name, file] of [['OverviewRuler', 'OverviewRuler'], ['UnplacedDiagnostics', 'UnplacedDiagnostics']]) {
    const statement = `import ${name} from './${file}.svelte';`;
    if (!code.includes(statement) || replacements.some(([from]) => from === statement)) continue;
    goToFileModules[name] ??= loadServerComponent(new URL(`../src/lib/components/common/go-to-file/${file}.svelte`, import.meta.url), []);
    globalThis[`__${name}`] = (await goToFileModules[name]).default;
    replacements.push([statement, `const ${name} = globalThis.__${name};`]);
  }
  const agentIconImport = code.match(/import AgentIcon from '[./]*(?:common\/)?AgentIcon\.svelte';/)?.[0];
  if (agentIconImport && !replacements.some(([from]) => from === agentIconImport)) {
    agentIconModule ??= loadServerComponent(new URL('../src/lib/components/common/AgentIcon.svelte', import.meta.url), []);
    globalThis.__agentIcon = (await agentIconModule).default;
    replacements.push([agentIconImport, 'const AgentIcon = globalThis.__agentIcon;']);
  }
  // Code checks and the chat setup render the real shared install cards.
  for (const name of ['ManagedInstallReview', 'ManagedInstallProgress']) {
    const statement = code.match(new RegExp(`import ${name} from '[./]*(?:common/)?${name}\\.svelte';`))?.[0];
    if (!statement || replacements.some(([from]) => from === statement)) continue;
    managedInstallModules[name] ??= loadServerComponent(new URL(`../src/lib/components/common/${name}.svelte`, import.meta.url), []);
    globalThis[`__${name}`] = (await managedInstallModules[name]).default;
    replacements.push([statement, `const ${name} = globalThis.__${name};`]);
  }
  // Pure display-label helpers shared by the diagnostics components resolve to the real module.
  for (const [module, file] of [["'../../../codeCheckLabels'", 'codeCheckLabels'], ["'../../../problemOverview'", 'problemOverview'], ["'../../../managedTools'", 'managedTools'], ["'../managedTools'", 'managedTools'], ["'../codeLanguages'", 'codeLanguages'], ["'../../../codeLanguages'", 'codeLanguages'], ["'../../../date'", 'date'], ["'../appearance'", 'appearance']]) {
    if (code.includes(module) && !replacements.some(([from]) => from === module)) {
      replacements.push([module, JSON.stringify(new URL(`../src/lib/${file}.ts`, import.meta.url).href)]);
    }
  }
  const agentSettings = "import { agentSettings } from '../../stores/agentSettings.svelte';";
  if (code.includes(agentSettings) && !replacements.some(([from]) => from === agentSettings)) {
    replacements.push([agentSettings, `const agentSettings = { get activeAgents() { return globalThis.__componentFixture?.agentSettings?.activeAgents ?? [{id:'claude-code',label:'Claude Code'},{id:'codex',label:'Codex'},{id:'opencode',label:'OpenCode'}]; } };`]);
  }
  // The browser-only splitter renders as a bare separator.
  const splitter = "import Splitter from '../Splitter.svelte';";
  if (code.includes(splitter) && !replacements.some(([from]) => from === splitter)) {
    replacements.push([splitter, `const Splitter = ($$renderer, $$props) => $$renderer.push(\`<div role="separator" aria-label="\${$$props.ariaLabel}"></div>\`);`]);
  }
  // The diagnostics store wires its singleton to the saved setting; store tests construct their own.
  const preferences = "import { preferences } from './preferences.svelte';";
  if (code.includes(preferences) && !replacements.some(([from]) => from === preferences)) {
    replacements.push([preferences, 'const preferences = { autoInstallLanguageServers: false, setAutoInstallLanguageServers() {}, disabledLanguages: [], setLanguageEnabled() {} };']);
  }
  // The code checks panel only opens Settings; a render may record the section.
  const manage = "import { manage } from '../../../stores/manage.svelte';";
  if (code.includes(manage) && !replacements.some(([from]) => from === manage)) {
    replacements.push([manage, 'const manage = { openSettings: section => globalThis.__componentFixture?.manage?.openSettings?.(section) };']);
  }
  // Go to File reads only the time zone, for blame times.
  const viewerPreferences = "import { preferences } from '../../../stores/preferences.svelte';";
  if (code.includes(viewerPreferences) && !replacements.some(([from]) => from === viewerPreferences)) {
    replacements.push([viewerPreferences, "const preferences = { timeZone: 'utc' };"]);
  }
  // Go to File's content search: a fixture store (Files mode unless the
  // render says otherwise), a stub pane, and the real pure helpers.
  for (const store of ["import { contentSearch } from '../../../stores/contentSearch.svelte';", "import { contentSearch } from '../../stores/contentSearch.svelte';"]) {
    if (code.includes(store) && !replacements.some(([from]) => from === store)) {
      replacements.push([store, "const contentSearch = new Proxy({}, { get: (_, key) => (globalThis.__componentFixture?.contentSearch ?? { mode: 'files' })[key] });"]);
    }
  }
  for (const store of ["import { gitChanges } from '../../../stores/gitChanges.svelte';", "import { gitChanges } from '../../stores/gitChanges.svelte';"]) {
    if (code.includes(store) && !replacements.some(([from]) => from === store)) {
      replacements.push([store, "const gitChanges = new Proxy({}, { get: (_, key) => (globalThis.__componentFixture?.gitChanges ?? { status: null, refresh() {} })[key] });"]);
    }
  }
  for (const file of ['gitChanges', 'gitHistory']) {
    for (const module of [`'../../${file}'`, `'../../../${file}'`]) {
      if (code.includes(module) && !replacements.some(([from]) => from === module)) replacements.push([module, JSON.stringify(new URL(`../src/lib/${file}.ts`, import.meta.url).href)]);
    }
  }
  const changesPane = "import ChangesPane from './ChangesPane.svelte';";
  if (code.includes(changesPane) && !replacements.some(([from]) => from === changesPane)) replacements.push([changesPane, "const ChangesPane = ($$renderer) => $$renderer.push('<div data-changes-pane></div>');"]);
  const searchPane = "import SearchPane from './SearchPane.svelte';";
  if (code.includes(searchPane) && !replacements.some(([from]) => from === searchPane)) {
    replacements.push([searchPane, "const SearchPane = ($$renderer) => $$renderer.push('<div data-search-pane></div>');"]);
  }
  for (const module of ["'../../contentSearch'", "'../../../contentSearch'"]) {
    if (code.includes(module) && !replacements.some(([from]) => from === module)) {
      replacements.push([module, JSON.stringify(new URL('../src/lib/contentSearch.ts', import.meta.url).href)]);
    }
  }
  for (const module of ["'../../referenceResults'", "'../../../referenceResults'"]) {
    if (code.includes(module) && !replacements.some(([from]) => from === module)) {
      replacements.push([module, JSON.stringify(new URL('../src/lib/referenceResults.ts', import.meta.url).href)]);
    }
  }
  for (const [from, to] of replacements) {
    assert.ok(code.includes(from), `compiled ${filename} no longer contains ${from}`);
    code = code.replace(from, to);
  }
  return import(`data:text/javascript,${encodeURIComponent(code)}`);
}

export async function renderWithFixture(module, fixture, props = {}) {
  const { render } = await import('svelte/server');
  globalThis.__componentFixture = fixture;
  try {
    return render(module.default, { props }).body;
  } finally {
    delete globalThis.__componentFixture;
  }
}

export function renderedButtons(html) {
  return [...html.matchAll(/<button\b([^>]*)>([\s\S]*?)<\/button>/g)].map(([, attrs, text]) => ({
    text: text.replace(/<!--[\s\S]*?-->/g, '').trim(),
    disabled: /\sdisabled(?:=|\s|$)/.test(attrs),
  }));
}

const RECONNECT_BANNER_URL = new URL('../src/lib/components/common/ReconnectBanner.svelte', import.meta.url);

let reconnectBannerModule;

export async function renderReconnectBanner(connectionStore) {
  reconnectBannerModule ??= await loadServerComponent(RECONNECT_BANNER_URL, [
    ["'../../guidance'", JSON.stringify(new URL('../../guidance.ts', RECONNECT_BANNER_URL).href)],
    ["import { connectionStore } from '../../stores/connection.svelte';", fixtureStore('connectionStore')],
  ]);
  return renderWithFixture(reconnectBannerModule, {
    connectionStore: { generation: 1, connect() {}, disconnect() {}, ...connectionStore },
  });
}

export function unescapeHTML(text) {
  return text.replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'").replace(/&amp;/g, '&');
}

export const WINDOWS_DIRS = [
  'C:\\Users\\me\\repo',
  'C:\\',
  '\\\\fileserver\\share\\team-repo\\',
  '\\\\?\\D:\\Workspaces\\x',
  'C:/Users/me\\mixed/repo',
];

const SESSION_HEADER_URL = new URL('../src/lib/components/transcript/SessionHeader.svelte', import.meta.url);

let sessionHeaderModule;

export async function renderSessionHeader(fixture, props = {}) {
  const lib = (rel) => JSON.stringify(new URL(rel, SESSION_HEADER_URL).href);
  sessionHeaderModule ??= await loadServerComponent(SESSION_HEADER_URL, [
    ["'../../format'", lib('../../format.ts')],
    ["'../../date'", lib('../../date.ts')],
    ["'../../portable'", lib('../../portable.ts')],
    ["'../../link'", lib('../../link.ts')],
    ["'../../transcript'", lib('../../transcript.ts')],
    ["'../../guidance'", lib('../../guidance.ts')],
    ["'../../terminal'", lib('../../terminal.ts')],
    ["import { newSession } from '../../stores/newSession.svelte';", fixtureStore('newSession')],
    ["import { terminals } from '../../stores/terminals.svelte';", fixtureStore('terminals')],
    ["import { toast } from '../../stores/toast.svelte';", fixtureStore('toast')],
    ["import AgentIcon from '../common/AgentIcon.svelte';", 'const AgentIcon = () => {};'],
    ["import NavButtons from './NavButtons.svelte';", 'const NavButtons = () => {};'],
    ["import CodexRepairNotice from './CodexRepairNotice.svelte';", 'const CodexRepairNotice = () => {};'],
    ["import { appState } from '../../stores/appState.svelte';", fixtureStore('appState')],
    ["import { handoff } from '../../stores/handoff.svelte';", fixtureStore('handoff')],
    ["import { exporter } from '../../stores/export.svelte';", fixtureStore('exporter')],
    ["import { link } from '../../stores/link.svelte';", fixtureStore('link')],
    ["import { launcher } from '../../stores/launcher.svelte';", fixtureStore('launcher')],
    ["import { preferences } from '../../stores/preferences.svelte';", fixtureStore('preferences')],
    ["import { chats } from '../../stores/chats.svelte';", fixtureStore('chats')],
    ["import { chatOpen } from '../../stores/chatOpen.svelte';", fixtureStore('chatOpen')],
    ["import { manage } from '../../stores/manage.svelte';", fixtureStore('manage')],
  ]);
  const meta = {
    ref: { agent: 'codex', id: 's1' },
    title: 'Fix the build',
    cwd: 'C:\\work\\proj',
    counts: { user: 1, assistant: 1 },
    tokens: { input: 10, output: 5 },
    createdAt: '2026-10-01T10:00:00Z',
    updatedAt: '2026-10-01T11:00:00Z',
  };
  return renderWithFixture(
    sessionHeaderModule,
    { appState: {}, manage: { settings: { enabled: false } }, handoff: {}, exporter: {}, toast: {}, preferences: { timeZone: 'utc' }, newSession: { available: false, choices: [], starting: false, refresh() {} }, terminals: { tabFor: () => undefined }, chats: { forSession: () => undefined }, chatOpen: { opening: false, refresh() {}, canChat: () => false }, ...fixture },
    { meta, mode: 'activity', onModeChange() {}, onResume() {}, ...props },
  );
}

const HANDOFF_DIALOG_URL = new URL('../src/lib/components/common/HandoffDialog.svelte', import.meta.url);

let handoffDialogModule;

export async function renderHandoffDialog(fixture) {
  const lib = (rel) => JSON.stringify(new URL(rel, HANDOFF_DIALOG_URL).href);
  globalThis.__remoteCommandNote ??= (await loadRemoteCommandNote()).default;
  handoffDialogModule ??= await loadServerComponent(HANDOFF_DIALOG_URL, [
    ["'../../portable'", lib('../../portable.ts')],
    ["'../../format'", lib('../../format.ts')],
    ["import AgentIcon from './AgentIcon.svelte';", 'const AgentIcon = () => {};'],
    ["import RemoteCommandNote from './RemoteCommandNote.svelte';", 'const RemoteCommandNote = globalThis.__remoteCommandNote;'],
    ["import MarkdownDoc from './MarkdownDoc.svelte';", 'const MarkdownDoc = () => {};'],
    ["import { handoff } from '../../stores/handoff.svelte';", fixtureStore('handoff')],
    ["import { link } from '../../stores/link.svelte';", fixtureStore('link')],
    ["import { launcher } from '../../stores/launcher.svelte';", fixtureStore('launcher')],
  ]);
  const handoff = {
    dialogOpen: true,
    session: { ref: { agent: 'claude-code', id: 's1' }, cwd: 'C:\\work' },
    target: 'codex',
    budget: 80000,
    cwd: 'C:\\work',
    launching: false,
    preview: {
      command: "Set-Location -LiteralPath C:\\work -ErrorAction Stop; codex 'Read it'",
      promptMarkdown: 'Read it',
      fullMarkdown: '# Handoff',
      promptBytes: 7,
      report: { estimatedTokens: 100, trimmed: false, droppedItems: [] },
    },
    ...fixture.handoff,
  };
  return renderWithFixture(handoffDialogModule, { link: { dataHost: undefined }, ...fixture, handoff });
}

const REMOTE_COMMAND_NOTE_URL = new URL('../src/lib/components/common/RemoteCommandNote.svelte', import.meta.url);

let remoteCommandNoteModule;

export async function loadRemoteCommandNote() {
  remoteCommandNoteModule ??= await loadServerComponent(REMOTE_COMMAND_NOTE_URL, [
    ["'../../hosts'", JSON.stringify(new URL('../../hosts.ts', REMOTE_COMMAND_NOTE_URL).href)],
  ]);
  return remoteCommandNoteModule;
}

let appearanceSettingsModule;
let sizeStepperModule;
let terminalFontSettingModule;

const commonComponents = new URL('../src/lib/components/common/', import.meta.url);
const appearanceModule = JSON.stringify(new URL('../src/lib/appearance.ts', import.meta.url).href);

/** The real size stepper, as `globalThis.__sizeStepper` for the components that use it. */
async function loadSizeStepper() {
  sizeStepperModule ??= loadServerComponent(new URL('SizeStepper.svelte', commonComponents), [["'../../appearance'", appearanceModule]]);
  globalThis.__sizeStepper = (await sizeStepperModule).default;
}

/** Settings → Terminal's font and size, with the real picker and stepper; `preferences` comes from the fixture. */
export function loadTerminalFontSetting() {
  terminalFontSettingModule ??= (async () => {
    await loadSizeStepper();
    return loadServerComponent(new URL('TerminalFontSetting.svelte', commonComponents), [
      ["'../../appearance'", appearanceModule],
      ["import { preferences } from '../../stores/preferences.svelte';", fixtureStore('preferences')],
      ["import SizeStepper from './SizeStepper.svelte';", 'const SizeStepper = globalThis.__sizeStepper;'],
    ]);
  })();
  return terminalFontSettingModule;
}

/** Settings → Appearance with the real stepper, sample and font picker; `preferences` and `installedFonts` come from the fixture. */
export function loadAppearanceSettings() {
  const common = commonComponents;
  const appearance = appearanceModule;
  appearanceSettingsModule ??= (async () => {
    await loadSizeStepper();
    globalThis.__typeSample = (await loadServerComponent(new URL('TypeSample.svelte', common), [])).default;
    return loadServerComponent(new URL('AppearanceSettings.svelte', common), [
      ["'../../appearance'", appearance],
      ["import { preferences } from '../../stores/preferences.svelte';", fixtureStore('preferences')],
      ["import SizeStepper from './SizeStepper.svelte';", 'const SizeStepper = globalThis.__sizeStepper;'],
      ["import TypeSample from './TypeSample.svelte';", 'const TypeSample = globalThis.__typeSample;'],
    ]);
  })();
  return appearanceSettingsModule;
}

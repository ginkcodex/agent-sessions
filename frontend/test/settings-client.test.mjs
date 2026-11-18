// The Settings dialog mounted in a DOM, for what its markup alone cannot
// show: changing a control calls the store. Its own file, as the DOM globals
// it installs would change the server-rendered tests in settings.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import { compile } from 'svelte/compiler';
import { fixtureStore } from './helpers.mjs';
import { SUGGEST_KEY, SUGGEST_KEY_MAC, macKeys, suggestShortcutLabel } from '../src/lib/editorKeys.ts';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });

const client = (path) => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);

const DIALOG_URL = new URL('../src/lib/components/common/ManageSettingsDialog.svelte', import.meta.url);

async function loadDialog() {
  const filename = fileURLToPath(DIALOG_URL);
  const lib = (rel) => JSON.stringify(new URL(rel, DIALOG_URL).href);
  const stub = (name) => [`import ${name} from './${name}.svelte';`, `const ${name} = () => {};`];
  const replacements = [
    ["'svelte/internal/disclose-version'", client('internal/disclose-version.js')],
    ["'svelte/internal/client'", client('internal/client/index.js')],
    ...['Icon', 'DialogCloseButton', 'TerminalFontSetting', 'CodingAgentsSettings', 'AppearanceSettings', 'Dropdown'].map(stub),
    ...['format', 'fullscreen', 'terminal', 'update', 'codeLanguages', 'transcript', 'date', 'editorKeys'].map(m => [`'../../${m}'`, lib(`../../${m}.ts`)]),
    ...['manage', 'launcher', 'terminals', 'alerts', 'preferences', 'updates', 'codeDiagnostics'].map(s => [`import { ${s} } from '../../stores/${s}.svelte';`, fixtureStore(s)]),
    ["import { translateSettings } from '../../stores/translate.svelte';", fixtureStore('translateSettings')],
    ["import { installedFonts } from '../../stores/installedFonts.svelte';", 'const installedFonts = { markStale() {} };'],
  ];
  let code = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).js.code;
  for (const [from, to] of replacements) {
    assert.ok(code.includes(from), `the dialog still imports ${from}`);
    code = code.replace(from, to);
  }
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}

const Dialog = await loadDialog();

test('Settings › General: the suggestion checkbox turns suggesting while typing off and on', () => {
  const calls = [];
  const preferences = {
    transcriptMode: 'activity', timeZone: 'utc', disabledLanguages: [], autoInstallLanguageServers: true, suggestWhileTyping: true,
    setSuggestWhileTyping(on) { calls.push(on); this.suggestWhileTyping = on; },
  };
  globalThis.__componentFixture = {
    manage: {
      settingsDialogOpen: true, settingsSection: 'general', settings: { enabled: false, allowPermanentDelete: false }, loadingSettings: false,
      settingsError: null, firstEnableWarningVisible: false, handoffCache: null, handoffCacheError: null, handoffCacheBusy: false,
      codeCheckLogs: null, codeCheckLogsError: null, codeCheckLogsBusy: false,
    },
    launcher: { terminalError: null, terminal: null, info: { terminal: true, chooseTerminal: false, shell: 'powershell' } },
    terminals: { position: 'bottom', autoActivate: true },
    alerts: { notifyNeeds: true, notifyDone: true, status: null, refreshStatus: async () => {} },
    translateSettings: {
      settings: { baseURL: '', model: '', language: 'Vietnamese', apiKeySet: false, configured: false },
      configured: false, error: null, saving: false, testing: false, testResult: null, testError: null,
    },
    preferences,
    updates: { available: false, status: { state: 'idle' }, auto: true, checkFailed: false, canCheck: false, checking: false, checkResult: null, saving: false, restarting: false, error: null },
    codeDiagnostics: {},
  };
  const target = document.createElement('div');
  document.body.appendChild(target);
  let component;
  try {
    component = mount(Dialog, { target });
    flushSync();
    const label = [...target.querySelectorAll('label')].find(l => /Suggest code while typing/.test(l.textContent));
    assert.ok(label, 'the setting is shown');
    const checkbox = label.querySelector('input[type="checkbox"]');
    assert.equal(checkbox.checked, true);
    checkbox.click();
    flushSync();
    assert.deepEqual(calls, [false], 'unchecking turns it off');
    checkbox.click();
    flushSync();
    assert.deepEqual(calls, [false, true], 'checking turns it back on');
    assert.ok(target.textContent.includes(`Off, ${suggestShortcutLabel()} still shows them.`), target.textContent);
  } finally {
    if (component) unmount(component);
    target.remove();
    delete globalThis.__componentFixture;
  }
});

test('Settings names the key the editor binds for suggestions', () => {
  // CodeMirror binds `mac` keys where navigator.platform says Mac, and `key` elsewhere.
  for (const [platform, mac] of [['MacIntel', true], ['Macintosh', true], ['Win32', false], ['Linux x86_64', false], ['', false]]) {
    assert.equal(macKeys(platform), mac, platform);
  }
  assert.equal(suggestShortcutLabel('MacIntel'), '⌘I');
  assert.equal(suggestShortcutLabel('Win32'), 'Ctrl+Space');
  assert.equal(suggestShortcutLabel('Linux x86_64'), 'Ctrl+Space');
  // The label spells the key the editor's keymap binds.
  const spelled = (key) => key.replace(/^Cmd-/, '⌘').replace(/-/g, '+').replace(/(?<=[⌘+])[a-z]$/, c => c.toUpperCase());
  assert.equal(spelled(SUGGEST_KEY_MAC), suggestShortcutLabel('MacIntel'));
  assert.equal(spelled(SUGGEST_KEY), suggestShortcutLabel('Win32'));
});

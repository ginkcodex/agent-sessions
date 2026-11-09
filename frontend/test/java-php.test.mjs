import test from 'node:test';
import assert from 'node:assert/strict';
import { fixtureStore, loadServerComponent, renderWithFixture, renderedButtons } from './helpers.mjs';
import { languageOfProvider } from '../src/lib/codeLanguages.ts';
import { toolName } from '../src/lib/codeCheckLabels.ts';

const panel = await loadServerComponent(new URL('../src/lib/components/common/go-to-file/CodeChecks.svelte', import.meta.url), [
  ["import { codeDiagnostics as checks } from '../../../stores/codeDiagnostics.svelte';", fixtureStore('checks')],
]);
const live = (providerId, ext) => ({ providerId, availability: 'missingTool', phase: 'idle', freshness: 'uncertain', documentExtensions: [ext], coverage: { kind: 'openedDocuments', roots: [''], externalChanges: 'unwatched', partial: true } });
function render(off = [], approval = false) {
  return renderWithFixture(panel, { checks: { status: 'ready', liveEnabled: true, project: { hostId: 'ssh:test', root: '/p' }, snapshot: { approval: { state: approval ? 'required' : 'approved', mode: 'session' }, proposal: { tools: [] } }, providers: [live('jdtls', '.java'), live('intelephense', '.php')], approved: !approval, counts: {}, languagesOff: off, languagesIgnored: [], managedInstalls: {} } });
}
test('Java/PHP map navigation and checks to their language and server', () => {
  assert.equal(languageOfProvider('jdtls')?.id, 'java');
  assert.equal(languageOfProvider('intelephense')?.id, 'php');
  assert.equal(toolName('jdtls'), 'Java language server (JDT LS)');
  assert.equal(toolName('intelephense'), 'PHP language server (Intelephense)');
});
test('Java/PHP setup is manual, host-local and honest about coverage and execution', async () => {
  const html = await render();
  assert.match(html, /JDK 25 or newer on the remote host/);
  assert.match(html, /npm install -g intelephense/);
  assert.match(html, /server is proprietary/);
  assert.match(html, /definition, references, hover and opened-file diagnostics are free/);
  assert.match(html, /Maven\/Gradle import can execute project build code and download dependencies/);
  assert.match(html, /not Java archive\/decompiled targets/);
  assert.match(html, /not a whole-project Check Project build/);
  assert.ok(!renderedButtons(html).some(b => /^Install (jdtls|intelephense)/.test(b.text)));
  const off = await render(['java', 'php']);
  assert.doesNotMatch(off, /Java uses a user-installed|PHP uses user-installed|npm install -g intelephense|JDK 25 or newer/);
  assert.match(off, /Java and PHP are off in Settings/);
});

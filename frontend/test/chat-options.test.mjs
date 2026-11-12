// Session option pickers and the choices remembered per agent and host.
import test from 'node:test';
import assert from 'node:assert/strict';
import { choicesToApply, loadChoices, pickers, pickerOptions, saveChoice, selectValues, skipsPermissions } from '../src/lib/chatOptions.ts';

// The options the ACP-02 spike saw, trimmed.
const CLAUDE = [
  { id: 'mode', name: 'Mode', category: 'mode', type: 'select', currentValue: 'default', options: [
    { value: 'default', name: 'Default' }, { value: 'acceptEdits', name: 'Accept Edits' }, { value: 'plan', name: 'Plan' }, { value: 'bypassPermissions', name: 'Bypass Permissions' },
  ] },
  { id: 'model', name: 'Model', category: 'model', type: 'select', currentValue: 'default', options: [{ value: 'default', name: 'Default' }, { value: 'fable', name: 'Fable' }] },
  { id: 'effort', name: 'Effort', type: 'select', currentValue: 'high', options: [{ value: 'low', name: 'Low' }, { value: 'high', name: 'High' }] },
  { id: 'ultracode', name: 'Ultracode', type: 'boolean', currentValue: false },
];
const CODEX_MODE = { id: 'mode', name: 'Approval', category: 'mode', type: 'select', currentValue: 'agent', options: [
  { value: 'read-only', name: 'Read Only' }, { value: 'agent', name: 'Agent' }, { value: 'agent-full-access', name: 'Agent (full access)' },
] };
const OPENCODE_MODEL = { id: 'model', name: 'Model', type: 'select', currentValue: 'anthropic/claude-fable-5-2', options: [
  { group: 'anthropic', name: 'Anthropic', options: [{ value: 'anthropic/claude-fable-5-2', name: 'Fable 5.2' }] },
  { group: 'openai', name: 'OpenAI', options: [{ value: 'openai/gpt-x', name: 'GPT X', description: 'logged in' }] },
] };

test('pickers show every select and boolean option with the value the agent reports', () => {
  const got = pickers('claude-code', CLAUDE);
  assert.deepEqual(got.map((p) => [p.id, p.kind, p.value]), [['mode', 'select', 'default'], ['model', 'select', 'default'], ['effort', 'select', 'high'], ['ultracode', 'boolean', false]]);
  assert.deepEqual(got[0].choices.filter((c) => c.skipsPermissions).map((c) => c.value), ['bypassPermissions']);
  assert.ok(got[1].choices.every((c) => !c.skipsPermissions), 'only a mode skips permissions');

  const [codex] = pickers('codex', [CODEX_MODE]);
  assert.deepEqual(codex.choices.filter((c) => c.skipsPermissions).map((c) => c.value), ['agent-full-access']);
  // The same value names nothing for another agent.
  assert.equal(skipsPermissions('opencode', CODEX_MODE, 'agent-full-access'), false);

  const [model] = pickers('opencode', [OPENCODE_MODEL]);
  assert.deepEqual(model.choices.map((c) => [c.value, c.group]), [['anthropic/claude-fable-5-2', 'Anthropic'], ['openai/gpt-x', 'OpenAI']]);
  assert.equal(model.choices[1].description, 'logged in');
});

test("the dropdown options name the choices plainly and keep the agent's groups", () => {
  const [mode] = pickers('claude-code', CLAUDE);
  assert.deepEqual(pickerOptions(mode).map((o) => [o.value, o.label]), [
    ['default', 'Default'],
    ['acceptEdits', 'Accept Edits'],
    ['plan', 'Plan'],
    ['bypassPermissions', 'Bypass Permissions'],
  ]);
  const [model] = pickers('opencode', [OPENCODE_MODEL]);
  assert.deepEqual(pickerOptions(model), [
    { value: 'anthropic/claude-fable-5-2', label: 'Fable 5.2', group: 'Anthropic' },
    { value: 'openai/gpt-x', label: 'GPT X', title: 'logged in', group: 'OpenAI' },
  ]);
  const [codexMode] = pickers('codex', [CODEX_MODE]);
  assert.equal(pickerOptions(codexMode)[2].label, 'Agent (full access)');
});

test('a value the agent reports but does not list still shows; options without values do not', () => {
  const [p] = pickers('codex', [{ ...CODEX_MODE, currentValue: 'custom' }]);
  assert.equal(p.value, 'custom');
  assert.equal(p.choices[0].value, 'custom');
  assert.deepEqual(pickers('codex', [{ id: 'x', name: 'X', type: 'select', options: [] }, { id: 'y', name: 'Y', type: 'text', currentValue: 'a' }]), []);
  assert.deepEqual(selectValues(undefined), []);
});

test('remembered choices apply when the agent still offers them, in its order', () => {
  const saved = { effort: 'low', model: 'fable', gone: 'x', ultracode: true };
  assert.deepEqual(choicesToApply('claude-code', CLAUDE, saved), [['model', 'fable'], ['effort', 'low'], ['ultracode', true]]);
  assert.deepEqual(choicesToApply('claude-code', CLAUDE, { model: 'retired-model', effort: 'high', ultracode: 'yes' }), [], 'not offered, already set, or the wrong type');
  assert.deepEqual(choicesToApply('opencode', [OPENCODE_MODEL], { model: 'openai/gpt-x' }), [['model', 'openai/gpt-x']], 'values inside groups count');
});

test('a mode that skips permission requests is never applied from memory', () => {
  assert.deepEqual(choicesToApply('claude-code', CLAUDE, { mode: 'bypassPermissions' }), []);
  assert.deepEqual(choicesToApply('codex', [CODEX_MODE], { mode: 'agent-full-access' }), []);
  assert.deepEqual(choicesToApply('codex', [CODEX_MODE], { mode: 'read-only' }), [['mode', 'read-only']]);
});

function withStorage(fn) {
  const had = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
  const data = new Map();
  globalThis.localStorage = {
    getItem: (k) => (data.has(k) ? data.get(k) : null),
    setItem: (k, v) => data.set(k, String(v)),
    removeItem: (k) => data.delete(k),
  };
  try {
    return fn(data);
  } finally {
    if (had) Object.defineProperty(globalThis, 'localStorage', had);
    else delete globalThis.localStorage;
  }
}

test('choices are remembered per agent and host', () => {
  withStorage((data) => {
    saveChoice('claude-code', undefined, CLAUDE[1], 'fable');
    saveChoice('claude-code', undefined, CLAUDE[0], 'plan');
    saveChoice('claude-code', 'dev-box', CLAUDE[1], 'default');
    saveChoice('codex', undefined, CODEX_MODE, 'read-only');
    assert.deepEqual(loadChoices('claude-code'), { model: 'fable', mode: 'plan' });
    assert.deepEqual(loadChoices('claude-code', 'local'), { model: 'fable', mode: 'plan' }, 'local is this computer');
    assert.deepEqual(loadChoices('claude-code', 'dev-box'), { model: 'default' });
    assert.deepEqual(loadChoices('codex'), { mode: 'read-only' });
    assert.deepEqual(loadChoices('opencode'), {});
    assert.ok([...data.keys()].every((k) => k.startsWith('agent-sessions:chat-options:')));
    assert.ok(![...data.keys()].some((k) => k.includes('dev-box')), 'host names are hashed, as other per-host keys are');

    // Picking a mode that skips permission requests forgets the mode instead.
    saveChoice('claude-code', undefined, CLAUDE[0], 'bypassPermissions');
    assert.deepEqual(loadChoices('claude-code'), { model: 'fable' });

    // Damaged storage reads as nothing remembered.
    data.set([...data.keys()][0], '[1,2]');
    data.set([...data.keys()][1], '{not json');
    assert.deepEqual(loadChoices('claude-code', 'dev-box'), {});
  });
  assert.deepEqual(loadChoices('claude-code'), {}, 'no storage');
});

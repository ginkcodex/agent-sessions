import test from 'node:test';
import assert from 'node:assert/strict';
import {
  applySessionUpdate,
  beginTurn,
  emptyTail,
  endTurn,
  MAX_HELD_EVENTS,
  newSequence,
  reconcile,
  savedMessageId,
  sequence,
  toolOutput,
} from '../src/lib/chatTail.ts';

const chunk = (kind, text, messageId) => ({ sessionUpdate: kind, content: { type: 'text', text }, ...(messageId && { messageId }) });
const apply = (tail, ...updates) => updates.reduce(applySessionUpdate, tail);
const turn = (prompt) => beginTurn(emptyTail(), prompt && { text: prompt, sentAt: Date.parse('2026-11-11T10:00:00Z') });

test('chunks append to the item with their messageId; a new messageId starts an item', () => {
  let t = apply(
    turn('hi'),
    chunk('agent_thought_chunk', 'Let me ', 'm1'),
    chunk('agent_thought_chunk', 'think.', 'm1'),
    chunk('agent_message_chunk', 'Hello', 'm1'),
    chunk('agent_message_chunk', ' world', 'm1'),
    chunk('agent_message_chunk', 'Second', 'm2'),
  );
  assert.deepEqual(
    t.items.map((i) => [i.type, i.role ?? '', i.messageId ?? '', i.text]),
    [
      ['text', 'user', '', 'hi'],
      ['thought', '', 'm1', 'Let me think.'],
      ['text', 'assistant', 'm1', 'Hello world'],
      ['text', 'assistant', 'm2', 'Second'],
    ],
  );
  // A late chunk of an earlier message still finds it.
  t = applySessionUpdate(t, chunk('agent_message_chunk', '!', 'm1'));
  assert.equal(t.items[2].text, 'Hello world!');
  assert.equal(t.items.length, 4);
  assert.equal(new Set(t.items.map((i) => i.key)).size, 4, 'keys are unique');
  assert.ok(t.items.every((i) => i.turn === 1));
});

test('chunks without a messageId continue the item they follow', () => {
  const t = apply(
    turn(),
    chunk('agent_message_chunk', 'a'),
    chunk('agent_message_chunk', 'b'),
    chunk('agent_thought_chunk', 'c'),
    chunk('agent_message_chunk', 'd'),
    chunk('user_message_chunk', 'e'),
  );
  assert.deepEqual(t.items.map((i) => [i.type, i.role ?? '', i.text]), [
    ['text', 'assistant', 'ab'],
    ['thought', '', 'c'],
    ['text', 'assistant', 'd'],
    ['text', 'user', 'e'],
  ]);
});

test('chunks with no text, and non-text blocks, change nothing', () => {
  const t = turn();
  assert.equal(applySessionUpdate(t, chunk('agent_message_chunk', '', 'm1')), t);
  assert.equal(applySessionUpdate(t, { sessionUpdate: 'agent_message_chunk', content: { type: 'image', data: 'xx' } }), t);
  assert.equal(applySessionUpdate(t, { sessionUpdate: 'agent_message_chunk' }), t);
});

test('a Claude tool call fills in over its updates and shows a Write as a diff', () => {
  const id = 'toolu_01';
  const t = apply(
    turn(),
    { sessionUpdate: 'tool_call', toolCallId: id, name: 'Write', title: 'Preparing file…', kind: 'edit', status: 'pending', rawInput: {}, content: [], locations: [] },
    { sessionUpdate: 'tool_call_update', toolCallId: id, title: 'Write hello.txt', rawInput: { file_path: '/p/hello.txt' }, locations: [{ path: '/p/hello.txt' }] },
    { sessionUpdate: 'tool_call_update', toolCallId: id, rawInput: { file_path: '/p/hello.txt', content: 'hi\n' }, content: [{ type: 'diff', path: '/p/hello.txt', oldText: null, newText: 'hi\n' }] },
    { sessionUpdate: 'tool_call_update', toolCallId: id, _meta: { claudeCode: { toolName: 'Write' } } },
    { sessionUpdate: 'tool_call_update', toolCallId: id, status: 'completed', rawOutput: 'File created successfully' },
  );
  assert.equal(t.items.length, 1);
  const tool = t.items[0];
  assert.equal(tool.type, 'tool');
  assert.equal(tool.name, 'Write');
  assert.equal(tool.title, 'Write hello.txt');
  assert.equal(tool.kind, 'edit');
  assert.equal(tool.status, 'completed');
  assert.deepEqual(tool.input, { file_path: '/p/hello.txt', content: 'hi\n' });
  assert.deepEqual(tool.locations, [{ path: '/p/hello.txt' }]);
  assert.deepEqual(tool.diffs, [{ path: '/p/hello.txt', oldText: null, newText: 'hi\n' }]);
  assert.equal(tool.terminal, undefined);
  assert.equal(toolOutput(tool), 'File created successfully');
});

test("Codex's command output and exit code come from _meta", () => {
  const id = 'exec-1';
  let t = apply(
    turn(),
    { sessionUpdate: 'tool_call', toolCallId: id, name: 'exec_command', kind: 'execute', title: 'ls', status: 'in_progress', content: [{ type: 'terminal', terminalId: id }], rawInput: { command: 'ls' }, _meta: { terminal_info: { terminal_id: id } } },
  );
  assert.deepEqual(t.items[0].terminal, { output: '' });
  t = apply(
    t,
    { sessionUpdate: 'tool_call_update', toolCallId: id, _meta: { terminal_output_delta: { data: 'a.txt\n', terminal_id: id } } },
    { sessionUpdate: 'tool_call_update', toolCallId: id, status: 'completed', _meta: { terminal_output_delta: { data: 'b.txt\n', terminal_id: id }, terminal_exit: { exit_code: 0, signal: null, terminal_id: id } } },
  );
  assert.deepEqual(t.items[0].terminal, { output: 'a.txt\nb.txt\n', exitCode: 0 });
  assert.equal(t.items[0].status, 'completed');
  assert.equal(toolOutput(t.items[0]), 'a.txt\nb.txt\n');
  // A command killed by a signal has no exit code.
  t = applySessionUpdate(t, { sessionUpdate: 'tool_call_update', toolCallId: id, _meta: { terminal_exit: { exit_code: null, signal: 'SIGKILL' } } });
  assert.equal(t.items[0].terminal.exitCode, null);
});

test('content replaces the old content; an update for an unknown call starts it', () => {
  let t = apply(
    turn(),
    { sessionUpdate: 'tool_call_update', toolCallId: 'call_1', status: 'in_progress', kind: 'execute', title: 'touch x', rawInput: { command: 'touch x' } },
    { sessionUpdate: 'tool_call_update', toolCallId: 'call_1', status: 'failed', content: [{ type: 'content', content: { type: 'text', text: 'The user declined this tool call' } }], rawOutput: { error: 'declined' } },
  );
  assert.equal(t.items.length, 1);
  assert.equal(t.items[0].status, 'failed');
  assert.equal(toolOutput(t.items[0]), 'The user declined this tool call');
  t = applySessionUpdate(t, { sessionUpdate: 'tool_call_update', toolCallId: 'call_1', content: [] });
  assert.equal(t.items[0].text, '');
  assert.equal(applySessionUpdate(t, { sessionUpdate: 'tool_call_update' }), t, 'no toolCallId');
});

test('a plan replaces the plan item; a later turn moves it to the end', () => {
  const plan = (...statuses) => ({ sessionUpdate: 'plan', entries: statuses.map((status, i) => ({ content: `step ${i}`, priority: 'high', status })) });
  let t = apply(turn('go'), plan('in_progress', 'pending'), chunk('agent_message_chunk', 'working', 'm1'), plan('completed', 'in_progress'));
  assert.deepEqual(t.items.map((i) => i.type), ['text', 'plan', 'text']);
  assert.deepEqual(t.items[1].entries.map((e) => e.status), ['completed', 'in_progress']);
  t = apply(beginTurn(endTurn(t)), chunk('agent_message_chunk', 'next', 'm2'), plan('completed', 'completed'));
  assert.deepEqual(t.items.map((i) => i.type), ['text', 'text', 'text', 'plan']);
  assert.equal(t.items.filter((i) => i.type === 'plan').length, 1);
  assert.equal(t.items[3].turn, 2);
  assert.equal(applySessionUpdate(t, { sessionUpdate: 'plan', entries: 'nope' }), t);
});

test('kinds the tail does not keep, and unknown kinds, return the tail unchanged', () => {
  const t = turn();
  for (const u of [
    { sessionUpdate: 'available_commands_update', availableCommands: [] },
    { sessionUpdate: 'current_mode_update', currentModeId: 'plan' },
    { sessionUpdate: 'config_option_update', configOptions: [] },
    { sessionUpdate: 'something_new', x: 1 },
    undefined,
    null,
  ]) {
    assert.equal(applySessionUpdate(t, u), t, JSON.stringify(u));
  }
  const titled = applySessionUpdate(t, { sessionUpdate: 'session_info_update', title: 'Fix the build' });
  assert.equal(titled.title, 'Fix the build');
  assert.equal(applySessionUpdate(titled, { sessionUpdate: 'session_info_update', title: 'Fix the build' }), titled);
});

test('usage_update keeps the latest context use; a malformed one changes nothing', () => {
  const t = turn();
  const used = applySessionUpdate(t, { sessionUpdate: 'usage_update', used: 15385, size: 475000 });
  assert.deepEqual(used.usage, { used: 15385, size: 475000 });
  assert.equal(used.items, t.items, 'no item for usage');
  assert.equal(applySessionUpdate(used, { sessionUpdate: 'usage_update', used: 15385, size: 475000, cost: { amount: 0, currency: 'USD' } }), used);
  assert.deepEqual(applySessionUpdate(used, { sessionUpdate: 'usage_update', used: 16000, size: 475000 }).usage, { used: 16000, size: 475000 });
  for (const bad of [{}, { used: 1 }, { used: '1', size: 10 }, { used: 1, size: 0 }, { used: -1, size: 10 }, { used: Infinity, size: 10 }]) {
    assert.equal(applySessionUpdate(used, { sessionUpdate: 'usage_update', ...bad }), used, JSON.stringify(bad));
  }
});

test('turns count up; the prompt sent starts its turn', () => {
  let t = beginTurn(emptyTail(), { text: 'one', sentAt: 1 });
  assert.equal(t.turn, 1);
  assert.equal(t.turnOpen, true);
  assert.deepEqual(t.items.map((i) => [i.role, i.text, i.sentAt]), [['user', 'one', 1]]);
  t = endTurn(t);
  assert.equal(t.turnOpen, false);
  assert.equal(endTurn(t), t);
  t = beginTurn(t);
  assert.equal(t.turn, 2);
  assert.equal(t.items.length, 1, 'no prompt, no item');
});

const msg = (id, role, parts, time = '2026-11-11T10:00:05Z') => ({ id, role, time, parts });

test('reconcile drops saved messages once they are complete, never the one still streaming', () => {
  let t = apply(
    turn('hi'),
    chunk('agent_thought_chunk', 'thinking', 'm1'),
    chunk('agent_message_chunk', 'Hello', 'm1'),
  );
  // Claude saves the thinking block first: the thought goes, the text streams on.
  const savedThought = [msg('u1', 'user', [{ kind: 'text', text: 'hi' }]), msg('m1', 'assistant', [{ kind: 'reasoning', text: 'thinking' }])];
  t = reconcile(t, savedThought);
  assert.deepEqual(t.items.map((i) => i.type), ['text']);
  assert.equal(t.items[0].text, 'Hello');
  // The text block saved, but the turn runs and the text is the last item.
  const savedText = [msg('u1', 'user', [{ kind: 'text', text: 'hi' }]), msg('m1', 'assistant', [{ kind: 'reasoning', text: 'thinking' }, { kind: 'text', text: 'Hello' }])];
  assert.equal(reconcile(t, savedText), t);
  t = endTurn(t);
  assert.deepEqual(reconcile(t, savedText).items, []);
});

test('reconcile keeps items whose message is not saved, and returns the same tail when nothing changes', () => {
  const t = endTurn(apply(turn('hi'), chunk('agent_message_chunk', 'Hello', 'm1')));
  assert.equal(reconcile(t, []), t);
  assert.equal(reconcile(t, [msg('other', 'assistant', [{ kind: 'text', text: 'x' }])]).items.length, 2);
  // A saved message without a text part does not stand for streamed text.
  assert.deepEqual(reconcile(t, [msg('m1', 'assistant', [{ kind: 'reasoning', text: 'x' }])]).items.map((i) => i.role), ['user', 'assistant']);
});

test("reconcile matches OpenCode's thought IDs without their :reasoning:N suffix", () => {
  assert.equal(savedMessageId('msg_1:reasoning:0'), 'msg_1');
  assert.equal(savedMessageId('msg_1'), 'msg_1');
  let t = apply(turn(), chunk('agent_thought_chunk', 'hmm', 'msg_1:reasoning:0'), chunk('agent_message_chunk', 'ok', 'msg_1'), chunk('agent_message_chunk', 'more', 'msg_2'));
  t = reconcile(t, [msg('msg_1', 'assistant', [{ kind: 'reasoning', text: 'hmm' }, { kind: 'text', text: 'ok' }])]);
  assert.deepEqual(t.items.map((i) => i.messageId), ['msg_2']);
});

test('reconcile drops tool calls saved under their ID once they have a result', () => {
  let t = apply(
    turn(),
    { sessionUpdate: 'tool_call', toolCallId: 'toolu_1', title: 'Bash', status: 'completed' },
    { sessionUpdate: 'tool_call', toolCallId: 'toolu_2', title: 'Read', status: 'in_progress' },
    chunk('agent_message_chunk', 'still going', 'm2'),
  );
  const saved = [msg('m1', 'assistant', [
    { kind: 'tool', tool: { id: 'toolu_1', name: 'Bash', status: 'completed' } },
    { kind: 'tool', tool: { id: 'toolu_2', name: 'Read', status: 'pending' } },
  ])];
  t = reconcile(t, saved);
  assert.deepEqual(t.items.map((i) => i.toolCallId ?? i.messageId), ['toolu_2', 'm2'], 'a saved call without a result stays while the turn runs');
  assert.deepEqual(reconcile(endTurn(t), saved).items.map((i) => i.messageId), ['m2']);
});

test("reconcile drops a whole ended turn once its last message is saved: Codex's tool IDs differ", () => {
  let t = apply(
    turn('list'),
    { sessionUpdate: 'plan', entries: [{ content: 'list', status: 'completed' }] },
    { sessionUpdate: 'tool_call', toolCallId: 'exec-1', kind: 'execute', status: 'completed', content: [{ type: 'terminal', terminalId: 'exec-1' }] },
    chunk('agent_message_chunk', 'Two files.', 'msg_a'),
  );
  t = beginTurn(endTurn(t), { text: 'again', sentAt: Date.parse('2026-11-11T10:01:00Z') });
  t = applySessionUpdate(t, chunk('agent_message_chunk', 'Working', 'msg_b'));
  const saved = [
    msg('u1', 'user', [{ kind: 'text', text: 'list' }]),
    msg('msg_a', 'assistant', [{ kind: 'tool', tool: { id: 'call_x', name: 'exec_command', status: 'completed' } }, { kind: 'text', text: 'Two files.' }]),
  ];
  t = reconcile(t, saved);
  assert.deepEqual(t.items.map((i) => [i.turn, i.type, i.text ?? '']), [[2, 'text', 'again'], [2, 'text', 'Working']]);
});

test('reconcile drops a prompt once a saved user message has its text, not an older one', () => {
  const sentAt = Date.parse('2026-11-11T10:00:00Z');
  const t = beginTurn(emptyTail(), { text: 'continue', sentAt });
  const older = [msg('u0', 'user', [{ kind: 'text', text: 'continue' }], '2026-11-11T09:00:00Z')];
  assert.equal(reconcile(t, older).items.length, 1);
  const now = [msg('u1', 'user', [{ kind: 'text', text: ' continue\n' }], '2026-11-11T10:00:01Z')];
  assert.equal(reconcile(t, now).items.length, 0);
  // A replayed user message is matched by its ID.
  const replayed = applySessionUpdate(beginTurn(emptyTail()), chunk('user_message_chunk', 'old prompt', 'u7'));
  assert.equal(reconcile(replayed, [msg('u7', 'user', [{ kind: 'text', text: 'other' }])]).items.length, 0);
});

test('sequence applies events in seq order and drops duplicates and old ones', () => {
  const ev = (seq) => ({ seq });
  let s = newSequence(1);
  const take = (e) => {
    const r = sequence(s, e);
    s = r.seq;
    return r.ready.map((x) => x.seq);
  };
  assert.deepEqual(take(ev(1)), [1]);
  assert.deepEqual(take(ev(3)), []);
  assert.deepEqual(take(ev(4)), []);
  assert.deepEqual(take(ev(3)), [], 'duplicate of a held event');
  assert.deepEqual(take(ev(2)), [2, 3, 4]);
  assert.deepEqual(take(ev(2)), [], 'already applied');
  assert.deepEqual(take(ev(0)), [], 'not a seq');
  assert.deepEqual(take({ seq: 1.5 }), []);
  assert.equal(s.next, 5);
  assert.deepEqual(s.held, []);
});

test('sequence starts from the first event when the base is not known', () => {
  let r = sequence(newSequence(), { seq: 40 });
  assert.deepEqual(r.ready.map((e) => e.seq), [40]);
  r = sequence(r.seq, { seq: 39 });
  assert.deepEqual(r.ready, []);
  r = sequence(r.seq, { seq: 41 });
  assert.deepEqual(r.ready.map((e) => e.seq), [41]);
});

test('sequence skips an event that never comes once too many wait behind it', () => {
  let s = newSequence(1);
  for (let i = 0; i < MAX_HELD_EVENTS; i++) {
    const r = sequence(s, { seq: i + 2 });
    s = r.seq;
    assert.deepEqual(r.ready, []);
  }
  const r = sequence(s, { seq: MAX_HELD_EVENTS + 2 });
  assert.equal(r.ready.length, MAX_HELD_EVENTS + 1);
  assert.equal(r.ready[0].seq, 2);
  assert.equal(r.seq.next, MAX_HELD_EVENTS + 3);
});

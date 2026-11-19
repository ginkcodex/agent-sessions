import test from 'node:test';
import assert from 'node:assert/strict';
import {
  addAnswer,
  addPrompt,
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

test('reconcile drops thoughts and compactions before the last saved item of a running turn', () => {
  const compact = (id) => ({ sessionUpdate: 'tool_call', toolCallId: id, title: 'Compact conversation', kind: 'think', status: 'completed' });
  const t = apply(
    turn('go'),
    chunk('agent_thought_chunk', 'a', 'm1'),
    compact('c1'),
    chunk('agent_thought_chunk', 'b', 'm2'),
    { sessionUpdate: 'tool_call', toolCallId: 'toolu_1', title: 'Bash', kind: 'execute', status: 'in_progress' },
    compact('c2'),
    chunk('agent_thought_chunk', 'c', 'm3'),
  );
  // Claude saves a thinking block empty, so no saved part stands for a thought.
  const saved = [msg('u1', 'user', [{ kind: 'text', text: 'go' }]), msg('m2', 'assistant', [{ kind: 'tool', tool: { id: 'toolu_1', name: 'Bash', status: 'pending' } }])];
  assert.deepEqual(reconcile(t, saved).items.map((i) => i.toolCallId ?? i.messageId), ['toolu_1', 'c2', 'm3']);
  // Nothing saved yet: nothing is dropped.
  assert.equal(reconcile(t, [saved[0]]).items.length, t.items.length - 1);
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

test('reconcile matches a command typed by its short name to the full name it is saved under', () => {
  const sentAt = Date.parse('2026-11-11T10:00:00Z');
  const at = '2026-11-11T10:00:01Z';
  const user = (text) => [msg('u1', 'user', [{ kind: 'text', text }], at)];
  const left = (typed, saved) => reconcile(beginTurn(emptyTail(), { text: typed, sentAt }), user(saved)).items.length;
  assert.equal(left('/ponytail full', '/ponytail:ponytail full'), 0);
  assert.equal(left('/ponytail', '/ponytail:ponytail'), 0);
  assert.equal(left('/ponytail:ponytail full', '/ponytail:ponytail full'), 0);
  assert.equal(left('$sites', '$sites:sites'), 0);
  assert.equal(left('/understand how\nit works', '/understand-anything:understand how\nit works'), 0);
  // Another command, other arguments, another sigil or a different full name are other prompts.
  assert.equal(left('/ponytail full', '/ponytail:ponytail lite'), 1);
  assert.equal(left('/ponytail full', '/ponytail:ponytail'), 1);
  assert.equal(left('/tail', '/ponytail:ponytail'), 1);
  assert.equal(left('/ponytail', '$ponytail:ponytail'), 1);
  assert.equal(left('/a:ponytail', '/b:ponytail'), 1);
  assert.equal(left('ponytail', 'x:ponytail'), 1);
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

test('a prompt of only files starts a user item that waits for its turn to be saved', () => {
  const files = [{ name: 'shot.png', mimeType: 'image/png', size: 70, data: 'iVBORw0KGgo=' }, { name: 'notes.md', mimeType: 'text/markdown', size: 12 }];
  let t = beginTurn(emptyTail(), { text: '', sentAt: Date.parse('2026-11-11T10:00:00Z'), attachments: files });
  assert.deepEqual(t.items.map((i) => [i.type, i.role, i.text]), [['text', 'user', '']]);
  assert.deepEqual(t.items[0].attachments, files);
  // No text and no files: no user item.
  assert.deepEqual(beginTurn(emptyTail(), { text: '', sentAt: 0 }).items, []);

  t = apply(t, chunk('agent_message_chunk', 'Got them', 'm1'));
  const saved = [
    msg('u1', 'user', [{ kind: 'file', file: { name: 'shot.png' } }, { kind: 'file', file: { name: 'notes.md' } }]),
    msg('m1', 'assistant', [{ kind: 'text', text: 'Got them' }]),
  ];
  // The turn runs: nothing in it is known to be saved yet.
  assert.equal(reconcile(t, saved), t);
  // Once it ends with its last message saved, the whole turn goes.
  assert.deepEqual(reconcile(endTurn(t), saved).items, []);
});

test('a prompt with text and files keeps both on its user item and matches its saved text', () => {
  const files = [{ name: 'notes.md', mimeType: 'text/markdown', size: 12 }];
  const t = endTurn(beginTurn(emptyTail(), { text: 'read this', sentAt: Date.parse('2026-11-11T10:00:00Z'), attachments: files }));
  assert.equal(t.items[0].text, 'read this');
  assert.deepEqual(t.items[0].attachments, files);
  assert.deepEqual(reconcile(t, [msg('u1', 'user', [{ kind: 'text', text: 'read this' }, { kind: 'file', file: { name: 'notes.md' } }])]).items, []);
});

test("a question's outcome stays in its turn until the tool call that asked is saved", () => {
  let t = apply(turn('choose'), chunk('agent_message_chunk', 'Asking.', 'm1'));
  t = addAnswer(t, { questionId: 'question-1', toolCallId: 'toolu_ask', outcome: 'accept', summary: 'Answered: Yes' });
  assert.equal(addAnswer(t, { questionId: 'question-1', outcome: 'cancel', summary: 'x' }), t, 'one line per question');
  const answer = t.items.at(-1);
  assert.deepEqual([answer.type, answer.turn, answer.summary], ['answer', 1, 'Answered: Yes']);

  const user = { id: 'u1', role: 'user', time: '2026-11-11T10:00:01Z', parts: [{ kind: 'text', text: 'choose' }] };
  assert.equal(reconcile(t, [user]).items.at(-1), answer, 'an unsaved question stays');
  const asked = { id: 'a1', role: 'assistant', time: '', parts: [{ kind: 'tool', tool: { id: 'toolu_ask', name: 'AskUserQuestion', status: 'completed' } }] };
  assert.deepEqual(reconcile(t, [user, asked]).items.map((i) => i.type), ['text'], 'the saved tool call shows the answer');

  // Without a tool call, the line goes with its saved turn.
  let u = addAnswer(t, { questionId: 'question-2', outcome: 'withdrawn', summary: 'The agent went on without an answer' });
  u = endTurn(u);
  const reply = { id: 'm1', role: 'assistant', time: '', parts: [{ kind: 'text', text: 'Asking.' }] };
  assert.deepEqual(reconcile(u, [user, reply]).items, []);
});

test('a steered message joins the open turn after what it streamed, and its saved copy replaces it', () => {
  const sentAt = Date.parse('2026-11-11T10:00:00Z');
  let t = beginTurn(emptyTail(), { text: 'fix the build', sentAt });
  t = applySessionUpdate(t, { sessionUpdate: 'agent_message_chunk', messageId: 'm1', content: { type: 'text', text: 'Looking.' } });
  t = addPrompt(t, { text: 'also the tests', sentAt: sentAt + 5000, attachments: [{ name: 'a.png', mimeType: 'image/png', size: 3, data: 'AAA' }] });
  assert.equal(t.turn, 1);
  assert.deepEqual(
    t.items.map((i) => [i.turn, i.role, i.text]),
    [
      [1, 'user', 'fix the build'],
      [1, 'assistant', 'Looking.'],
      [1, 'user', 'also the tests'],
    ],
  );
  assert.equal(t.items[2].attachments[0].data, 'AAA');
  assert.equal(addPrompt(t, { text: '', sentAt }), t, 'an empty message adds nothing');

  const saved = [
    { id: 'u1', role: 'user', time: '2026-11-11T10:00:00Z', parts: [{ kind: 'text', text: 'fix the build' }] },
    { id: 'u2', role: 'user', time: '2026-11-11T10:00:05Z', parts: [{ kind: 'text', text: 'also the tests' }] },
  ];
  assert.deepEqual(reconcile(t, saved).items.map((i) => i.text), ['Looking.']);
});

// Antigravity (agy acp) streams no message IDs, and its saved tool calls have
// other IDs than the streamed ones: its turns match the saved ones by prompt.
const agyTool = (id, status, extra = {}) => ({ sessionUpdate: id ? 'tool_call_update' : 'tool_call', toolCallId: 'call_1', name: 'run_command', args: { CommandLine: 'echo hi' }, rawInput: { CommandLine: 'echo hi' }, kind: 'execute', title: 'echo hi', status, ...extra });
const agyTurn = (tail, prompt, sentAt, ...updates) => apply(beginTurn(tail, { text: prompt, sentAt: Date.parse(sentAt) }), ...updates);
const saidUser = (id, text, time) => msg(id, 'user', [{ kind: 'text', text }], time);
const saidAgent = (id, text, time, tools = []) => msg(id, 'assistant', [...tools.map((t) => ({ kind: 'tool', tool: { status: 'completed', ...t } })), ...(text ? [{ kind: 'text', text }] : [])], time);

test('Antigravity: an ended turn goes whole once its prompt and reply are saved, turn by turn', () => {
  let t = agyTurn(emptyTail(), 'run it', '2026-11-11T10:00:00Z',
    chunk('agent_thought_chunk', 'Running the command.'),
    agyTool(false, 'in_progress'),
    agyTool(true, 'completed', { rawOutput: { output: 'hi\n', exit_code: 0 } }),
    chunk('agent_message_chunk', 'done'));
  const turn1 = [saidUser('s0', 'run it', '2026-11-11T10:00:01Z'), saidAgent('s1', '', '2026-11-11T10:00:02Z', [{ id: 'step-1-0', name: 'run_command', input: { CommandLine: 'echo hi' } }])];
  // While the turn runs, its saved prompt replaces the live one; the rest stays.
  t = reconcile(t, turn1);
  assert.deepEqual(t.items.map((i) => i.type), ['thought', 'tool', 'text']);
  // Ended, with the reply not yet saved: everything stays.
  t = endTurn(t);
  assert.equal(reconcile(t, turn1), t);
  const saved1 = [...turn1, saidAgent('s2', 'done', '2026-11-11T10:00:03Z')];
  t = reconcile(t, saved1);
  assert.deepEqual(t.items, []);
  assert.equal(reconcile(t, saved1), t);

  // A second turn streams under the first one's saved messages.
  t = agyTurn(t, 'again', '2026-11-11T10:01:00Z', chunk('agent_message_chunk', 'ok'));
  t = endTurn(t);
  assert.deepEqual(reconcile(t, saved1).items.map((i) => i.text), ['again', 'ok']);
  assert.deepEqual(reconcile(t, [...saved1, saidUser('s3', 'again', '2026-11-11T10:01:01Z'), saidAgent('s4', 'ok', '2026-11-11T10:01:02Z')]).items, []);
});

test('Antigravity: a repeated prompt waits for its own saved copy, not the earlier turn\'s', () => {
  const first = [saidUser('s0', 'continue', '2026-11-11T10:00:01Z'), saidAgent('s1', 'done', '2026-11-11T10:00:02Z')];
  let t = endTurn(agyTurn(emptyTail(), 'continue', '2026-11-11T10:00:00Z', chunk('agent_message_chunk', 'done')));
  t = reconcile(t, first);
  assert.deepEqual(t.items, []);
  // Ten seconds on, the same prompt and the same reply: the first turn's saved copy is not this one.
  t = endTurn(agyTurn(t, 'continue', '2026-11-11T10:00:10Z', chunk('agent_message_chunk', 'done')));
  // The live prompt goes by its text, as for every agent; the reply stays.
  assert.deepEqual(reconcile(t, first).items.map((i) => i.text), ['done']);
  const both = [...first, saidUser('s2', 'continue', '2026-11-11T10:00:11Z'), saidAgent('s3', 'done', '2026-11-11T10:00:12Z')];
  assert.deepEqual(reconcile(t, both).items, []);
});

test('Antigravity: a tool-only turn goes once its tool calls are saved; a cancelled one once the next prompt is', () => {
  let t = endTurn(agyTurn(emptyTail(), 'run it', '2026-11-11T10:00:00Z', agyTool(false, 'in_progress'), agyTool(true, 'completed')));
  const prompt = saidUser('s0', 'run it', '2026-11-11T10:00:01Z');
  assert.equal(reconcile(t, [prompt]).items.length, 1, 'the tool call waits');
  const other = saidAgent('s1', '', '2026-11-11T10:00:02Z', [{ id: 'step-1-0', name: 'run_command', input: { CommandLine: 'echo bye' } }]);
  assert.equal(reconcile(t, [prompt, other]).items.length, 1, 'another input is another call');
  // The saved input may list its keys in another order.
  t = endTurn(agyTurn(emptyTail(), 'run it', '2026-11-11T10:00:00Z', agyTool(false, 'in_progress', { rawInput: { CommandLine: 'echo hi', Wait: 5 } })));
  const same = saidAgent('s1', '', '2026-11-11T10:00:02Z', [{ id: 'step-1-0', name: 'run_command', input: { Wait: 5, CommandLine: 'echo hi' } }]);
  assert.deepEqual(reconcile(t, [prompt, same]).items, []);

  // Cancelled while thinking: nothing to match, until the next prompt is saved.
  t = endTurn(agyTurn(emptyTail(), 'think', '2026-11-11T10:00:00Z', chunk('agent_thought_chunk', 'Hmm')));
  const cancelled = [saidUser('s0', 'think', '2026-11-11T10:00:01Z'), { ...msg('s1', 'system', [{ kind: 'text', text: 'ERROR_MESSAGE' }], '2026-11-11T10:00:02Z'), isMeta: true }];
  assert.deepEqual(reconcile(t, cancelled).items.map((i) => i.type), ['thought']);
  t = agyTurn(t, 'go on', '2026-11-11T10:00:30Z');
  const later = [...cancelled, saidUser('s2', 'go on', '2026-11-11T10:00:31Z')];
  assert.deepEqual(reconcile(t, later).items, [], 'the cancelled turn goes; the open one\'s prompt is saved');
});

test('Antigravity: a notice saved between the prompt and the reply does not stop the match', () => {
  const t = endTurn(agyTurn(emptyTail(), 'hello', '2026-11-11T10:00:00Z', chunk('agent_message_chunk', 'Hi!')));
  const notice = { ...msg('s1', 'system', [{ kind: 'text', text: 'The agent restarted' }], '2026-11-11T10:00:01Z'), isMeta: true };
  const saved = [saidUser('s0', 'hello', '2026-11-11T10:00:01Z'), notice, saidAgent('s2', 'Hi!', '2026-11-11T10:00:02Z')];
  assert.deepEqual(reconcile(t, saved).items, []);
  // An ID-based agent's turn is not matched by prompt and reply text.
  const claude = endTurn(apply(beginTurn(emptyTail(), { text: 'hello', sentAt: Date.parse('2026-11-11T10:00:00Z') }), chunk('agent_message_chunk', 'Hi!', 'msg_9')));
  assert.deepEqual(reconcile(claude, saved).items.map((i) => i.messageId), ['msg_9']);
});

test('Antigravity: a command\'s output is its raw output object\'s text, shown once', () => {
  const done = apply(turn('run it'), agyTool(false, 'in_progress'), agyTool(true, 'completed', { rawOutput: { output: 'hi\n', exit_code: 0 } })).items[1];
  assert.equal(toolOutput(done), 'hi\n');
  const both = apply(turn('run it'), agyTool(true, 'completed', { rawOutput: { output: 'hi\n' }, content: [{ type: 'content', content: { type: 'text', text: 'hi\n' } }] })).items[1];
  assert.equal(toolOutput(both), 'hi\n');
  const read = apply(turn('read'), { sessionUpdate: 'tool_call', toolCallId: 'call_2', name: 'read_file', args: { Path: '/a' }, rawInput: { Path: '/a' }, kind: 'read', title: 'File a', status: 'completed' }).items[1];
  assert.equal(toolOutput(read), '');
  assert.equal(read.title, 'File a');
});

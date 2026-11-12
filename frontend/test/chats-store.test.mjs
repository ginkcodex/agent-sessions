import test from 'node:test';
import assert from 'node:assert/strict';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';
import { bumpEpoch } from '../src/lib/link.ts';
import { fixtureStore, loadServerModule } from './helpers.mjs';

const STORE_URL = new URL('../src/lib/stores/chats.svelte.ts', import.meta.url);

let storeModule;

// The real ChatsStore on the mock backend and its scripted agent.
async function loadChatsStore() {
  const lib = (rel) => JSON.stringify(new URL(rel, STORE_URL).href);
  storeModule ??= await loadServerModule(STORE_URL, [
    [
      "import { api, subscribeChatExit, subscribeChatUpdate } from '../api';",
      fixtureStore('api', '__storeFixture') +
        "const subscribeChatUpdate = (fn) => api.onEvent('chat:update', fn);" +
        "const subscribeChatExit = (fn) => api.onEvent('chat:exit', fn);",
    ],
    ["'../link'", lib('../link.ts')],
    ["'../chatTail'", lib('../chatTail.ts')],
    ["'../chatQuestions'", lib('../chatQuestions.ts')],
  ]);
  return storeModule;
}

async function withChats(fn, { init = true } = {}) {
  const { ChatsStore } = await loadChatsStore();
  const mock = new MockBackendAPI();
  const api = new Proxy(mock, { get: (m, key) => (typeof m[key] === 'function' ? m[key].bind(m) : m[key]) });
  globalThis.__storeFixture = { api };
  const store = new ChatsStore();
  if (init) await store.init();
  try {
    return await fn({ store, mock, ChatsStore });
  } finally {
    store.destroy();
    mock.chat.closeAll();
    delete globalThis.__storeFixture;
  }
}

async function until(what, ok) {
  const deadline = Date.now() + 5000;
  while (!ok()) {
    if (Date.now() > deadline) assert.fail(`timed out waiting for ${what}`);
    await new Promise((resolve) => setTimeout(resolve, 1));
  }
}

const DIR = '/home/user/projects/web-app';

test('a prompt streams into the live tail, and the saved turn replaces it', async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    // The started event came before the reply; the chat is ready.
    assert.equal(chat.phase, 'idle');
    assert.match(chat.chatId, /^chat-/);
    assert.equal(chat.session.agent, 'claude-code');
    assert.deepEqual(chat.configOptions.map((o) => o.id), ['mode', 'model']);

    await store.prompt(chat.chatId, 'tidy the readme');
    assert.equal(store.byChatId(chat.chatId).phase, 'working');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');

    const done = store.byChatId(chat.chatId);
    assert.equal(done.stopReason, 'end_turn');
    assert.equal(done.tail.turnOpen, false);
    assert.deepEqual(
      done.tail.items.map((i) => (i.type === 'text' ? `${i.type}:${i.role}` : i.type)),
      ['text:user', 'plan', 'thought', 'tool', 'text:assistant'],
    );
    assert.equal(done.tail.items[0].text, 'tidy the readme');
    assert.match(done.tail.items[4].text, /^Done\. You said "tidy the readme"/);
    assert.equal(done.tail.items[3].status, 'completed');
    assert.deepEqual(done.tail.items[1].entries.map((e) => e.status), ['completed', 'completed', 'completed']);
    assert.ok(store.forSession(done.session));

    // The mock agent saved the turn: the saved messages cover every item.
    const page = await mock.getMessages(done.session, -1, 100);
    store.reconcile(chat.chatId, page.messages);
    assert.deepEqual(store.byChatId(chat.chatId).tail.items, []);
  });
});

test('a permission request waits in the queue until answered', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'codex', dir: DIR });
    await store.prompt(chat.chatId, 'write notes, ask permission');
    await until('permission', () => store.byChatId(chat.chatId).permissions.length === 1);
    const perm = store.byChatId(chat.chatId).permissions[0];
    assert.deepEqual(perm.options.map((o) => o.kind), ['allow_once', 'allow_always', 'reject_once']);
    assert.equal(perm.toolCall.title, 'Write NOTES.md');
    assert.equal(store.byChatId(chat.chatId).phase, 'working');

    await store.respond(chat.chatId, perm.id, 'allow-once');
    assert.deepEqual(store.byChatId(chat.chatId).permissions, []);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const write = store.byChatId(chat.chatId).tail.items.find((i) => i.toolCallId?.endsWith('_write'));
    assert.equal(write.status, 'completed');
    assert.equal(write.diffs[0].path, `${DIR}/NOTES.md`);
    await assert.rejects(store.respond(chat.chatId, perm.id, 'allow-once'), /no longer pending/);
  });
});

test('cancelling answers the pending request cancelled and ends the turn cancelled', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'opencode', dir: DIR });
    await store.prompt(chat.chatId, 'approve this');
    await until('permission', () => store.byChatId(chat.chatId).permissions.length === 1);
    await store.cancel(chat.chatId);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const c = store.byChatId(chat.chatId);
    assert.deepEqual(c.permissions, []);
    assert.equal(c.stopReason, 'cancelled');
  });
});

test("Codex's command output shows in the tool item", async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'codex', dir: DIR });
    await store.prompt(chat.chatId, 'run ls');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const exec = store.byChatId(chat.chatId).tail.items.find((i) => i.kind === 'execute');
    assert.deepEqual(exec.terminal, { output: 'README.md\nmain.go\n', exitCode: 0 });
  });
});

test('a failed turn keeps its error and the login the agent asks for', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'opencode', dir: DIR });
    await store.prompt(chat.chatId, 'login first');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    let c = store.byChatId(chat.chatId);
    assert.match(c.error, /Authentication required/);
    assert.equal(c.authMethods[0].description, 'Run `mock auth login` in the terminal');
    // The next prompt clears them.
    await store.prompt(chat.chatId, 'hello');
    c = store.byChatId(chat.chatId);
    assert.equal(c.error, undefined);
    assert.equal(c.authMethods, undefined);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
  });
});

test('a refused prompt leaves the chat idle and throws', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    await store.prompt(chat.chatId, 'hang');
    await assert.rejects(store.prompt(chat.chatId, 'second'), /already running/);
    assert.equal(store.byChatId(chat.chatId).phase, 'working', 'the first turn still runs');
    await store.cancel(chat.chatId);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    assert.equal(store.byChatId(chat.chatId).tail.turn, 1, 'the refused prompt started no turn');
    assert.equal(store.byChatId(chat.chatId).tail.items.filter((i) => i.role === 'user').length, 1);
  });
});

test('options change through setOption and config_options events', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    const opts = await store.setOption(chat.chatId, 'mode', 'auto');
    assert.equal(opts.find((o) => o.id === 'mode').currentValue, 'auto');
    assert.equal(store.byChatId(chat.chatId).configOptions.find((o) => o.id === 'mode').currentValue, 'auto');
  });
});

test('a model set before the first prompt is the one the turn runs on', async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'opencode', dir: DIR });
    await store.setOption(chat.chatId, 'model', 'mock-small');
    await store.prompt(chat.chatId, 'hello');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const page = await mock.getMessages(store.byChatId(chat.chatId).session, 0, 10);
    assert.deepEqual(page.messages.filter((m) => m.role === 'assistant').map((m) => m.model), ['mock-small']);
  });
});

test('closing and crashing end the chat with their reason', async () => {
  await withChats(async ({ store }) => {
    const a = await store.start({ agent: 'claude-code', dir: DIR });
    await store.close(a.chatId);
    await until('exit', () => store.byChatId(a.chatId).phase === 'exited');
    assert.equal(store.byChatId(a.chatId).exit.reason, 'closed');
    assert.equal(store.forSession(a.session), undefined);
    store.dismiss(a.key);
    assert.equal(store.byChatId(a.chatId), undefined);

    const b = await store.start({ agent: 'codex', dir: DIR });
    await store.prompt(b.chatId, 'crash please');
    await until('exit', () => store.byChatId(b.chatId).phase === 'exited');
    const c = store.byChatId(b.chatId);
    assert.equal(c.exit.reason, 'crashed');
    assert.match(c.exit.stderrTail, /panic/);
    assert.equal(c.tail.turnOpen, false);
  });
});

test('a failed start removes the chat and throws', async () => {
  await withChats(async ({ store, mock }) => {
    mock.chat.missing.add('codex');
    await assert.rejects(store.start({ agent: 'codex', dir: DIR }), /codex-acp is not installed\. Set it up in Settings › Coding agents/);
    assert.deepEqual(store.chats, []);
    const agents = await store.loadAgents();
    assert.equal(agents.find((a) => a.agent === 'codex').available, false);
    assert.equal(store.agents.length, 3);
  });
});

test('events for unknown chats are ignored', async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    const before = JSON.stringify(store.chats);
    mock.emit('chat:update', { chatId: 'chat-999', seq: 2, kind: 'session_update', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'x' } } });
    mock.emit('chat:exit', { chatId: 'chat-999', seq: 3, reason: 'closed' });
    mock.emit('chat:update', { chatId: chat.chatId, kind: 'turn_start' }); // no seq
    mock.emit('chat:update', null);
    assert.equal(JSON.stringify(store.chats), before);
  });
});

test('events out of order are applied in seq order, duplicates once', async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR }); // seq 1 was "started"
    const u = (seq, text) => ({ chatId: chat.chatId, seq, kind: 'session_update', update: { sessionUpdate: 'agent_message_chunk', messageId: 'm', content: { type: 'text', text } } });
    mock.emit('chat:update', u(4, 'c'));
    mock.emit('chat:update', u(3, 'b'));
    assert.equal(store.byChatId(chat.chatId).tail.items.length, 0, 'waits for seq 2');
    mock.emit('chat:update', { chatId: chat.chatId, seq: 2, kind: 'turn_start' });
    mock.emit('chat:update', u(3, 'b'));
    const items = store.byChatId(chat.chatId).tail.items;
    assert.deepEqual(items.map((i) => i.text), ['bc']);
  });
});

test('events from before a host switch are ignored, and their chats dropped', async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    bumpEpoch();
    mock.emit('chat:update', { chatId: chat.chatId, seq: 2, kind: 'turn_start' });
    assert.equal(store.byChatId(chat.chatId), undefined);

    const again = await store.start({ agent: 'claude-code', dir: DIR });
    store.reset();
    assert.deepEqual(store.chats, []);
    mock.emit('chat:update', { chatId: again.chatId, seq: 2, kind: 'turn_start' });
    assert.deepEqual(store.chats, []);
  });
});

test('a start that answers after a host switch is closed and refused', async () => {
  await withChats(async ({ store, mock }) => {
    const real = mock.chatStart.bind(mock);
    mock.chatStart = async (req) => {
      const st = await real(req);
      bumpEpoch();
      return st;
    };
    await assert.rejects(store.start({ agent: 'claude-code', dir: DIR }), /connection changed/);
    assert.deepEqual(store.chats, []);
    assert.deepEqual(await mock.chatList(), [], 'the agent was closed');
  });
});

test('a reloaded page finds the open chats and follows them', async () => {
  await withChats(
    async ({ store, mock }) => {
      const st = await mock.chatStart({ agent: 'opencode', dir: DIR });
      await mock.chatPrompt(st.chatId, 'hang');
      await store.init();
      const c = store.byChatId(st.chatId);
      assert.equal(c.phase, 'working');
      assert.equal(c.agent, 'opencode');
      await store.cancel(st.chatId);
      await until('turn end', () => store.byChatId(st.chatId).phase === 'idle');
      assert.equal(store.byChatId(st.chatId).stopReason, 'cancelled');
    },
    { init: false },
  );
});

test("chats run on a connected host whose server has them, and refuse an older one", async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    await mock.connect('dev-box');
    // Connecting ended this computer's chat.
    await until('exit', () => store.byChatId(chat.chatId)?.phase === 'exited');
    const remote = await store.start({ agent: 'claude-code', dir: DIR });
    await store.prompt(remote.chatId, 'hello');
    await until('turn end', () => store.byChatId(remote.chatId).phase === 'idle');
    assert.ok(store.byChatId(remote.chatId).tail.items.length > 0, 'the host streamed');

    await mock.connect('prod-server');
    await until('exit', () => store.byChatId(remote.chatId)?.phase === 'exited');
    await assert.rejects(store.start({ agent: 'claude-code', dir: DIR }), /does not support chat/);
    await assert.rejects(store.loadAgents(), /does not support chat/);
    assert.match(store.agentsError, /does not support chat/);
  });
});

test("the same host reconnecting leaves its chats exited; another host's are forgotten", async () => {
  await withChats(async ({ store, mock }) => {
    await mock.connect('dev-box');
    const open = await store.start({ agent: 'claude-code', dir: DIR });
    await store.prompt(open.chatId, 'permission');
    await until('permission', () => store.byChatId(open.chatId).permissions.length === 1);
    const ended = await store.start({ agent: 'codex', dir: DIR });
    await store.close(ended.chatId);
    await until('exit', () => store.byChatId(ended.chatId).phase === 'exited');

    // The connection drops and comes back: the server ended its chats, and
    // their events stop. They stay so they can restart.
    bumpEpoch();
    store.reset(true);
    const c = store.byChatId(open.chatId);
    assert.equal(c.phase, 'exited');
    assert.equal(c.exit.reason, 'disconnected');
    assert.deepEqual(c.permissions, []);
    assert.equal(store.byChatId(ended.chatId).exit.reason, 'closed');
    assert.equal(store.forSession(c.session), undefined, 'the session can open again');
    mock.emit('chat:update', { chatId: open.chatId, seq: 99, kind: 'turn_start' });
    assert.equal(store.byChatId(open.chatId).phase, 'exited', 'the old connection is not followed');
    // A later event does not drop them as from before the reset.
    const next = await store.start({ agent: 'opencode', dir: DIR });
    await store.prompt(next.chatId, 'hello');
    await until('turn end', () => store.byChatId(next.chatId).phase === 'idle');
    assert.ok(store.byChatId(open.chatId));
    store.dismiss(store.byChatId(open.chatId).key);
    assert.equal(store.byChatId(open.chatId), undefined);

    bumpEpoch();
    store.reset(false);
    assert.deepEqual(store.chats, []);
  });
});

test('a reloaded page shows the permission requests still waiting, and they can be answered', async () => {
  await withChats(
    async ({ store, mock }) => {
      const st = await mock.chatStart({ agent: 'codex', dir: DIR });
      await mock.chatPrompt(st.chatId, 'write notes, ask permission');
      for (let i = 0; i < 5000 && !(await mock.chatList())[0]?.permissions?.length; i++) await new Promise((resolve) => setTimeout(resolve, 1));
      await store.init();
      const c = store.byChatId(st.chatId);
      assert.equal(c.phase, 'working');
      assert.deepEqual(c.permissions.map((p) => p.toolCall.title), ['Write NOTES.md']);
      assert.equal(c.permissions[0].toolCall.content[0].type, 'diff', 'the card can show the change');

      await store.respond(st.chatId, c.permissions[0].id, 'reject');
      assert.deepEqual(store.byChatId(st.chatId).permissions, []);
      await until('turn end', () => store.byChatId(st.chatId).phase === 'idle');
      const write = store.byChatId(st.chatId).tail.items.find((i) => i.toolCallId?.endsWith('_write'));
      assert.equal(write.status, 'failed', 'the reject reached the agent');
    },
    { init: false },
  );
});

test('every way a chat ends clears its waiting permission cards', async () => {
  await withChats(async ({ store, mock }) => {
    const waiting = async (agent) => {
      const chat = await store.start({ agent, dir: DIR });
      await store.prompt(chat.chatId, 'approve this');
      await until('permission', () => store.byChatId(chat.chatId).permissions.length === 1);
      return chat;
    };

    // Closing the chat: the agent's request is answered cancelled first.
    const closed = await waiting('claude-code');
    await store.close(closed.chatId);
    await until('exit', () => store.byChatId(closed.chatId).phase === 'exited');
    assert.deepEqual(store.byChatId(closed.chatId).permissions, []);

    // The turn ends while a request waits: its card goes with the turn.
    const ended = await waiting('codex');
    const agentSide = mock.chat['chats'].get(ended.chatId);
    mock.emit('chat:update', { chatId: ended.chatId, seq: ++agentSide.seq, kind: 'turn_end', stopReason: 'cancelled' });
    assert.equal(store.byChatId(ended.chatId).phase, 'idle');
    assert.deepEqual(store.byChatId(ended.chatId).permissions, []);

    // Quitting the app or the window closing ends every agent.
    const quit = await waiting('opencode');
    mock.chat.closeAll();
    await until('exit', () => store.byChatId(quit.chatId).phase === 'exited');
    assert.deepEqual(store.byChatId(quit.chatId).permissions, []);

    // A host switch drops the chats and their cards.
    await waiting('claude-code');
    store.reset();
    assert.deepEqual(store.chats, []);
  });
});

test('a prompt carries its files: the live tail shows them, the saved turn replaces them', async () => {
  const { mockAttachFiles } = await import('../src/lib/mock/mockChat.ts');
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    assert.deepEqual(chat.promptCapabilities, { image: true, embeddedContext: true });

    const picked = mockAttachFiles(['/home/mock/shot.png', '/home/mock/plan.md']);
    assert.deepEqual(picked.attachments.map((a) => [a.kind, a.name]), [['image', 'shot.png'], ['text', 'plan.md']]);
    await store.prompt(chat.chatId, '', picked.attachments);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');

    const done = store.byChatId(chat.chatId);
    const user = done.tail.items[0];
    assert.equal(user.role, 'user');
    assert.equal(user.text, '');
    // The live tail keeps an image's bytes to show it, not a text file's text.
    assert.deepEqual(user.attachments.map((a) => [a.name, a.mimeType, !!a.data]), [['shot.png', 'image/png', true], ['plan.md', 'text/plain', false]]);
    assert.match(done.tail.items.at(-1).text, /You sent 2 files \(shot\.png, plan\.md\)/);

    const page = await mock.getMessages(done.session, -1, 100);
    const saved = page.messages.filter((m) => m.role === 'user').at(-1);
    assert.deepEqual(saved.parts.map((p) => [p.kind, p.file?.name]), [['file', 'shot.png'], ['file', 'plan.md']]);
    store.reconcile(chat.chatId, page.messages);
    assert.deepEqual(store.byChatId(chat.chatId).tail.items, []);
  });
});

test('a refused prompt with files starts no turn and leaves no stale prompt', async () => {
  const { mockAttachFiles } = await import('../src/lib/mock/mockChat.ts');
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    const many = mockAttachFiles(Array.from({ length: 12 }, (_, i) => `/p/f${i}.txt`));
    assert.equal(many.attachments.length, 10);
    assert.deepEqual(many.skipped.map((s) => s.name), ['f10.txt', 'f11.txt']);
    await assert.rejects(store.prompt(chat.chatId, '', [...many.attachments, ...many.attachments.slice(0, 1)]), /at most 10/);
    assert.equal(store.byChatId(chat.chatId).phase, 'idle');
    // The next prompt's turn shows its own text, not the refused files.
    await store.prompt(chat.chatId, 'just text');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const users = store.byChatId(chat.chatId).tail.items.filter((i) => i.role === 'user');
    assert.deepEqual(users.map((u) => [u.text, u.attachments]), [['just text', undefined]]);
  });
});

test('the mock prepares pasted images and refuses what is not one', async () => {
  const { mockAttachFiles, mockAttachImage, MOCK_IMAGE_PNG } = await import('../src/lib/mock/mockChat.ts');
  const pasted = mockAttachImage('', MOCK_IMAGE_PNG);
  assert.deepEqual([pasted.kind, pasted.name, pasted.mimeType], ['image', 'Pasted image', 'image/png']);
  assert.throws(() => mockAttachImage('clip', btoa('plain text')), /clip/);
  assert.deepEqual(mockAttachFiles(['/p/report.pdf']).skipped.map((s) => s.name), ['report.pdf']);
});

const pendingQuestion = async (store, chatId) => {
  await until('question', () => store.byChatId(chatId).questions.length === 1);
  return store.byChatId(chatId).questions[0];
};

test("a question waits as a card; the answer goes back and leaves a summary in the tail", async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR, questions: true });
    await store.prompt(chat.chatId, 'ask me a question');
    const q = await pendingQuestion(store, chat.chatId);
    assert.equal(q.message, 'Please answer the following questions.');
    assert.equal(store.byChatId(chat.chatId).phase, 'working');

    // An answer the form does not have is refused, and the card stays.
    await assert.rejects(store.answer(chat.chatId, q.id, 'accept', { question_0: 'Something else' }), /not one of the options/);
    assert.equal(store.byChatId(chat.chatId).questions.length, 1);

    await store.answer(chat.chatId, q.id, 'accept', { question_0: 'New module', question_1: ['Unit tests', 'Documentation'], question_2_custom: 'Only after a backup' });
    assert.deepEqual(store.byChatId(chat.chatId).questions, []);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const done = store.byChatId(chat.chatId);
    const line = done.tail.items.find((i) => i.type === 'answer');
    assert.equal(line.summary, 'Answered: New module · Unit tests, Documentation · Only after a backup');
    assert.match(done.tail.items.findLast((i) => i.type === 'text').text, /You answered \{"question_0":"New module"/);
    await assert.rejects(store.answer(chat.chatId, q.id, 'decline'), /no longer pending/);

    const page = await mock.getMessages(done.session, -1, 100);
    store.reconcile(chat.chatId, page.messages);
    assert.deepEqual(store.byChatId(chat.chatId).tail.items, [], 'the saved turn shows the answer');
  });
});

test("Codex's required questions are refused unanswered; Skip declines", async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'codex', dir: DIR, questions: true });
    await store.prompt(chat.chatId, 'question');
    const q = await pendingQuestion(store, chat.chatId);
    assert.deepEqual(q.required, ['approach', 'confirm']);
    await assert.rejects(store.answer(chat.chatId, q.id, 'accept', { approach_note: 'Mine' }), /needs an answer/);
    await store.answer(chat.chatId, q.id, 'decline');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const c = store.byChatId(chat.chatId);
    assert.equal(c.tail.items.find((i) => i.type === 'answer').summary, 'Skipped the question');
    assert.equal(c.stopReason, 'end_turn');
  });
});

test('a withdrawn question says the agent went on, and stays late until dismissed', async () => {
  await withChats(async ({ store, mock }) => {
    mock.chat.withdrawMs = 20;
    const chat = await store.start({ agent: 'codex', dir: DIR, questions: true });
    await store.prompt(chat.chatId, 'withdraw the question');
    const q = await pendingQuestion(store, chat.chatId);
    assert.deepEqual(q.meta, { codex: { autoResolutionMs: 20 } });
    assert.equal(q.late, undefined);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const c = store.byChatId(chat.chatId);
    assert.deepEqual(c.questions, [{ ...q, late: true }], 'the card stays, late');
    assert.equal(c.tail.items.find((i) => i.type === 'answer').summary, 'The agent went on without an answer');
    await assert.rejects(store.answer(chat.chatId, q.id, 'accept', {}), /no longer pending/);
    assert.equal(store.byChatId(chat.chatId).questions.length, 1, 'a refused answer keeps the late card');

    store.dismissLate(chat.chatId, q.id);
    assert.deepEqual(store.byChatId(chat.chatId).questions, []);
  });
});

test("Codex's async question outlives the streamed reply, goes late at the turn's end, and its answers go as a message", async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'codex', dir: DIR, questions: true });
    await store.prompt(chat.chatId, 'ask me an async question');
    const q = await pendingQuestion(store, chat.chatId);
    assert.deepEqual(q.meta, { codex: { delivery: 'async' } });
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    let c = store.byChatId(chat.chatId);
    assert.match(c.tail.items.findLast((i) => i.type === 'text').text, /keep working while you answer.*No answer came before I finished\./s);
    assert.deepEqual(c.questions.map((x) => [x.id, x.late]), [[q.id, true]], 'the end of the turn does not take the card');

    await store.sendLate(chat.chatId, q.id, 'My answer to your question:\n- Which implementation approach should I take? — New module');
    assert.deepEqual(store.byChatId(chat.chatId).questions, [], 'a sent late answer drops its card');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    c = store.byChatId(chat.chatId);
    assert.match(c.tail.items.findLast((i) => i.role === 'user').text, /^My answer to your question:\n- Which implementation approach should I take\? — New module$/);
  });
});

test('a question still open at turn_end stays until its resolution, then goes late', async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'codex', dir: DIR, questions: true });
    // The agent's events as the backend sends them: Codex's adapter may end
    // the prompt before it withdraws an async question.
    const agent = mock.chat.chats.get(chat.chatId);
    const send = (fields) => mock.chat.update(agent, fields);
    const q = { id: 'question-9', message: 'Codex keeps working while you answer.', fields: [{ key: 'a', type: 'string', title: 'Why?' }], meta: { codex: { delivery: 'async' } } };
    send({ kind: 'turn_start' });
    send({ kind: 'question', question: q });
    send({ kind: 'turn_end', stopReason: 'end_turn' });
    let c = store.byChatId(chat.chatId);
    assert.equal(c.phase, 'idle');
    assert.deepEqual(c.questions, [q], 'the end of the turn does not take a pending question');

    send({ kind: 'question_resolved', question: { id: q.id, outcome: 'withdrawn' } });
    c = store.byChatId(chat.chatId);
    assert.deepEqual(c.questions, [{ ...q, late: true }]);
    assert.equal(c.tail.items.find((i) => i.type === 'answer').summary, 'The agent went on without an answer');

    send({ kind: 'turn_start' });
    send({ kind: 'turn_end', stopReason: 'end_turn' });
    assert.deepEqual(store.byChatId(chat.chatId).questions, [{ ...q, late: true }], 'a late card stays across turns');

    agent && mock.chat.close(chat.chatId);
    await until('exit', () => store.byChatId(chat.chatId).phase === 'exited');
    assert.deepEqual(store.byChatId(chat.chatId).questions, [], 'the chat ending clears every card');
  });
});

test('cancelling a turn takes its question with it, without a summary', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'opencode', dir: DIR, questions: true });
    await store.prompt(chat.chatId, 'question');
    const q = await pendingQuestion(store, chat.chatId);
    assert.deepEqual(q.fields.map((f) => f.key), ['approach', 'extras', 'extras_custom', 'overwrite']);
    await store.cancel(chat.chatId);
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const c = store.byChatId(chat.chatId);
    assert.deepEqual(c.questions, []);
    assert.equal(c.stopReason, 'cancelled');
    assert.equal(c.tail.items.some((i) => i.type === 'answer'), false);
  });
});

test('a chat that does not show questions is never asked one', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    await store.prompt(chat.chatId, 'question');
    await until('turn end', () => store.byChatId(chat.chatId).phase === 'idle');
    const c = store.byChatId(chat.chatId);
    assert.deepEqual(c.questions, []);
    assert.match(c.tail.items.findLast((i) => i.type === 'text').text, /does not show questions/);
  });
});

test('a reloaded page shows the question still waiting, and its answer leaves the summary', async () => {
  await withChats(
    async ({ store, mock }) => {
      const st = await mock.chatStart({ agent: 'opencode', dir: DIR, questions: true });
      await mock.chatPrompt(st.chatId, 'question');
      for (let i = 0; i < 5000 && !(await mock.chatList())[0]?.questions?.length; i++) await new Promise((resolve) => setTimeout(resolve, 1));
      await store.init();
      const [q] = store.byChatId(st.chatId).questions;
      assert.equal(q.id, 'question-1');
      await store.answer(st.chatId, q.id, 'accept', { approach: 'New module', overwrite: true });
      await until('turn end', () => store.byChatId(st.chatId).phase === 'idle');
      assert.equal(store.byChatId(st.chatId).tail.items.find((i) => i.type === 'answer').summary, 'Answered: New module · Yes');
    },
    { init: false },
  );
});

test('closing the chat clears its question', async () => {
  await withChats(async ({ store }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR, questions: true });
    await store.prompt(chat.chatId, 'question');
    await pendingQuestion(store, chat.chatId);
    await store.close(chat.chatId);
    await until('exit', () => store.byChatId(chat.chatId).phase === 'exited');
    assert.deepEqual(store.byChatId(chat.chatId).questions, []);
  });
});

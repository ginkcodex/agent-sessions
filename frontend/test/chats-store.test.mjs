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
    await assert.rejects(store.start({ agent: 'codex', dir: DIR }), /codex-acp was not found/);
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

test('chats refuse a connected host', async () => {
  await withChats(async ({ store, mock }) => {
    const chat = await store.start({ agent: 'claude-code', dir: DIR });
    await mock.connect('dev-box');
    // Connecting ended the chat.
    await until('exit', () => store.byChatId(chat.chatId)?.phase === 'exited');
    await assert.rejects(store.start({ agent: 'claude-code', dir: DIR }), /this computer's sessions only/);
    await assert.rejects(store.loadAgents(), /this computer's sessions only/);
    assert.match(store.agentsError, /this computer's sessions only/);
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

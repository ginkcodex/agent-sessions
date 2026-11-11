// Live tail against saved transcripts, for each agent: the turns in
// fixtures/acp-agents.json follow the ACP-02 spike's IDs, update order and
// saved records. After each turn the transcript must show every message once.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { applySessionUpdate, beginTurn, emptyTail, endTurn, reconcile } from '../src/lib/chatTail.ts';

const { agents } = JSON.parse(readFileSync(new URL('./fixtures/acp-agents.json', import.meta.url), 'utf8'));

const keys = (tail) => tail.items.map((i) => (i.type === 'tool' ? i.toolCallId : i.type === 'text' && i.role === 'user' ? `prompt:${i.text}` : `${i.type}:${i.messageId}`));
const ids = (tail) => tail.items.map((i) => (i.type === 'tool' ? i.toolCallId : i.messageId));

for (const { agent, turns } of agents) {
  test(`${agent}: the live tail gives way to the saved transcript, turn by turn`, () => {
    let tail = emptyTail();
    let saved = [];
    for (const t of turns) {
      tail = beginTurn(tail, { text: t.prompt, sentAt: Date.parse(t.sentAt) });
      tail = t.updates.reduce(applySessionUpdate, tail);
      assert.ok(tail.items.length > 0, `${t.label}: the turn streams`);

      // What the transcript shows before the agent saves this turn changes nothing for it.
      const before = reconcile(tail, saved);
      assert.ok(before.items.some((i) => i.turn === tail.turn && i.type === 'text' && i.role === 'user'), `${t.label}: the prompt shows until saved`);

      if (t.during) {
        const during = reconcile(tail, [...saved, ...t.during]);
        assert.deepEqual(ids(during), t.duringKeeps, `${t.label}: while the turn runs, only what is saved goes`);
        tail = during;
      }

      tail = endTurn(tail);
      saved = [...saved, ...t.saved];
      tail = reconcile(tail, saved);
      assert.deepEqual(ids(tail), t.savedKeeps ?? [], `${t.label}: after the turn, keys left ${JSON.stringify(keys(tail))}`);
      // Reconciling again changes nothing.
      assert.equal(reconcile(tail, saved), tail);
    }
  });
}

test("Codex: a running command stays until its turn's last message is saved, as its saved ID differs", () => {
  const codex = agents.find((a) => a.agent === 'codex').turns[0];
  let tail = beginTurn(emptyTail(), { text: codex.prompt, sentAt: Date.parse(codex.sentAt) });
  tail = codex.updates.reduce(applySessionUpdate, tail);
  tail = endTurn(tail);
  // The whole turn saved except the last message: the command stays.
  const partial = codex.saved.slice(0, 2);
  assert.deepEqual(ids(reconcile(tail, partial)), [codex.updates[2].toolCallId, codex.updates[4].messageId]);
  assert.deepEqual(reconcile(tail, codex.saved).items, []);
});

test('a failed prompt that is never saved goes once a later turn is saved', () => {
  let tail = beginTurn(emptyTail(), { text: 'first try', sentAt: Date.parse('2026-11-11T10:00:00Z') });
  tail = endTurn(tail); // The turn failed: the agent saved nothing.
  assert.deepEqual(keys(reconcile(tail, [])), ['prompt:first try']);
  tail = beginTurn(tail, { text: 'second try', sentAt: Date.parse('2026-11-11T10:01:00Z') });
  tail = applySessionUpdate(tail, { sessionUpdate: 'agent_message_chunk', messageId: 'msg_2', content: { type: 'text', text: 'Working' } });
  const saved = [{ id: 'msg_2', role: 'assistant', time: '2026-11-11T10:01:02Z', parts: [{ kind: 'text', text: 'Working' }] }];
  // The second turn runs; its message is saved but still streams.
  assert.deepEqual(keys(reconcile(tail, saved)), ['prompt:second try', 'text:msg_2']);
});

test('a replayed prompt whose ID the saved record lacks matches by its text', () => {
  // Claude's load replay sends user_message_chunk with a UUID; the saved prompt has no ID.
  let tail = beginTurn(emptyTail());
  tail = applySessionUpdate(tail, { sessionUpdate: 'user_message_chunk', messageId: 'f6575afa-c18e-4f83-b516-3186e944faa1', content: { type: 'text', text: 'Say hi' } });
  assert.deepEqual(reconcile(tail, [{ id: '', role: 'user', time: '2026-11-11T10:00:00Z', parts: [{ kind: 'text', text: 'Say hi' }] }]).items, []);
});

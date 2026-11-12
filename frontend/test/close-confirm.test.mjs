import test from 'node:test';
import assert from 'node:assert/strict';
import { confirmClosingWindow, closeWindowWarning, endingChatsWarning, unsavedDrafts } from '../src/lib/closeConfirm.ts';

const shell = (label, host = '') => ({ label, host, kind: 'shell', status: 'running' });
const agent = (label, host = '', id) => ({ label, host, kind: 'agent', agent: id, status: 'running' });
const row = (name, fields = {}) => ({ name, detail: undefined, icon: 'terminal', agent: undefined, ...fields });

test('no warning without running terminals', () => {
  assert.equal(closeWindowWarning([]), null);
  assert.equal(closeWindowWarning([{ ...agent('Claude Code'), status: 'exited' }]), null);
});

test('the warning counts the terminals and names them in a Terminals group', () => {
  assert.deepEqual(closeWindowWarning([shell('zsh')]), {
    message: 'A terminal is still running. Closing the window ends it:',
    groups: [{ title: 'Terminals', icon: 'terminal', items: [row('zsh')] }],
  });
  assert.deepEqual(closeWindowWarning([agent('Claude Code', '', 'claude-code')]), {
    message: 'A coding agent is still running. Closing the window ends it:',
    groups: [{ title: 'Terminals', icon: 'terminal', items: [row('Claude Code', { icon: 'agent', agent: 'claude-code' })] }],
  });
  const mixed = closeWindowWarning([agent('Claude Code', '', 'claude-code'), shell('zsh'), agent('Codex', 'wsl:Ubuntu', 'codex')]);
  assert.equal(mixed.message, '3 terminals are still running, including 2 coding agents. Closing the window ends them:');
  assert.deepEqual(mixed.groups, [{
    title: 'Terminals',
    icon: 'terminal',
    items: [
      row('Claude Code', { icon: 'agent', agent: 'claude-code' }),
      row('zsh'),
      row('Codex', { detail: 'on Ubuntu (WSL)', icon: 'agent', agent: 'codex' }),
    ],
  }]);
});

test('a group names six items and sums up the rest', () => {
  const many = closeWindowWarning(Array.from({ length: 8 }, (_, i) => shell(`sh${i}`)));
  assert.equal(many.groups[0].items.length, 6);
  assert.equal(many.groups[0].items.at(-1).name, 'sh5');
  assert.equal(many.groups[0].more, 2);
});

test('closing asks only while terminals run, and only Close window lets it go', async () => {
  const asked = [];
  const ask = answer => async (title, message, choices, groups) => { asked.push({ title, message, choices, groups }); return answer; };
  assert.equal(await confirmClosingWindow([], [], ask('close')), true);
  assert.equal(asked.length, 0);
  assert.equal(await confirmClosingWindow([agent('Claude Code')], [], ask(null)), false);
  assert.equal(await confirmClosingWindow([agent('Claude Code')], [], ask('close')), true);
  assert.equal(asked[0].title, 'Close window?');
  assert.deepEqual(asked[0].choices, [{ value: 'close', label: 'Close window', danger: true }]);
  assert.deepEqual(asked[0].groups.map(g => g.title), ['Terminals'], 'the cards ride along with the question');
});

const chat = (agent, dir, phase = 'idle') => ({ agent, dir, phase });
const chatRow = (name, agent, fields = {}) => ({ name, detail: undefined, icon: 'chat', agent, badge: undefined, ...fields });

test('open chats are named in a Chats group, working ones badged; ended ones are not', () => {
  assert.equal(closeWindowWarning([], [chat('claude-code', '/home/u/agent-sessions', 'exited')]), null);
  assert.deepEqual(closeWindowWarning([], [chat('claude-code', '/home/u/agent-sessions')]), {
    message: 'A chat is open. Closing the window ends it:',
    groups: [{ title: 'Chats', icon: 'chat', items: [chatRow('Claude Code', 'claude-code', { detail: 'in agent-sessions' })] }],
  });
  assert.deepEqual(closeWindowWarning([], [chat('codex', 'C:\\work\\api', 'working')]), {
    message: 'A chat is open and its agent is working. Closing the window ends it:',
    groups: [{ title: 'Chats', icon: 'chat', items: [chatRow('Codex', 'codex', { detail: 'in api', badge: 'working' })] }],
  });
  const mixed = closeWindowWarning([shell('zsh')], [chat('opencode', '/p', 'working'), chat('codex', '/q', 'starting')]);
  assert.equal(mixed.message, 'A terminal is still running. 2 chats are open, 1 of them working. Closing the window ends them:');
  assert.deepEqual(mixed.groups, [
    { title: 'Terminals', icon: 'terminal', items: [row('zsh')] },
    { title: 'Chats', icon: 'chat', items: [chatRow('OpenCode', 'opencode', { detail: 'in p', badge: 'working' }), chatRow('Codex', 'codex', { detail: 'in q' })] },
  ], 'each kind gets its own card');
  const many = closeWindowWarning([], Array.from({ length: 8 }, (_, i) => chat('codex', `/d${i}`)));
  assert.equal(many.groups.length, 1, 'no empty Terminals card');
  assert.equal(many.groups[0].items.at(-1).name, 'Codex');
  assert.equal(many.groups[0].more, 2);
});

test('closing asks while a chat is open, even with no terminal running', async () => {
  let asked = 0;
  const ask = async () => { asked++; return null; };
  assert.equal(await confirmClosingWindow([], [chat('codex', '/p')], ask), false);
  assert.equal(asked, 1);
  assert.equal(await confirmClosingWindow([], [chat('codex', '/p', 'exited')], ask), true);
  assert.equal(asked, 1);
});

test('a host switch names the open chats it ends; ended ones are not', () => {
  assert.equal(endingChatsWarning([], 'Connecting to box'), null);
  assert.equal(endingChatsWarning([chat('codex', '/p', 'exited')], 'Connecting to box'), null);
  assert.deepEqual(endingChatsWarning([chat('claude-code', '/home/u/agent-sessions', 'working')], 'Disconnecting from box'), {
    message: 'A chat is open and its agent is working. Disconnecting from box ends it:',
    groups: [{ title: 'Chats', icon: 'chat', items: [chatRow('Claude Code', 'claude-code', { detail: 'in agent-sessions', badge: 'working' })] }],
  });
  const two = endingChatsWarning([chat('codex', '/a'), chat('opencode', '/b', 'starting'), chat('codex', '/c', 'exited')], 'Connecting to box');
  assert.equal(two.message, '2 chats are open. Connecting to box ends them:');
  assert.deepEqual(two.groups, [{ title: 'Chats', icon: 'chat', items: [chatRow('Codex', 'codex', { detail: 'in a' }), chatRow('OpenCode', 'opencode', { detail: 'in b' })] }]);
  const many = endingChatsWarning(Array.from({ length: 8 }, (_, i) => chat('codex', `/d${i}`)), 'Connecting to box');
  assert.match(many.message, /^8 chats are open\. /);
  assert.equal(many.groups[0].more, 2);
});

test('the unsaved-changes question names every dirty file in its own card', () => {
  const dirty = [{ dir: '/p', tab: { path: 'src/a.ts' } }, { dir: 'C:\\work\\api', tab: { path: 'b.md' } }];
  assert.deepEqual(unsavedDrafts(dirty), {
    message: 'Unsaved changes in:',
    groups: [{
      title: 'Unsaved files',
      icon: 'file',
      items: [
        { name: 'src/a.ts', detail: 'in /p', icon: 'file' },
        { name: 'b.md', detail: 'in C:\\work\\api', icon: 'file' },
      ],
    }],
  });
  const many = unsavedDrafts(Array.from({ length: 8 }, (_, i) => ({ dir: '/p', tab: { path: `f${i}` } })));
  assert.equal(many.groups[0].items.length, 8, 'every file is named; the save or discard decision is taken file by file');
  assert.equal(many.groups[0].more, undefined);
});

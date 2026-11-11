import test from 'node:test';
import assert from 'node:assert/strict';
import { confirmClosingWindow, closeWindowWarning } from '../src/lib/closeConfirm.ts';

const shell = (label, host = '') => ({ label, host, kind: 'shell', status: 'running' });
const agent = (label, host = '') => ({ label, host, kind: 'agent', status: 'running' });

test('no warning without running terminals', () => {
  assert.equal(closeWindowWarning([]), null);
  assert.equal(closeWindowWarning([{ ...agent('Claude Code'), status: 'exited' }]), null);
});

test('the warning counts the terminals and coding agents and names them', () => {
  assert.equal(closeWindowWarning([shell('zsh')]),
    'A terminal is still running. Closing the window ends it:\nzsh');
  assert.equal(closeWindowWarning([agent('Claude Code')]),
    'A coding agent is still running. Closing the window ends it:\nClaude Code');
  assert.equal(closeWindowWarning([agent('Claude Code'), shell('zsh'), agent('Codex', 'wsl:Ubuntu')]),
    '3 terminals are still running, including 2 coding agents. Closing the window ends them:\nClaude Code\nzsh\nCodex on Ubuntu (WSL)');
  const many = Array.from({ length: 8 }, (_, i) => shell(`sh${i}`));
  assert.match(closeWindowWarning(many), /sh5\nand 2 more$/);
});

test('closing asks only while terminals run, and only Close window lets it go', async () => {
  const asked = [];
  const ask = answer => async (title, message, choices) => { asked.push({ title, message, choices }); return answer; };
  assert.equal(await confirmClosingWindow([], [], ask('close')), true);
  assert.equal(asked.length, 0);
  assert.equal(await confirmClosingWindow([agent('Claude Code')], [], ask(null)), false);
  assert.equal(await confirmClosingWindow([agent('Claude Code')], [], ask('close')), true);
  assert.equal(asked[0].title, 'Close window?');
  assert.deepEqual(asked[0].choices, [{ value: 'close', label: 'Close window', danger: true }]);
});

const chat = (agent, dir, phase = 'idle') => ({ agent, dir, phase });

test('open chats are named in the warning, working ones said so; ended ones are not', () => {
  assert.equal(closeWindowWarning([], [chat('claude-code', '/home/u/agent-sessions', 'exited')]), null);
  assert.equal(closeWindowWarning([], [chat('claude-code', '/home/u/agent-sessions')]),
    'A chat is open. Closing the window ends it:\nChat with Claude Code in agent-sessions');
  assert.equal(closeWindowWarning([], [chat('codex', 'C:\\work\\api', 'working')]),
    'A chat is open and its agent is working. Closing the window ends it:\nChat with Codex in api (working)');
  assert.equal(closeWindowWarning([shell('zsh')], [chat('opencode', '/p', 'working'), chat('codex', '/q', 'starting')]),
    'A terminal is still running. 2 chats are open, 1 of them working. Closing the window ends them:\nzsh\nChat with OpenCode in p (working)\nChat with Codex in q');
  const many = closeWindowWarning(Array.from({ length: 5 }, (_, i) => shell(`sh${i}`)), [chat('codex', '/a'), chat('codex', '/b')]);
  assert.match(many, /sh4\nChat with Codex in a\nand 1 more$/);
});

test('closing asks while a chat is open, even with no terminal running', async () => {
  let asked = 0;
  const ask = async () => { asked++; return null; };
  assert.equal(await confirmClosingWindow([], [chat('codex', '/p')], ask), false);
  assert.equal(asked, 1);
  assert.equal(await confirmClosingWindow([], [chat('codex', '/p', 'exited')], ask), true);
  assert.equal(asked, 1);
});

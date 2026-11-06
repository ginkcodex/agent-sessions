import test from 'node:test';
import assert from 'node:assert/strict';
import { confirmEndingTerminals, runningTerminalsWarning } from '../src/lib/closeConfirm.ts';

const shell = (label, host = '') => ({ label, host, kind: 'shell', status: 'running' });
const agent = (label, host = '') => ({ label, host, kind: 'agent', status: 'running' });

test('no warning without running terminals', () => {
  assert.equal(runningTerminalsWarning([]), null);
  assert.equal(runningTerminalsWarning([{ ...agent('Claude Code'), status: 'exited' }]), null);
});

test('the warning counts the terminals and coding agents and names them', () => {
  assert.equal(runningTerminalsWarning([shell('zsh')]),
    'A terminal is still running. Closing the window ends it:\nzsh');
  assert.equal(runningTerminalsWarning([agent('Claude Code')]),
    'A coding agent is still running. Closing the window ends it:\nClaude Code');
  assert.equal(runningTerminalsWarning([agent('Claude Code'), shell('zsh'), agent('Codex', 'wsl:Ubuntu')]),
    '3 terminals are still running, including 2 coding agents. Closing the window ends them:\nClaude Code\nzsh\nCodex on Ubuntu (WSL)');
  const many = Array.from({ length: 8 }, (_, i) => shell(`sh${i}`));
  assert.match(runningTerminalsWarning(many), /sh5\nand 2 more$/);
});

test('closing asks only while terminals run, and only Close window lets it go', async () => {
  const asked = [];
  const ask = answer => async (title, message, choices) => { asked.push({ title, message, choices }); return answer; };
  assert.equal(await confirmEndingTerminals([], ask('close')), true);
  assert.equal(asked.length, 0);
  assert.equal(await confirmEndingTerminals([agent('Claude Code')], ask(null)), false);
  assert.equal(await confirmEndingTerminals([agent('Claude Code')], ask('close')), true);
  assert.equal(asked[0].title, 'Close window?');
  assert.deepEqual(asked[0].choices, [{ value: 'close', label: 'Close window', danger: true }]);
});

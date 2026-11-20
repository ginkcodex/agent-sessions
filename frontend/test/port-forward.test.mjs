import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { validatePortForward } from '../src/lib/portForward.ts';
import { MockBackendAPI } from '../src/lib/mock/mockApi.ts';

const rule = { remoteHost: 'localhost', remotePort: 3000, localPort: 3000, autoStart: false };

test('port forwarding validates port ranges and remote host input', () => {
  assert.equal(validatePortForward(rule), null);
  assert.equal(validatePortForward({ ...rule, remoteHost: '::1' }), null);
  for (const remotePort of [0, 65536, 1.5, NaN]) assert.ok(validatePortForward({ ...rule, remotePort }));
  assert.ok(validatePortForward({ ...rule, remoteHost: 'bad host' }));
});

test('mock forwarding rules are isolated per SSH host and support lifecycle actions', async () => {
  const mock = new MockBackendAPI();
  await mock.connect('dev-box');
  const saved = await mock.savePortForward(rule);
  const id = saved.rules[0].id;
  assert.equal(saved.rules[0].status, 'stopped');
  assert.equal((await mock.startPortForward(id)).rules[0].status, 'active');
  await assert.rejects(mock.savePortForward(rule), /already configured/);
  assert.equal((await mock.stopPortForward(id)).rules[0].status, 'stopped');
  await mock.connect('other-host');
  assert.equal((await mock.listPortForwards()).rules.length, 0);
  await mock.connect('dev-box');
  assert.equal((await mock.listPortForwards()).rules[0].id, id);
  assert.equal((await mock.removePortForward(id)).rules.length, 0);
});

test('compact rule rows keep a named, titled control for every action', () => {
  const source = readFileSync(new URL('../src/lib/components/common/PortForwardDialog.svelte', import.meta.url), 'utf8');
  const buttons = source.match(/<button[^>]*class="icon-btn[^>]*>/g) ?? [];
  assert.equal(buttons.length, 5);
  for (const button of buttons) assert.match(button, /aria-label="[^"]+".*title=/);
  for (const name of ['external-link', 'copy', 'pencil', 'trash-2']) assert.match(source, new RegExp(`<Icon name="${name}" />`));
});

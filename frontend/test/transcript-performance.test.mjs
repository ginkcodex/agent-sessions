import test from 'node:test';
import assert from 'node:assert/strict';
import { render } from 'svelte/server';
import { loadServerComponent } from './helpers.mjs';

const base = new URL('../src/lib/components/transcript/parts/', import.meta.url);
const reasoning = await loadServerComponent(new URL('ReasoningPart.svelte', base), [
  ["import { renderMarkdown } from '../../../markdown';", 'const renderMarkdown = text => { globalThis.__renderCalls++; return `<p>${text}</p>`; };'],
  ["import { handleMessageClick } from '../../../messageFileClick';", 'const handleMessageClick = () => {};'],
  ["import { renderMermaid } from '../../../mermaid';", 'const renderMermaid = () => {};'],
  ["import { theme } from '../../../stores/theme.svelte';", "const theme = { resolved: 'dark' };"],
]);
const tool = await loadServerComponent(new URL('ToolPart.svelte', base), [
  ["import { highlightCode } from '../../../highlight';", 'const highlightCode = text => { globalThis.__renderCalls++; return text; };'],
  ["'../../../layout'", JSON.stringify(new URL('../src/lib/layout.ts', import.meta.url).href)],
  ["import { api } from '../../../api';", 'const api = {};'],
  ["import SubagentLink from './SubagentLink.svelte';", 'const SubagentLink = () => {};'],
]);

for (const [name, component, props, content] of [
  ['reasoning', reasoning.default, { text: 'thinking-body-fixture' }, 'thinking-body-fixture'],
  ['parameters', tool.default, { tool: { name: 'Read', input: 'parameter-body-fixture', status: 'completed' }, sessionRef: { agent: 'claude', id: 'test' } }, 'parameter-body-fixture'],
]) {
  test(`collapsed ${name} skips parsing and hidden body markup`, () => {
    globalThis.__renderCalls = 0;
    const html = render(component, { props }).body;
    assert.equal(globalThis.__renderCalls, 0);
    assert.ok(!html.includes(content));
    delete globalThis.__renderCalls;
  });
  test(`search-expanded ${name} renders before a toggle event`, () => {
    globalThis.__renderCalls = 0;
    const html = render(component, { props: { ...props, defaultExpanded: true } }).body;
    assert.equal(globalThis.__renderCalls, 1);
    assert.ok(html.includes(content));
    assert.match(html, /<details[^>]* open/);
    delete globalThis.__renderCalls;
  });
}

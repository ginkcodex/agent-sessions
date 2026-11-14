import test from 'node:test';
import assert from 'node:assert/strict';
import { readableToolOutput, toolOutputPreview, toolInputSummary, toolFileTarget } from '../src/lib/toolPresentation.ts';
import { render } from 'svelte/server';
import { loadServerComponent } from './helpers.mjs';

const raw = JSON.stringify([{ type: 'input_text', text: 'Script completed\nOutput:' }, { type: 'input_text', text: 'first\nsecond' }]);
test('decodes Codex text blocks and standard text envelopes', () => {
  assert.equal(readableToolOutput(raw), 'Script completed\nOutput:\n\nfirst\nsecond');
  assert.equal(readableToolOutput(JSON.stringify({ content: [{ type: 'text', text: 'a\nb' }] })), 'a\nb');
  assert.equal(readableToolOutput('[{"type":"output_text","text":"ok"}]'), 'ok');
});
test('preserves plain escapes, malformed JSON and mixed/unknown data', () => {
  for (const value of ['path\\new\\test', '{broken', 'Exit code 2\nfailed']) assert.equal(readableToolOutput(value), value);
  for (const value of ['{"items":[1]}', '[{"type":"text","text":"ok"},{"type":"image","data":"keep"}]', '[]',
    '{"id":9223372036854775807}', '{"content":[{"type":"text","text":"ok"}],"status":"failed","exitCode":127}',
    '[{"type":"text","text":"ok","structuredContent":{"status":"failed"}}]']) {
    assert.equal(readableToolOutput(value), value);
  }
});
test('bounds previews by both lines and characters', () => {
  assert.deepEqual(toolOutputPreview('short\noutput'), { text: 'short\noutput', shortened: false });
  assert.equal(toolOutputPreview('x'.repeat(10000)).text.length, 1200);
  const preview = toolOutputPreview(Array.from({ length: 50 }, (_, i) => `line ${i}`).join('\n'));
  assert.equal(preview.text.split('\n').length, 8);
  assert.equal(preview.shortened, true);
  assert.equal(toolOutputPreview('x'.repeat(1199) + '😀tail').text, 'x'.repeat(1199));
});
test('summaries show command/path arguments, not hidden arbitrary parameters', () => {
  assert.equal(toolInputSummary({ command: 'git\nstatus' }), 'git status');
  assert.equal(toolInputSummary('{"filePath":"src/main.go"}'), 'src/main.go');
  assert.equal(toolInputSummary({ query: 'needle' }), 'needle');
  assert.equal(toolInputSummary('unstructured hidden parameters'), '');
  assert.equal(toolInputSummary(null), '');
});

const tool = await loadServerComponent(new URL('../src/lib/components/transcript/parts/ToolPart.svelte', import.meta.url), [
  ["import { highlightCode } from '../../../highlight';", 'const highlightCode = text => text;'],
  ["'../../../toolPresentation'", JSON.stringify(new URL('../src/lib/toolPresentation.ts', import.meta.url).href)],
  ["import { api } from '../../../api';", 'const api = {};'],
  ["import SubagentLink from './SubagentLink.svelte';", 'const SubagentLink = () => {};'],
]);
const props = { tool: { id: 'opaque-call-id', name: 'exec', status: 'completed', input: { command: 'git status' }, output: raw }, sessionRef: { agent: 'codex', id: 'test' } };
test('tool markup renders readable text and a useful summary', () => {
  const html = render(tool.default, { props }).body;
  assert.ok(html.includes('first\nsecond'));
  assert.ok(html.includes('git status'));
  assert.ok(html.includes('title="exec\nopaque-call-id"'));
  assert.ok(!html.includes('input_text'));
  assert.ok(!html.includes('Show more output'));
});
test('long outputs are bounded; search expansion renders the landing text', () => {
  const output = Array.from({ length: 50 }, (_, i) => `line ${i}`).join('\n');
  const longProps = { ...props, tool: { ...props.tool, output, outputTruncated: true, outputRef: 'blob' } };
  const html = render(tool.default, { props: longProps }).body;
  assert.ok(html.includes('Show more output'));
  assert.ok(!html.includes('line 49'));
  assert.ok(html.includes('Load full output'));
  const expanded = render(tool.default, { props: { ...longProps, defaultExpanded: true } }).body;
  assert.ok(expanded.includes('line 49'));
  assert.ok(!expanded.includes('Show more output'));
});

test('file tools name their file; other tools and directories do not', () => {
  // Claude Code, OpenCode, notebooks and JSON-string input.
  assert.deepEqual(toolFileTarget('Edit', { file_path: '/w/docs/plan.md', old_string: 'a' }), { path: '/w/docs/plan.md' });
  assert.deepEqual(toolFileTarget('write', { filePath: 'src/main.go' }), { path: 'src/main.go' });
  assert.deepEqual(toolFileTarget('NotebookEdit', { notebook_path: 'a.ipynb' }), { path: 'a.ipynb' });
  assert.deepEqual(toolFileTarget('Write', '{"file_path":"b.ts"}'), { path: 'b.ts' });
  // Any tool whose input names a file_path, such as an ACP title.
  assert.deepEqual(toolFileTarget('Read main.go', { file_path: 'main.go' }), { path: 'main.go' });
  // A generic path is a file only for file tools.
  assert.deepEqual(toolFileTarget('read', { path: 'a.txt' }), { path: 'a.txt' });
  assert.equal(toolFileTarget('Grep', { path: 'src', pattern: 'x' }), null);
  assert.equal(toolFileTarget('Glob', { path: '/w', pattern: '*.go' }), null);
  assert.equal(toolFileTarget('Bash', { command: 'cat a.txt' }), null);
  // Nothing to open.
  for (const input of [undefined, null, 'not json', [], { file_path: '' }, { file_path: '  ' }, { file_path: 3 },
    { file_path: 'docs/' }, { file_path: 'C:\\w\\' }, { file_path: 'a\nb' }]) {
    assert.equal(toolFileTarget('Edit', input), null, JSON.stringify(input));
  }
});

test('Read opens at its offset; the path is never URI-decoded', () => {
  assert.deepEqual(toolFileTarget('Read', { file_path: 'a.go', offset: 120, limit: 40 }), { path: 'a.go', line: 120 });
  for (const offset of [0, -3, 1.5, '12']) {
    assert.deepEqual(toolFileTarget('Read', { file_path: 'a.go', offset }), { path: 'a.go' });
  }
  // OpenCode's read may count from 0, so only Claude Code's Read gets a line.
  assert.deepEqual(toolFileTarget('read', { filePath: 'a.go', offset: 120 }), { path: 'a.go' });
  assert.deepEqual(toolFileTarget('Edit', { file_path: 'docs/100%25 done.md' }), { path: 'docs/100%25 done.md' });
});

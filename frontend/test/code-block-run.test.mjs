import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { runnableCommand } from '../src/lib/runCommand.ts';
import { renderMarkdown } from '../src/lib/markdown.ts';
import { handleMessageClick } from '../src/lib/messageFileClick.ts';
import { fixtureStore, loadServerComponent, renderWithFixture, unescapeHTML } from './helpers.mjs';

test('runnableCommand takes shell blocks only, and nothing empty', () => {
  for (const lang of ['bash', 'sh', 'shell', 'zsh', 'Bash', 'ZSH']) assert.equal(runnableCommand(lang, 'ls -la'), 'ls -la', lang);
  for (const lang of ['', 'text', 'python', 'powershell', 'console', 'go']) assert.equal(runnableCommand(lang, 'ls'), '', lang);
  for (const code of ['', '  \n\t\n', '!', '! ', '$ ', '\x1b\x07']) assert.equal(runnableCommand('bash', code), '', JSON.stringify(code));
});

test('runnableCommand drops control characters, a leading ! and uniform prompts', () => {
  // An ESC could end the bracketed paste and run the rest; C1 and DEL go too.
  assert.equal(runnableCommand('bash', 'echo hi\x1b[201~; rm -rf ~\x07\x7f\x9b'), 'echo hi[201~; rm -rf ~');
  assert.equal(runnableCommand('bash', 'printf "a\\tb"\r\nexit\r\n'), 'printf "a\\tb"\nexit', 'CR goes, tab and LF stay');
  assert.equal(runnableCommand('bash', 'echo\ta'), 'echo\ta');

  // Claude Code's shell-mode prefix, as agents quote it.
  assert.equal(
    runnableCommand('bash', '! cd /tmp/structlayout.eqITmh && go run golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest ./...'),
    'cd /tmp/structlayout.eqITmh && go run golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest ./...',
  );
  assert.equal(runnableCommand('sh', '\n\n!make test\n'), 'make test');
  assert.equal(runnableCommand('sh', 'make && ! grep x f'), 'make && ! grep x f', 'only a leading !');

  // Prompts go only when every line has one; else they may be part of the command.
  assert.equal(runnableCommand('bash', '$ cd app\n\n  $ npm test'), 'cd app\n\nnpm test');
  assert.equal(runnableCommand('bash', '$ ls\nfile.txt'), '$ ls\nfile.txt');
  assert.equal(runnableCommand('bash', 'echo $ HOME'), 'echo $ HOME');
  assert.equal(runnableCommand('bash', '$HOME/bin/tool'), '$HOME/bin/tool');
  assert.equal(runnableCommand('bash', '$ ls\n$\n$ pwd'), 'ls\n\npwd', 'a bare $ line is an empty prompt');
});

test('renderMarkdown puts Run before Copy on shell blocks only when runnable', () => {
  const md = '```bash\n$ go test ./...\n```\n\n```python\nprint(1)\n```\n\n```sh\n\n```\n\n```\nls\n```';
  const html = renderMarkdown(md, 'transcript', undefined, { runnable: true });
  const runs = [...html.matchAll(/<button type="button" class="run-code-btn" data-command="([^"]*)"[^>]*>Run<\/button>/g)];
  assert.deepEqual(runs.map((m) => m[1]), ['go test ./...'], 'python, an empty sh block and a block without a language get none');
  assert.match(
    html,
    /<span class="code-lang-tag">bash<\/span><span class="code-block-actions"><button type="button" class="run-code-btn"[^>]*>Run<\/button><button type="button" class="copy-code-btn" data-code="\$ go test \.\/\.\.\.">Copy<\/button><\/span>/,
    'Run sits left of Copy, which still copies the block as written',
  );
  assert.equal((html.match(/class="copy-code-btn"/g) ?? []).length, 4);

  // Without the option, and by default, the output is as before.
  const plain = renderMarkdown(md, 'transcript');
  assert.equal(renderMarkdown(md, 'transcript', undefined, { runnable: false }), plain);
  assert.doesNotMatch(plain, /run-code-btn|code-block-actions/);

  // Blocks in a quote count; the command is escaped like Copy's code.
  const quoted = renderMarkdown('> ```bash\n> echo "<b>" \'x\' & ls\n> ```', 'transcript', undefined, { runnable: true });
  assert.match(quoted, /data-command="echo &quot;&lt;b&gt;&quot; &#39;x&#39; &amp; ls"/);

  // Raw HTML in the text cannot make a Run button.
  const raw = renderMarkdown('<button class="run-code-btn" data-command="rm -rf ~">Run</button>', 'transcript', undefined, { runnable: true });
  assert.doesNotMatch(raw, /<button/);
});

class FakeElement {
  constructor(matches = {}, inside = true) {
    this.matches = matches;
    this.inside = inside;
  }
  closest(selector) {
    return this.matches[selector] ?? null;
  }
  contains() {
    return this.inside;
  }
}

test('a Run click calls the owner with the command and goes no further', () => {
  const previous = globalThis.Element;
  globalThis.Element = FakeElement;
  try {
    const button = { dataset: { command: 'make test' } };
    const commands = [];
    let stopped = 0;
    let prevented = 0;
    const event = (target, inside = true) => ({
      target,
      currentTarget: new FakeElement({}, inside),
      stopPropagation: () => stopped++,
      preventDefault: () => prevented++,
    });
    handleMessageClick(event(new FakeElement({ '.run-code-btn': button })), undefined, (c) => commands.push(c));
    assert.deepEqual([commands, stopped, prevented], [['make test'], 1, 0]);

    // No owner, an empty command, or a button outside the message: nothing runs.
    handleMessageClick(event(new FakeElement({ '.run-code-btn': button })));
    handleMessageClick(event(new FakeElement({ '.run-code-btn': { dataset: {} } })), undefined, (c) => commands.push(c));
    handleMessageClick(event(new FakeElement({ '.run-code-btn': button }), false), undefined, (c) => commands.push(c));
    assert.deepEqual(commands, ['make test']);
  } finally {
    if (previous === undefined) delete globalThis.Element;
    else globalThis.Element = previous;
  }
});

const TEXT_PART_URL = new URL('../src/lib/components/transcript/parts/TextPart.svelte', import.meta.url);
const lib = (rel) => JSON.stringify(new URL(rel, TEXT_PART_URL).href);
let textPartModule;

async function renderTextPart(props) {
  textPartModule ??= await loadServerComponent(TEXT_PART_URL, [
    ["'../../../markdown'", lib('../../../markdown.ts')],
    ["'../../../messageFileClick'", lib('../../../messageFileClick.ts')],
    ["import { renderMermaid } from '../../../mermaid';", 'const renderMermaid = () => {};'],
    ["import { theme } from '../../../stores/theme.svelte';", fixtureStore('theme')],
  ]);
  return unescapeHTML(await renderWithFixture(textPartModule, { theme: { resolved: 'dark' } }, props));
}

test('a text part shows Run only when its owner can run commands', async () => {
  const text = 'Try:\n\n```bash\nnpm test\n```';
  assert.match(await renderTextPart({ text, onRunCommand: () => {} }), /class="run-code-btn" data-command="npm test"/);
  assert.doesNotMatch(await renderTextPart({ text }), /run-code-btn/);
});

test('Run is wired to assistant text in the transcript and the live chat, where the panel can open a shell', () => {
  const source = (path) => readFileSync(new URL(`../src/lib/components/${path}.svelte`, import.meta.url), 'utf8');
  const bubble = source('transcript/MessageBubble');
  assert.match(bubble, /<TextPart text=\{part\.text\} \{onFileLink\} onRunCommand=\{isAssistant \? onRunCommand : undefined\} \/>/);
  assert.doesNotMatch(bubble, /<ReasoningPart[^>]*onRunCommand/, 'reasoning keeps Copy only');

  const transcript = source('transcript/TranscriptView');
  assert.match(transcript, /launcher\.info\.panel && session && !session\.cwdMissing \? session\.cwd : ''/);
  assert.match(transcript, /runDir \? \(command: string\) => void terminals\.runCommand\(command, runDir\) : undefined/);
  assert.match(transcript, /<MessageBubble\s+message=\{msg\}[\s\S]*?\{onRunCommand\}[\s\S]*?\/>/);
  assert.match(transcript, /onRunCommand=\{launcher\.info\.panel && chat\.dir \? \(command\) => void terminals\.runCommand\(command, chat\.dir\) : undefined\}/);
  assert.match(source('chat/LiveTail'), /<MessageBubble [^>]*\{onRunCommand\} \/>/);

  const part = source('transcript/parts/TextPart');
  assert.match(part, /\.text-part-container \.copy-code-btn, \.text-part-container \.run-code-btn,/, 'Run looks like Copy');
  assert.match(part, /\.text-part-container \.copy-code-btn:hover, \.text-part-container \.run-code-btn:hover,/);
});

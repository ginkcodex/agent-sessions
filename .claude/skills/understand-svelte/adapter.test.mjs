import assert from 'node:assert/strict';
import { test } from 'node:test';
import { pathToFileURL, fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { homedir } from 'node:os';
import { compilerFor } from './svelte-source.mjs';
import { analyzeSvelte, installAdapter } from './adapter.mjs';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const plugin = process.env.UA_PLUGIN_ROOT || join(homedir(), '.claude/plugins/cache/understand-anything/understand-anything/2.9.8');
const core = await import(pathToFileURL(join(plugin, 'packages/core/dist/index.js')).href);
const { compareFileSymbols } = await import(pathToFileURL(join(plugin, 'skills/understand/validate-incremental-symbols.mjs')).href);
const compiler = compilerFor(root);
const parser = new core.TreeSitterPlugin(core.builtinLanguageConfigs.filter(c => c.treeSitter));
await parser.init();
const { strict } = installAdapter(core, compiler);
const evidence = source => analyzeSvelte(parser, strict, source, compiler);
const graph = (name, lineRange = [2, 2]) => ({ nodes: [{ id: `function:Foo.svelte:${name}`, type: 'function', name, filePath: 'Foo.svelte', lineRange }], edges: [] });

test('real strict TS evidence preserves a moved Svelte callable', () => {
  const before = evidence('<script lang="ts">\nfunction send(value: string) { return value; }\n</script>');
  const after = evidence('<h1>changed</h1>\n<script lang="ts">\nfunction send(value: string) { return value; }\n</script>');
  assert.equal(before.status, 'succeeded');
  assert.deepEqual(compareFileSymbols(graph('send'), graph('send', [3, 3]), before, after).missing, []);
});

test('omitted still-present symbols block, and deletion remains conservative', () => {
  const before = evidence('<script>\nfunction send() {}\nfunction keep() {}\n</script>');
  assert.equal(compareFileSymbols(graph('send'), { nodes: [], edges: [] }, before, before).missing[0].status, 'still-present');
  const after = evidence('<script>\nfunction keep() {}\n</script>');
  assert.equal(compareFileSymbols(graph('send'), graph('keep'), before, after).missing[0].status, 'unknown');
});

test('module and instance top-level functions have different source owners', () => {
  const result = evidence('<script module>\nfunction same() {}\n</script>\n<script>\nfunction same() {}\n</script>');
  const functions = result.symbolEvidence.functions.filter(f => f.name === 'same');
  assert.equal(functions.length, 2);
  assert.deepEqual(functions.map(f => f.scope), [{ kind: 'class', name: '$svelte.module' }, { kind: 'file' }]);
});

test('moving a function between module and instance is not identity preservation', () => {
  const before = evidence('<script>\nfunction send() {}\n</script>');
  const after = evidence('<script module>\nfunction send() {}\n</script>');
  assert(compareFileSymbols(graph('send'), graph('send'), before, after).missing.length > 0);
});

test('malformed source fails closed and Svelte fingerprints remain content-only', () => {
  assert.equal(parser.analyzeFileStrict('Foo.svelte', '<script>function {</script>').status, 'failed');
  const registry = new core.PluginRegistry();
  registry.register(parser);
  assert.equal(registry.getLanguageForFile('Foo.svelte'), 'svelte');
  assert.equal(registry.getPluginForFile('Foo.svelte'), parser);
  assert.equal(registry.analyzeFile('Foo.svelte', '<script>function f() {}</script>').functions[0].name, 'f');
  assert(!new Set(core.builtinLanguageConfigs.filter(c => c.treeSitter).map(c => c.id)).has('svelte'));
});

test('local shadow is not a file-scope replacement', () => {
  const before = evidence('<script>\nfunction send() {}\n</script>');
  const after = evidence('<script>\nfunction outer() { function send() {} }\n</script>');
  assert(compareFileSymbols(graph('send'), graph('send'), before, after).missing.length > 0);
});

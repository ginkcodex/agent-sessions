import assert from 'node:assert/strict';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { compilerFor, parseSvelte, routeContext } from './svelte-source.mjs';

const compiler = compilerFor(fileURLToPath(new URL('../../../', import.meta.url)));

test('compiler identifies separate TS module and instance scripts at original offsets', () => {
  const source = `<!-- <script>not a script</script> -->\n<script module lang="ts">export function same() { return 1; }</script>\n<script lang="ts">\nlet { value: local = 'x', ...rest }: Props = $props();\nfunction same(x: string) { return x; }\n</script>\n<h1>{local}</h1><style>h1 { color: red; }</style>`;
  const result = parseSvelte(source, compiler);
  assert.deepEqual(result.scripts.map(s => s.scope), ['module', 'instance']);
  for (const script of result.scripts) {
    assert.equal(script.source.length, source.length);
    assert.equal(script.source.slice(script.start, script.end), source.slice(script.start, script.end));
    assert.equal(script.source.split('\n').length, source.split('\n').length);
    assert(!script.source.includes('<h1>'));
    assert(script.source.includes('function same'));
  }
  assert.deepEqual(result.scripts[1].props.map(({ name, bindings, rest }) => ({ name, bindings, rest })), [
    { name: 'value', bindings: ['local'], rest: false }, { name: null, bindings: ['rest'], rest: true },
  ]);
});

test('JS, legacy context module, props object and script-less files', () => {
  assert.equal(parseSvelte('<p>hello</p>', compiler).scripts.length, 0);
  const result = parseSvelte(`<script context="module">export const x = 1;</script><script>import Child from './Child.svelte'; let props = $props();</script>`, compiler);
  assert.equal(result.scripts[0].scope, 'module');
  assert.deepEqual(result.scripts[1].props[0].bindings, ['props']);
  assert.equal(result.scripts[1].imports[0].source, './Child.svelte');
});

test('legacy props and bindable defaults retain public and local names', () => {
  const legacy = parseSvelte('<script>export let title = "hello";</script>', compiler);
  assert.equal(legacy.scripts[0].props[0].name, 'title');
  assert.equal(legacy.scripts[0].props[0].legacy, true);
  const runes = parseSvelte('<script lang="ts">let { value: local = $bindable() } = $props();</script>', compiler);
  assert.equal(runes.scripts[0].props[0].name, 'value');
  assert.deepEqual(runes.scripts[0].props[0].bindings, ['local']);
});

test('malformed input and duplicate scripts fail closed', () => {
  assert.throws(() => parseSvelte('<script lang="ts">function {</script>', compiler));
  assert.throws(() => parseSvelte('<script></script><script></script>', compiler));
});

test('SvelteKit file conventions are metadata, not inferred calls', () => {
  assert.deepEqual(routeContext('src/routes/(app)/users/[id]/+page.svelte'), { kind: 'page-component', server: false, route: '/users/[id]' });
  assert.deepEqual(routeContext('web/src/routes/+layout.server.ts'), { kind: 'layout-load', server: true, route: '/' });
  assert.deepEqual(routeContext('src/routes/api/+server.js'), { kind: 'endpoint', server: true, route: '/api' });
  assert.equal(routeContext('src/lib/+page.svelte'), null);
  assert.equal(routeContext('src/routes/foo/+server.svelte'), null);
  assert.equal(routeContext('src/routes/+page.server.svelte'), null);
});

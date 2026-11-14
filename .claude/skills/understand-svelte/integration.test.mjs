import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, symlinkSync } from 'node:fs';
import { tmpdir, homedir } from 'node:os';
import { resolve, join } from 'node:path';
import { pathToFileURL, fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const root = fileURLToPath(new URL('../../../', import.meta.url));
const skill = fileURLToPath(new URL('.', import.meta.url));
const plugin = process.env.UA_PLUGIN_ROOT || join(homedir(), '.claude/plugins/cache/understand-anything/understand-anything/2.9.8');
const env = { ...process.env, UA_SVELTE_CORE: join(plugin, 'packages/core/dist/index.js'), UA_SVELTE_PROJECT: root };
const preload = pathToFileURL(join(skill, 'preload.mjs')).href;
function node(args, options = {}) {
  const result = spawnSync(process.execPath, args, { encoding: 'utf8', ...options });
  assert.equal(result.status, 0, result.stderr || result.stdout);
  return result.stdout;
}

test('local planner changes count policy but not upstream defaults', () => {
  const program = `const {classifyUpdate}=await import(${JSON.stringify(pathToFileURL(env.UA_SVELTE_CORE).href)}); console.log(JSON.stringify(classifyUpdate({newFiles:[],deletedFiles:[],structurallyChangedFiles:Array.from({length:58},(_,i)=>'src/'+i+'.ts'),cosmeticOnlyFiles:[]},100)));`;
  const stock = JSON.parse(node(['--input-type=module', '-e', program]));
  const local = JSON.parse(node(['--import', preload, '--input-type=module', '-e', program], { env }));
  assert.equal(stock.action, 'FULL_UPDATE');
  assert.equal(local.action, 'ARCHITECTURE_UPDATE');
  assert.equal(local.filesToReanalyze.length, 58);
  assert.equal(local.rerunArchitecture, true);
  assert.equal(local.rerunTour, true);
});

test('unsupported plugin versions fail before invoking helpers', () => {
  const directory = mkdtempSync(join(tmpdir(), 'ua-svelte-version-'));
  writeFileSync(join(directory, 'package.json'), '{"version":"9.9.9"}');
  const result = spawnSync(process.execPath, [join(skill, 'run.mjs'), 'validate', root], {
    env: { ...process.env, UA_PLUGIN_ROOT: directory }, encoding: 'utf8',
  });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /Only Understand-Anything 2.9.8 is supported/);
});

test('wrapped extraction, imports and content-only fingerprints use the adapter', () => {
  const directory = mkdtempSync(join(tmpdir(), 'ua-svelte-fixture-'));
  mkdirSync(join(directory, 'src'));
  symlinkSync(join(root, 'frontend/node_modules'), join(directory, 'node_modules'));
  writeFileSync(join(directory, 'package.json'), '{"type":"module"}');
  writeFileSync(join(directory, 'src/Child.svelte'), '<p>child</p>');
  writeFileSync(join(directory, 'src/Page.svelte'), `<script lang="ts">import Child from './Child.svelte';\nlet { title }: {title: string} = $props();\nfunction send() { return title; }</script><Child/><h1>{title}</h1>`);
  const files = [
    { path: 'src/Page.svelte', language: 'svelte', sizeLines: 3, fileCategory: 'code' },
    { path: 'src/Child.svelte', language: 'svelte', sizeLines: 1, fileCategory: 'code' },
  ];
  const input = join(directory, 'input.json'), output = join(directory, 'output.json');
  writeFileSync(input, JSON.stringify({ projectRoot: directory, files, batchFiles: files, batchImportData: {} }));
  node([join(skill, 'run.mjs'), 'extract', directory, input, output]);
  const extraction = JSON.parse(readFileSync(output));
  assert.deepEqual(extraction.filesSkipped, []);
  assert(JSON.stringify(extraction).includes('send'));
  node([join(skill, 'run.mjs'), 'imports', directory, input, output]);
  const imports = JSON.parse(readFileSync(output));
  assert.deepEqual(imports.failures, []);
  assert.deepEqual(imports.importMap['src/Page.svelte'], ['src/Child.svelte']);
  const coreProgram = `import {createRequire} from 'node:module';const core=await import(${JSON.stringify(pathToFileURL(env.UA_SVELTE_CORE).href)});const p=new core.TreeSitterPlugin(core.builtinLanguageConfigs.filter(c=>c.treeSitter));await p.init();const r=new core.PluginRegistry();r.register(p);const store=core.buildFingerprintStore(${JSON.stringify(directory)},['src/Page.svelte'],r,'test',{structuralFingerprintLanguages:new Set(core.builtinLanguageConfigs.filter(c=>c.treeSitter).map(c=>c.id))});console.log(JSON.stringify(store));`;
  const fingerprints = JSON.parse(node(['--import', preload, '--input-type=module', '-e', coreProgram], { env }));
  assert.equal(fingerprints.files['src/Page.svelte'].hasStructuralAnalysis, false);
});

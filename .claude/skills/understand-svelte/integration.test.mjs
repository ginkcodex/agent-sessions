import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, symlinkSync, rmSync, existsSync } from 'node:fs';
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

test('wrapped architecture update normalizes the tour before finalization without publishing early', () => {
  const directory = mkdtempSync(join(tmpdir(), 'ua-svelte-architecture-'));
  try {
    const ua = join(directory, '.ua'), intermediate = join(ua, 'intermediate');
    mkdirSync(join(directory, 'src'));
    mkdirSync(intermediate, { recursive: true });
    symlinkSync(join(root, 'frontend/node_modules'), join(directory, 'node_modules'));
    const json = (path, value) => writeFileSync(path, JSON.stringify(value));
    const read = (path) => JSON.parse(readFileSync(path, 'utf8'));
    const git = (...args) => {
      const result = spawnSync('git', args, { cwd: directory, encoding: 'utf8' });
      assert.equal(result.status, 0, result.stderr);
      return result.stdout.trim();
    };
    const run = (command, ...args) => node([join(skill, 'run.mjs'), command, directory, ...args]);
    writeFileSync(join(directory, '.gitignore'), '.ua/\nnode_modules/\n');
    writeFileSync(join(directory, 'README.md'), '# Fixture\nA Svelte project.\n');
    writeFileSync(join(directory, 'src/Page.svelte'), '<h1>Hello</h1>\n');
    git('init', '-q');
    git('add', '.');
    git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false', 'commit', '-qm', 'baseline');
    const baseCommit = git('rev-parse', 'HEAD');
    const nodes = [
      { id: 'document:README.md', type: 'document', name: 'README.md', filePath: 'README.md', summary: 'Fixture overview.', tags: ['documentation'], complexity: 'simple' },
      { id: 'file:src/Page.svelte', type: 'file', name: 'Page.svelte', filePath: 'src/Page.svelte', summary: 'Greeting component.', tags: ['component'], complexity: 'simple' },
    ];
    json(join(ua, 'knowledge-graph.json'), { version: '1.0.0', project: { name: 'fixture', gitCommitHash: baseCommit }, nodes, edges: [], layers: [], tour: [] });
    json(join(ua, 'meta.json'), { gitCommitHash: baseCommit, version: '1.0.0', analyzedFiles: 2 });
    json(join(ua, 'code-lessons.json'), { version: 1, files: { 'src/Page.svelte': { overview: 'Greeting.', sections: [] }, 'removed.svelte': { overview: 'Orphan.', sections: [] } } });
    const fingerprintInput = join(ua, 'fingerprint-input.json');
    json(fingerprintInput, { projectRoot: directory, filePaths: ['README.md', 'src/Page.svelte'], gitCommitHash: baseCommit });
    run('fingerprints', fingerprintInput);
    mkdirSync(join(directory, 'components'));
    writeFileSync(join(directory, 'components/Child.svelte'), '<p>Child</p>\n');
    git('add', 'components/Child.svelte');
    git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false', 'commit', '-qm', 'add component');
    const headCommit = git('rev-parse', 'HEAD');
    run('prepare', baseCommit);
    const plan = read(join(intermediate, 'incremental-plan.json'));
    assert.equal(plan.action, 'ARCHITECTURE_UPDATE');
    assert.equal(plan.rerunTour, true);
    assert.deepEqual(plan.filesToReanalyze, ['components/Child.svelte']);
    json(join(intermediate, 'batch-0.json'), { nodes: [{ ...nodes[1], id: 'file:components/Child.svelte', name: 'Child.svelte', filePath: 'components/Child.svelte', summary: 'Child component.' }], edges: [] });
    run('merge');
    assert.equal(read(join(intermediate, 'incremental-symbol-report.json')).ok, true);
    const published = ['knowledge-graph.json', 'meta.json', 'fingerprints.json', 'code-lessons.json'];
    const before = published.map(name => readFileSync(join(ua, name), 'utf8'));
    const failed = spawnSync(process.execPath, [join(skill, 'run.mjs'), 'normalize-tour', directory], { encoding: 'utf8' });
    assert.notEqual(failed.status, 0);
    assert.match(failed.stderr, /tour\.json is missing/);
    assert.equal(existsSync(join(intermediate, 'tour.normalized.json')), false);
    json(join(intermediate, 'layers.json'), [{ id: 'layer:fixture', name: 'Fixture', description: 'Fixture files.', nodeIds: [...nodes.map(n => n.id), 'file:components/Child.svelte'] }]);
    json(join(intermediate, 'tour.json'), { steps: [
      { order: 2, title: 'Child', description: 'Read the new component.', nodesToInspect: ['components/Child.svelte', 'missing.svelte'] },
      { order: 1, title: 'Overview', description: 'Start with the documentation.', nodeIds: ['README.md'] },
    ] });
    run('normalize-tour');
    const tour = read(join(intermediate, 'tour.normalized.json'));
    assert.deepEqual(tour.map(step => step.nodeIds), [['document:README.md'], ['file:components/Child.svelte']]);
    assert.deepEqual(published.map(name => readFileSync(join(ua, name), 'utf8')), before);
    run('finalize');
    const graph = read(join(ua, 'knowledge-graph.json'));
    assert.deepEqual(graph.tour, tour);
    assert.equal(graph.nodes.length, 3);
    assert.equal(graph.project.gitCommitHash, headCommit);
    assert.equal(read(join(ua, 'meta.json')).gitCommitHash, headCommit);
    assert.equal(read(join(ua, 'fingerprints.json')).gitCommitHash, headCommit);
    assert.equal(read(join(intermediate, 'incremental-symbol-report.json')).ok, true);
    run('prune');
    assert.deepEqual(Object.keys(read(join(ua, 'code-lessons.json')).files), ['src/Page.svelte']);
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});

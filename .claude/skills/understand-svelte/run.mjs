import { existsSync, readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { resolve, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';

if (Number(process.versions.node.split('.')[0]) < 24) throw new Error('Understand Svelte requires Node >=24.');
const [command, rootArgument, ...args] = process.argv.slice(2);
const projectRoot = resolve(rootArgument || '.');
const pluginRoot = process.env.UA_PLUGIN_ROOT || join(homedir(), '.claude/plugins/cache/understand-anything/understand-anything/2.9.8');
const scripts = {
  prepare: 'prepare-incremental.mjs',
  batches: 'compute-batches.mjs',
  extract: 'extract-structure.mjs',
  imports: 'extract-import-map.mjs',
  validate: 'validate-incremental-symbols.mjs',
  retry: 'prepare-symbol-retry.mjs',
  merge: 'merge-batch-graphs.py',
  'normalize-tour': 'normalize-tour.mjs',
  finalize: 'finalize-incremental.mjs',
  fingerprints: 'build-fingerprints.mjs',
  prune: 'prune-code-lessons.mjs',
};
if (!scripts[command]) throw new Error(`Usage: node run.mjs <${Object.keys(scripts).join('|')}> <projectRoot> [script arguments]`);
if (JSON.parse(readFileSync(join(pluginRoot, 'package.json'), 'utf8')).version !== '2.9.8') {
  throw new Error('Only Understand-Anything 2.9.8 is supported. Reverify integration before upgrading.');
}
const core = join(pluginRoot, 'packages/core/dist/index.js');
if (!existsSync(core)) throw new Error('Understand-Anything core is not built. Build the plugin first.');
const preload = pathToFileURL(fileURLToPath(new URL('./preload.mjs', import.meta.url))).href;
const env = {
  ...process.env,
  UA_SVELTE_CORE: core,
  UA_SVELTE_PROJECT: projectRoot,
  NODE_OPTIONS: `${process.env.NODE_OPTIONS || ''} --import=${preload}`.trim(),
};
const script = join(pluginRoot, 'skills/understand', scripts[command]);
const scriptArgs = ['extract', 'imports', 'fingerprints'].includes(command) ? args : [projectRoot, ...args];
const result = spawnSync(command === 'merge' ? 'python3' : process.execPath,
  command === 'merge' ? ['-I', script, ...scriptArgs] : [script, ...scriptArgs],
  { cwd: projectRoot, env, stdio: 'inherit' });
if (result.error) throw result.error;
if (result.signal) throw new Error(`Understand-Anything stopped by ${result.signal}`);
process.exitCode = result.status;

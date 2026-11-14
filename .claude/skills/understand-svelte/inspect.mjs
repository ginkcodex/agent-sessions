import { readFileSync, realpathSync } from 'node:fs';
import { resolve, relative, isAbsolute } from 'node:path';
import { compilerFor, parseSvelte, routeContext } from './svelte-source.mjs';

const [rootArgument, ...paths] = process.argv.slice(2);
if (!rootArgument || !paths.length) throw new Error('Usage: node inspect.mjs <projectRoot> <relative-path>...');
const root = realpathSync(rootArgument);
for (const path of paths) {
  const absolute = realpathSync(resolve(root, path));
  const local = relative(root, absolute);
  if (isAbsolute(path) || local === '..' || local.startsWith('../')) throw new Error('Path must be inside the project');
  const source = readFileSync(absolute, 'utf8');
  const scripts = path.endsWith('.svelte') ? parseSvelte(source, compilerFor(root)).scripts.map(({ source: masked, ...script }) => script) : [];
  console.log(JSON.stringify({ filePath: path, route: routeContext(path), scripts }, null, 2));
}

import { pathToFileURL } from 'node:url';
import { registerHooks } from 'node:module';
import { compilerFor } from './svelte-source.mjs';
import { installAdapter } from './adapter.mjs';

if (!process.env.UA_SVELTE_CORE || !process.env.UA_SVELTE_PROJECT) {
  throw new Error('Run through understand-svelte/run.mjs, not the preload directly.');
}
const root = process.env.UA_SVELTE_CORE.replace(/packages\/core\/dist\/index\.js$/, '');
function replaceOnce(source, before, after) {
  if (!source.includes(before) || source.indexOf(before) !== source.lastIndexOf(before)) throw new Error('Unsupported Understand-Anything integration point');
  return source.replace(before, after);
}
registerHooks({
  load(url, context, nextLoad) {
    const result = nextLoad(url, context);
    if (!url.startsWith(pathToFileURL(root).href)) return result;
    let source = result.source?.toString();
    if (url.endsWith('/change-classifier.js')) {
      source = replaceOnce(source, 'const triggeredByCount = structuralCount > 30;', 'const triggeredByCount = false; // Local policy: counts alone do not require a full rebuild.');
      source = replaceOnce(source, 'if (triggeredByCount || triggeredByPercentage) {', 'if (false) { // Local policy: use incremental architecture refresh even for large diffs.');
    }
    if (url.endsWith('/extract-import-map.mjs')) {
      source = replaceOnce(source, "'typescript', 'javascript', 'tsx', 'jsx', 'vue',", "'typescript', 'javascript', 'tsx', 'jsx', 'vue', 'svelte',");
    }
    return source === result.source?.toString() ? result : { ...result, source };
  },
});
const core = await import(pathToFileURL(process.env.UA_SVELTE_CORE).href);
installAdapter(core, compilerFor(process.env.UA_SVELTE_PROJECT));

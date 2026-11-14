import { parseSvelte } from './svelte-source.mjs';

export function analyzeSvelte(parser, originalStrict, source, compiler) {
  const { scripts } = parseSvelte(source, compiler);
  const structure = { functions: [], classes: [], imports: [], exports: [] };
  const symbolEvidence = { version: 2, functions: [], classes: [], effects: [], coverage: { profile: 'structural-declarations-v1', gaps: [] } };
  for (const [index, script] of scripts.entries()) {
    const evidence = originalStrict.call(parser, 'component.ts', script.source);
    if (evidence.status !== 'succeeded' || evidence.symbolEvidence?.version !== 2) {
      throw new Error(`Cannot parse ${script.scope} script with strict TypeScript evidence`);
    }
    for (const key of ['functions', 'classes', 'imports', 'exports']) structure[key].push(...evidence.structure[key]);
    const scoped = entries => entries.map(entry => ({ ...entry, scope: entry.scope.kind === 'local'
      ? { ...entry.scope, id: entry.scope.id * 2 + index }
      : script.scope === 'module' && entry.scope.kind === 'file'
        ? { kind: 'class', name: '$svelte.module' } : entry.scope }));
    for (const key of ['functions', 'classes', 'effects']) symbolEvidence[key].push(...scoped(evidence.symbolEvidence[key]));
    symbolEvidence.coverage.gaps.push(...scoped(evidence.symbolEvidence.coverage.gaps));
  }
  // Template snippets and cross-script dynamic bindings are not TS declarations.
  // Keep deletion conservative until a full Svelte lexical-scope adapter exists.
  symbolEvidence.coverage.gaps.push({ kind: null, scope: { kind: 'unknown' }, name: null, reason: 'Svelte template and cross-script declaration coverage is conservative', lineRange: [1, source.split('\n').length] });
  return { status: 'succeeded', language: 'svelte', structure, symbolEvidence };
}

export function installAdapter(core, compiler) {
  const prototype = core.TreeSitterPlugin.prototype;
  const strict = prototype.analyzeFileStrict;
  const analyze = prototype.analyzeFile;
  const full = prototype.analyzeFileFull;
  const calls = prototype.extractCallGraph;
  const isSvelte = path => path.endsWith('.svelte');
  prototype.analyzeFileStrict = function(path, content) {
    if (!isSvelte(path)) return strict.call(this, path, content);
    try { return analyzeSvelte(this, strict, content, compiler); }
    catch { return { status: 'failed', structure: null, symbolEvidence: null }; }
  };
  prototype.analyzeFile = function(path, content) {
    return isSvelte(path) ? analyzeSvelte(this, strict, content, compiler).structure : analyze.call(this, path, content);
  };
  prototype.extractCallGraph = function(path, content) {
    if (!isSvelte(path)) return calls.call(this, path, content);
    // A combined graph could bind same-name calls to the wrong script scope.
    return [];
  };
  prototype.analyzeFileFull = function(path, content) {
    return isSvelte(path) ? { structure: this.analyzeFile(path, content), callGraph: [] } : full.call(this, path, content);
  };
  const registry = core.PluginRegistry.prototype;
  const getPlugin = registry.getPluginForFile;
  const getLanguage = registry.getLanguageForFile;
  registry.getPluginForFile = function(path) {
    return isSvelte(path) ? this.getPlugins().find(p => p instanceof core.TreeSitterPlugin) ?? null : getPlugin.call(this, path);
  };
  registry.getLanguageForFile = function(path) {
    return isSvelte(path) ? 'svelte' : getLanguage.call(this, path);
  };
  return { strict };
}

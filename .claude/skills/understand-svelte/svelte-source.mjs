import { createRequire } from 'node:module';
import { resolve } from 'node:path';

export function compilerFor(projectRoot) {
  for (const base of ['frontend/package.json', 'package.json']) {
    const require = createRequire(resolve(projectRoot, base));
    try { return require('svelte/compiler'); } catch (error) {
      if (error.code !== 'MODULE_NOT_FOUND') throw error;
    }
  }
  throw new Error('Svelte compiler not installed: install the project dependencies first.');
}

export function routeContext(filePath) {
  const path = filePath.replaceAll('\\', '/');
  const match = path.match(/(?:^|\/)src\/routes\/(.*\/)?\+(page|layout|server)(\.server)?\.(svelte|ts|js)$/);
  if (!match) return null;
  const [, directory = '', kind, server, extension] = match;
  if (kind === 'server' && (server || extension === 'svelte')) return null;
  if (server && extension === 'svelte') return null;
  return {
    kind: extension === 'svelte' ? `${kind}-component` : kind === 'server' ? 'endpoint' : `${kind}-load`,
    server: kind === 'server' || Boolean(server),
    route: '/' + directory.split('/').filter(segment => segment && !/^\([^/]+\)$/.test(segment)).join('/'),
  };
}

function bindingNames(node) {
  if (!node) return [];
  if (node.type === 'Identifier') return [node.name];
  if (node.type === 'AssignmentPattern') return bindingNames(node.left);
  if (node.type === 'RestElement') return bindingNames(node.argument);
  if (node.type === 'ObjectPattern') return node.properties.flatMap(p => bindingNames(p.value || p.argument));
  if (node.type === 'ArrayPattern') return node.elements.flatMap(bindingNames);
  return [];
}

export function parseSvelte(source, compiler) {
  const ast = compiler.parse(source, { modern: true });
  const scripts = [];
  for (const [scope, block] of [['module', ast.module], ['instance', ast.instance]]) {
    if (!block) continue;
    const { start, end } = block.content;
    if (!Number.isInteger(start) || !Number.isInteger(end) || start < 0 || end > source.length) {
      throw new Error('Unsupported Svelte compiler script ranges');
    }
    const props = [];
    const imports = [];
    for (const statement of block.content.body) {
      if (statement.type === 'ImportDeclaration') imports.push({ source: statement.source.value, start: statement.start, end: statement.end });
      const declaration = statement.type === 'ExportNamedDeclaration' ? statement.declaration : statement;
      if (declaration?.type !== 'VariableDeclaration') continue;
      for (const item of declaration.declarations) {
        if (scope === 'instance' && statement.type === 'ExportNamedDeclaration' && declaration.kind === 'let') {
          props.push({ name: item.id.name ?? null, bindings: bindingNames(item.id), rest: false, legacy: true, start: item.start, end: item.end });
        }
        if (scope !== 'instance' || item.init?.type !== 'CallExpression' || item.init.callee?.type !== 'Identifier' || item.init.callee.name !== '$props') continue;
        if (item.id.type === 'ObjectPattern') {
          for (const prop of item.id.properties) {
            props.push({
              name: prop.type === 'RestElement' ? null : prop.computed ? null : prop.key.name ?? prop.key.value,
              bindings: bindingNames(prop.type === 'RestElement' ? prop.argument : prop.value),
              rest: prop.type === 'RestElement',
              start: prop.start,
              end: prop.end,
            });
          }
        } else props.push({ name: null, bindings: bindingNames(item.id), rest: true, start: item.start, end: item.end });
      }
    }
    // Whitespace masking preserves UTF-16 offsets and lines for the TS parser.
    const masked = source.slice(0, start).replace(/[^\r\n]/g, ' ') + source.slice(start, end) + source.slice(end).replace(/[^\r\n]/g, ' ');
    scripts.push({ scope, start, end, source: masked, props, imports });
  }
  return { scripts };
}

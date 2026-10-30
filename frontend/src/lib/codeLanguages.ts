/**
 * Languages that Settings can turn off for code checks and code navigation.
 * IDs, providers and extensions mirror the backend's language table
 * (internal/codediagnostics/languages.go); the backend owns discovery.
 */
export type CodeLanguageId = 'go' | 'typescript' | 'svelte' | 'python' | 'rust';

export interface CodeLanguage {
  id: CodeLanguageId;
  name: string;
  /** Provider IDs: language servers first, then saved-file checkers. */
  providers: readonly string[];
  /** Lowercase source extensions, as the backend's live discovery sees them. */
  extensions: readonly string[];
}

export const CODE_LANGUAGES: readonly CodeLanguage[] = [
  { id: 'go', name: 'Go', providers: ['gopls', 'go-vet', 'go-syntax'], extensions: ['.go'] },
  { id: 'typescript', name: 'TypeScript/JavaScript', providers: ['typescript'], extensions: ['.ts', '.mts', '.cts', '.tsx', '.js', '.mjs', '.cjs', '.jsx'] },
  { id: 'svelte', name: 'Svelte', providers: ['svelte', 'svelte-check'], extensions: ['.svelte'] },
  { id: 'python', name: 'Python', providers: ['ruff', 'pyright'], extensions: ['.py', '.pyi'] },
  { id: 'rust', name: 'Rust', providers: ['rust-analyzer', 'cargo-check'], extensions: ['.rs'] },
];

export function isCodeLanguage(id: unknown): id is CodeLanguageId {
  return CODE_LANGUAGES.some(l => l.id === id);
}

/** Known IDs once each, in table order; anything else is dropped. */
export function codeLanguageIds(ids: readonly unknown[]): CodeLanguageId[] {
  return CODE_LANGUAGES.filter(l => ids.includes(l.id)).map(l => l.id);
}

/** The language a provider belongs to; nested Go vet contexts are numbered `go-vet-N`. */
export function languageOfProvider(providerId: string): CodeLanguage | undefined {
  const id = /^go-vet-\d+$/.test(providerId) ? 'go-vet' : providerId;
  return CODE_LANGUAGES.find(l => l.providers.includes(id));
}

/** The language whose sources include path, by extension. */
export function languageOfPath(path: string): CodeLanguage | undefined {
  const lower = path.toLowerCase();
  return CODE_LANGUAGES.find(l => l.extensions.some(ext => lower.endsWith(ext)));
}

/** "Go", "Go and Rust", "Go, Python and Rust". */
export function languageList(names: readonly string[]): string {
  return names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names.at(-1)}` : names[0] ?? '';
}

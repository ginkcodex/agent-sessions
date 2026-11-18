import type { CodeDiagnosticsAPI, CodeDiagnosticsContext, CodeDiagnosticsSnapshot, CodeDiagnosticsRun, CodeDiagnosticsDocumentReceipt, CodeDiagnosticPosition, CodeNavigationLocation, CodeNavigationOp, CodeNavigationResult, CodeCompletionTrigger, CodeCompletionResult, CodeCompletionDetails, CodeCompletionItem } from '../codeDiagnostics';
import { CODE_DIAGNOSTICS_EVENT, CodeDiagnosticsError, sameContext } from '../codeDiagnostics';
import type { FileContent } from '../types';
import { CODE_LANGUAGES } from '../codeLanguages';

/** Synthetic dependency sources outside every project, for go to definition. */
const MOCK_EXTERNAL: Record<string, string> = {
  '/usr/local/go/src/fmt/print.go': 'package fmt\n\n// Println formats using the default formats for its operands and writes to standard output.\nfunc Println(a ...any) (n int, err error) {\n\treturn Fprintln(os.Stdout, a...)\n}\n',
};

/** Synthetic, in-memory checker. No executable or editor/text dependencies. */
export class MockCodeDiagnostics implements CodeDiagnosticsAPI {
  private serial = 0;
  private snapshots = new Map<string, CodeDiagnosticsSnapshot>();
  private publishEvent: (name: string, event: unknown) => void;
  /** Synthetic in-memory document mirror, per attachment. Live support is never advertised. */
  liveDocuments = false;
  private documents = new Map<string, Map<string, { path: string; version: number; text: string }>>();
  private closed = new Set<string>();
  /** External paths navigation answers pointed to, per attachment. */
  private grants = new Map<string, Set<string>>();
  /** Each document's last completion answer, by document ID. */
  private answers = new Map<string, { resultId: string; version: number; items: { item: CodeCompletionItem; path: string; declared?: string }[] }>();
  /** The languages the last attach turned off. */
  disabledLanguages: string[] = [];
  constructor(publishEvent: (name: string, event: unknown) => void) { this.publishEvent = publishEvent; }
  private get(c: CodeDiagnosticsContext, host: string) {
    const s = this.snapshots.get(c.attachmentId);
    if (host !== 'mock:local' || !s || !sameContext(c, s.context)) throw new CodeDiagnosticsError('staleContext', 'Mock attachment expired');
    return s;
  }
  private publish(s: CodeDiagnosticsSnapshot) {
    s.sequence++;
    this.publishEvent(CODE_DIAGNOSTICS_EVENT, structuredClone({ contractVersion: 1, context: s.context, sequence: s.sequence, updateId: `mock-update-${++this.serial}`, partIndex: 0, partCount: 1, snapshot: s }));
    return structuredClone(s);
  }
  async attachCodeDiagnostics(dir: string, version: 1, host: string, disabled: string[] = []) {
    if (host !== 'mock:local' || version !== 1) throw new CodeDiagnosticsError('unsupported', 'Mock diagnostics support Local only');
    this.disabledLanguages = [...disabled];
    const context = { connectionId: 'mock-connection', attachmentId: `mock-${++this.serial}`, generation: `generation-${this.serial}` };
    const snapshot: CodeDiagnosticsSnapshot = { contractVersion: 1, context, sequence: 0, approval: { state: 'required' }, providers: [{ providerId: 'mock-checker', availability: 'approvalRequired', phase: 'idle', coverage: { kind: 'savedProject', roots: [''], partial: true, externalChanges: 'unwatched' }, freshness: 'uncertain', truncated: false }], resultSets: [], proposal: { fingerprint: 'mock-fingerprint', tools: [{ providerId: 'mock-checker', executable: '/synthetic/checker', configPaths: [] }] } };
    // Like the backend, each language that is off is listed with nothing to approve.
    for (const language of CODE_LANGUAGES.filter(l => disabled.includes(l.id))) {
      snapshot.providers.push({ providerId: language.providers[0], availability: 'disabled', notice: 'Off in Settings', phase: 'idle', coverage: { kind: 'openedDocuments', roots: [], partial: false, externalChanges: 'unwatched' }, freshness: 'uncertain', truncated: false });
    }
    this.snapshots.set(context.attachmentId, snapshot);
    return structuredClone({ project: { hostId: host, root: dir }, context, snapshot });
  }
  async detachCodeDiagnostics(c: CodeDiagnosticsContext, host: string) { this.get(c, host); this.snapshots.delete(c.attachmentId); this.documents.delete(c.attachmentId); this.grants.delete(c.attachmentId); return { context: c }; }
  private docs(c: CodeDiagnosticsContext, host: string) {
    const s = this.get(c, host);
    if (!this.liveDocuments) throw new CodeDiagnosticsError('unsupported', 'Mock live diagnostics are unavailable');
    if (s.approval.state !== 'approved') throw new CodeDiagnosticsError('approvalRequired', 'Approve mock checker first');
    let m = this.documents.get(c.attachmentId);
    if (!m) this.documents.set(c.attachmentId, m = new Map());
    return m;
  }
  async openCodeDiagnosticDocument(c: CodeDiagnosticsContext, path: string, documentId: string, version: 1, text: string, _revision: string | undefined, host: string): Promise<CodeDiagnosticsDocumentReceipt> {
    const m = this.docs(c, host);
    if (version !== 1 || !documentId || m.has(documentId) || this.closed.has(documentId) || [...m.values()].some(d => d.path === path)) throw new CodeDiagnosticsError('invalidArgument', 'Invalid mock document open');
    m.set(documentId, { path, version, text });
    return { context: c, documentId, version };
  }
  async changeCodeDiagnosticDocument(c: CodeDiagnosticsContext, documentId: string, version: number, text: string, host: string): Promise<CodeDiagnosticsDocumentReceipt> {
    const d = this.docs(c, host).get(documentId);
    if (!d) throw new CodeDiagnosticsError('notFound', 'Mock document is not open');
    if (version < d.version || (version === d.version && text !== d.text)) throw new CodeDiagnosticsError('invalidArgument', 'Mock document version conflict');
    d.version = version; d.text = text;
    return { context: c, documentId, version };
  }
  async saveCodeDiagnosticDocument(c: CodeDiagnosticsContext, documentId: string, savedVersion: number, _revision: string, host: string): Promise<CodeDiagnosticsDocumentReceipt> {
    const d = this.docs(c, host).get(documentId);
    if (!d) throw new CodeDiagnosticsError('notFound', 'Mock document is not open');
    if (savedVersion > d.version) throw new CodeDiagnosticsError('invalidArgument', 'Mock saved version is unknown');
    return { context: c, documentId };
  }
  async closeCodeDiagnosticDocument(c: CodeDiagnosticsContext, documentId: string, host: string): Promise<CodeDiagnosticsDocumentReceipt> {
    const m = this.docs(c, host);
    if (m.delete(documentId)) this.closed.add(documentId);
    this.answers.delete(documentId);
    return { context: c, documentId };
  }
  /**
   * Suggests the words of the open mirrors that start with the word before
   * the position, replacing that word. A declared name resolves to its
   * declaring line, as navigateCode finds it.
   */
  async completeCode(c: CodeDiagnosticsContext, _requestId: string, documentId: string, version: number, position: CodeDiagnosticPosition, trigger: CodeCompletionTrigger, host: string): Promise<CodeCompletionResult> {
    const m = this.docs(c, host), d = m.get(documentId);
    if (!d) throw new CodeDiagnosticsError('notFound', 'Mock document is not open');
    if (version !== d.version) throw new CodeDiagnosticsError('staleVersion', 'The document changed since this request');
    const line = d.text.split('\n')[position.line];
    if (line === undefined || position.character > line.length) throw new CodeDiagnosticsError('invalidArgument', 'Position is outside the document');
    // Like a server whose only trigger character is '.'.
    const triggerCharacters = '.';
    if (trigger.kind === 'character' && !triggerCharacters.includes(trigger.character)) return { documentId, version, items: [], triggerCharacters };
    const prefix = /\w*$/.exec(line.slice(0, position.character))![0];
    const range = { start: { line: position.line, character: position.character - prefix.length }, end: position };
    const found = new Map<string, { path: string; declared?: string }>();
    for (const doc of m.values()) {
      for (const text of doc.text.split('\n')) {
        for (const [word] of text.matchAll(/\b[A-Za-z_]\w{2,}\b/g)) {
          if (word === prefix || !word.toLowerCase().startsWith(prefix.toLowerCase())) continue;
          const declared = new RegExp(`\\b(?:func|function|const|let|var|def|class|type)\\s+${word}\\b`).test(text) ? text.trim() : undefined;
          const seen = found.get(word);
          if (!seen || (declared && !seen.declared)) found.set(word, { path: doc.path, declared });
        }
      }
    }
    const words = [...found.keys()].sort();
    const items = words.slice(0, 100).map(word => {
      const { path, declared } = found.get(word)!;
      // A declared function goes in as a snippet with its parameters as tab stops.
      const params = declared && new RegExp(`\\b${word}\\s*\\(([^)]*)\\)`).exec(declared)?.[1];
      const snippet = params === undefined ? undefined
        : `${word}(${params.split(',').map(p => p.trim()).filter(Boolean).map((p, i) => `\${${i + 1}:${p.replace(/[$}\\]/g, '\\$&')}}`).join(', ')})$0`;
      return { item: { label: word, kind: declared ? 3 : 1, edit: { range, newText: snippet ?? word }, ...(snippet ? { snippet: true } : {}) }, path, declared };
    });
    const resultId = `mock-completion-${++this.serial}`;
    this.answers.set(documentId, { resultId, version, items });
    return { documentId, version, resultId, items: items.map(i => structuredClone(i.item)), triggerCharacters, ...(words.length > items.length ? { isIncomplete: true } : {}) };
  }
  /**
   * The kept answer survives changes. At the current version, a name declared
   * in another open document also resolves to an import of it, at the top.
   */
  async resolveCodeCompletion(c: CodeDiagnosticsContext, documentId: string, resultId: string, index: number, version: number, host: string): Promise<CodeCompletionDetails> {
    const d = this.docs(c, host).get(documentId);
    if (!d) throw new CodeDiagnosticsError('notFound', 'Mock document is not open');
    if (!Number.isInteger(version) || version < 0) throw new CodeDiagnosticsError('invalidArgument', 'Invalid document version');
    const answer = this.answers.get(documentId);
    if (!answer || answer.resultId !== resultId) throw new CodeDiagnosticsError('staleVersion', 'The completion answer is no longer kept');
    const kept = answer.items[index];
    if (!kept) throw new CodeDiagnosticsError('invalidArgument', 'No such completion item');
    if (version > 0 && version !== d.version) throw new CodeDiagnosticsError('staleVersion', 'The document changed since this request');
    const details: CodeCompletionDetails = { documentId, resultId, index, detail: kept.path, ...(kept.declared ? { documentation: { kind: 'markdown', value: '```\n' + kept.declared + '\n```' } } : {}) };
    if (version > 0 && kept.declared && kept.path !== d.path) {
      const from = './' + kept.path.replace(/^.*\//, '').replace(/\.[^.]*$/, '');
      const top = { line: 0, character: 0 };
      details.version = version;
      details.extraEdits = [{ range: { start: top, end: top }, newText: `import { ${kept.item.label} } from '${from}';\n` }];
    }
    return details;
  }
  async cancelCodeCompletion(_requestId: string): Promise<void> {}
  /**
   * Answers from the open mirrors: the word at the position, its declaring line
   * (`func`, `function`, `const`, `let`, `var`, `def`, `class`, `type`) and every use.
   */
  async navigateCode(c: CodeDiagnosticsContext, documentId: string, version: number, op: CodeNavigationOp, position: CodeDiagnosticPosition, host: string): Promise<CodeNavigationResult> {
    const m = this.docs(c, host), d = m.get(documentId);
    if (!d) throw new CodeDiagnosticsError('notFound', 'Mock document is not open');
    if (version !== d.version) throw new CodeDiagnosticsError('staleVersion', 'The document changed since this request');
    const line = d.text.split('\n')[position.line];
    if (line === undefined || position.character > line.length) throw new CodeDiagnosticsError('invalidArgument', 'Position is outside the document');
    const word = /\w*$/.exec(line.slice(0, position.character))![0] + /^\w*/.exec(line.slice(position.character))![0];
    const result: CodeNavigationResult = { op, documentId, version, locations: [] };
    if (!word) return result;
    const uses: CodeNavigationLocation[] = [], declarations: CodeNavigationLocation[] = [];
    let declared = '';
    const declaring = new RegExp(`\\b(?:func|function|const|let|var|def|class|type)\\s+${word}\\b`);
    const sources = [...m.values(), ...Object.entries(MOCK_EXTERNAL).map(([path, text]) => ({ path, text, external: true as const }))];
    for (const doc of sources) {
      const external = 'external' in doc;
      doc.text.split('\n').forEach((text, i) => {
        for (const hit of text.matchAll(new RegExp(`\\b${word}\\b`, 'g'))) {
          const place: CodeNavigationLocation = { path: doc.path, range: { start: { line: i, character: hit.index }, end: { line: i, character: hit.index + word.length } }, ...(external ? { external } : {}) };
          if (!external) uses.push(place);
          if (declaring.test(text) && !declarations.some(p => p.path === doc.path && p.range.start.line === i)) {
            declarations.push(place);
            declared ||= text.trim();
          }
        }
      });
    }
    if (op === 'references') result.locations = uses;
    else if (op === 'definition') result.locations = declarations;
    else if (declared) result.hover = { kind: 'markdown', value: '```\n' + declared + '\n```\n\nMock symbol `' + word + '`.' };
    let granted = this.grants.get(c.attachmentId);
    if (!granted) this.grants.set(c.attachmentId, granted = new Set());
    for (const l of result.locations) if (l.external) granted.add(l.path);
    return result;
  }
  async readNavigationFile(c: CodeDiagnosticsContext, path: string, host: string): Promise<FileContent> {
    this.docs(c, host);
    const text = MOCK_EXTERNAL[path];
    if (!this.grants.get(c.attachmentId)?.has(path) || text === undefined) throw new CodeDiagnosticsError('notFound', 'This file was not opened by code navigation');
    return { path, size: text.length, text, revision: `mock-external-${text.length}` };
  }
  async invalidateCodeDiagnosticsPath(c: CodeDiagnosticsContext, path: string, host: string) {
    const s = this.get(c, host);
    if (!path || path.length > 4096 || /[\\\\\0]/.test(path) || path.split('/').some(p => !p || p === '.' || p === '..')) throw new CodeDiagnosticsError('invalidArgument', 'Invalid path');
    for (const p of s.providers) if (p.phase === 'checking' || p.phase === 'starting') p.phase = 'canceled';
    for (const [id, d] of this.documents.get(c.attachmentId) ?? []) if (d.path === path) { this.documents.get(c.attachmentId)!.delete(id); this.closed.add(id); }
    s.resultSets = s.resultSets.filter(set => set.scope.kind !== 'document' || set.scope.path !== path).map(set => ({ ...set, diagnostics: set.diagnostics.filter(d => d.path !== path) }));
    return this.publish(s);
  }
  async getCodeDiagnosticsSnapshot(c: CodeDiagnosticsContext, host: string) { return structuredClone(this.get(c, host)); }
  async approveCodeDiagnostics(c: CodeDiagnosticsContext, mode: 'session' | 'persistent', fingerprint: string, host: string) {
    const s = this.get(c, host);
    if (fingerprint !== s.proposal?.fingerprint) throw new CodeDiagnosticsError('approvalChanged', 'Mock proposal changed');
    s.approval = { state: 'approved', mode, fingerprint }; s.providers[0].availability = 'available';
    return this.publish(s);
  }
  async revokeCodeDiagnostics(c: CodeDiagnosticsContext, host: string) {
    const s = this.get(c, host); s.context = { ...c, generation: `generation-${++this.serial}` }; s.approval = { state: 'revoked' }; this.grants.delete(c.attachmentId); s.resultSets = [];
    s.providers[0] = { ...s.providers[0], phase: 'idle', availability: 'approvalRequired', runId: undefined };
    return this.publish(s);
  }
  async runCodeDiagnostics(c: CodeDiagnosticsContext, ids: string[] | undefined, host: string) {
    const s = this.get(c, host);
    if (s.approval.state !== 'approved') throw new CodeDiagnosticsError('approvalRequired', 'Approve mock checker first');
    if (ids && (ids.length !== 1 || ids[0] !== 'mock-checker')) throw new CodeDiagnosticsError('invalidArgument', 'Unknown mock provider');
    const run = { providerId: 'mock-checker', runId: `run-${++this.serial}` };
    s.providers[0] = { ...s.providers[0], runId: run.runId, phase: 'checking' }; this.publish(s);
    // Complete on a later microtask, with the same ownership/latest-run guards as a real checker.
    queueMicrotask(() => {
      if (this.snapshots.get(c.attachmentId) !== s || !sameContext(c, s.context) || s.providers[0].runId !== run.runId || s.providers[0].phase !== 'checking') return;
      s.providers[0].phase = 'idle';
      s.resultSets = [{ ...run, origin: 'checker', scope: { kind: 'project' }, diagnostics: [{ path: 'example.go', severity: 'warning', message: 'Synthetic mock finding', source: 'mock-checker', freshness: 'uncertain', basis: { kind: 'saved' } }] }];
      this.publish(s);
    });
    return { context: c, runs: [run] };
  }
  async cancelCodeDiagnostics(c: CodeDiagnosticsContext, runs: CodeDiagnosticsRun[], host: string) {
    const s = this.get(c, host);
    for (const r of runs) if (s.providers[0].runId === r.runId && r.providerId === 'mock-checker') {
      s.providers[0].phase = 'canceled'; s.providers[0].freshness = 'stale';
      s.resultSets = s.resultSets.map(set => ({ ...set, diagnostics: set.diagnostics.map(d => ({ ...d, freshness: 'stale' })) }));
    }
    this.publish(s); return { context: c };
  }
}

import type { FileContent } from './types';

/** Stage 1 contract v1. Scanner/provider health diagnostics are unrelated. */
export interface CodeDiagnosticsContext { connectionId: string; attachmentId: string; generation: string }
export interface CodeDiagnosticsProject { hostId: string; root: string }
export interface CodeDiagnosticPosition { line: number; character: number }
export interface CodeDiagnosticRange { start: CodeDiagnosticPosition; end: CodeDiagnosticPosition }
export type CodeDiagnosticFreshness = 'current' | 'stale' | 'uncertain';
export interface CodeDiagnostic {
  path: string;
  severity: 'error' | 'warning' | 'information' | 'hint';
  message: string;
  source: string;
  freshness: CodeDiagnosticFreshness;
  basis: { kind: 'saved'; revision?: string } | { kind: 'document'; documentId: string; version?: number; observedVersion?: number };
  range?: CodeDiagnosticRange;
  code?: string;
  locationIssue?: 'invalidRange';
}
export interface CodeDiagnosticsResultSet {
  providerId: string;
  origin: 'checker' | 'lsp';
  scope: { kind: 'project' } | { kind: 'document'; path: string; documentId: string };
  diagnostics: CodeDiagnostic[];
  runId?: string;
}
export interface CodeDiagnosticsProvider {
  providerId: string;
  availability: 'approvalRequired' | 'available' | 'missingTool' | 'unsupported' | 'disabled';
  phase: 'idle' | 'starting' | 'checking' | 'failed' | 'canceled' | 'disconnected';
  coverage: { kind: 'savedProject' | 'openedDocuments'; roots: string[]; partial: boolean; externalChanges: 'reconciled' | 'unwatched' | 'watching' };
  /** Opened-documents providers: file extensions they synchronize (additive; absent on older backends). */
  documentExtensions?: string[];
  /** Some nested projects lack this server even if other projects can use it. */
  missingTool?: boolean;
  freshness: CodeDiagnosticFreshness;
  truncated: boolean;
  runId?: string;
  notice?: string;
  failure?: { code: string; message: string };
}
export interface CodeDiagnosticsSnapshot {
  contractVersion: 1;
  context: CodeDiagnosticsContext;
  sequence: number;
  approval: { state: 'required' | 'approved' | 'revoked'; mode?: 'session' | 'persistent'; fingerprint?: string };
  providers: CodeDiagnosticsProvider[];
  resultSets: CodeDiagnosticsResultSet[];
  /** `analyzers` are built-in in-process checks gated by the same approval; they run no program. */
  proposal?: {
    fingerprint: string;
    tools: { providerId: string; executable: string; configPaths: string[]; buildContext?: string }[];
    analyzers?: { providerId: string; description: string }[];
    /** Backend-owned setup covered by the same approval; names/commands never execute here. */
    preparation?: { name: string; command: string }[];
    outputs?: string[];
  };
  /** Backend-owned setup progress/failure. A non-nil value must never activate tools. */
  preparation?: { state: 'running' | 'failed' | 'canceled'; step?: string; index?: number; total?: number; message?: string };
}
export interface CodeDiagnosticsAttachment {
  project: CodeDiagnosticsProject;
  context: CodeDiagnosticsContext;
  snapshot: CodeDiagnosticsSnapshot;
}
export interface CodeDiagnosticsUpdate {
  contractVersion: 1;
  context: CodeDiagnosticsContext;
  sequence: number;
  updateId: string;
  partIndex: number;
  partCount: number;
  snapshot: CodeDiagnosticsSnapshot;
}
export interface CodeDiagnosticsRun { providerId: string; runId: string }
export interface CodeDiagnosticsAck { context: CodeDiagnosticsContext }
export interface CodeDiagnosticsRuns extends CodeDiagnosticsAck { runs: CodeDiagnosticsRun[] }
/** Document operations return identity only, never text. */
export interface CodeDiagnosticsDocumentReceipt { context: CodeDiagnosticsContext; documentId: string; version?: number }

export type CodeNavigationOp = 'definition' | 'references' | 'hover';
/**
 * One place an answer points to: a project path, or with `external` the
 * absolute path on the host (a library or the toolchain). UTF-16 range.
 */
export interface CodeNavigationLocation { path: string; range: CodeDiagnosticRange; external?: boolean }
export interface CodeHoverText { kind: 'markdown' | 'plaintext'; value: string; range?: CodeDiagnosticRange }
/** The answer for the document at `version`; drop it when the document moved on. */
export interface CodeNavigationResult {
  op: CodeNavigationOp;
  documentId: string;
  version: number;
  locations: CodeNavigationLocation[];
  hover?: CodeHoverText;
  truncated?: boolean;
}

/**
 * Why completion was asked: on request, after one of the server's trigger
 * characters, or again after an incomplete answer.
 */
export type CodeCompletionTrigger = { kind: 'invoked' | 'incomplete' } | { kind: 'character'; character: string };
export interface CodeTextEdit { range: CodeDiagnosticRange; newText: string }
export interface CodeMarkupText { kind: 'markdown' | 'plaintext'; value: string }
/**
 * One suggestion. `edit` replaces a range on the asked line (otherwise
 * `insertText`, or the label, goes in at the word); `extraEdits` change other
 * lines (imports). `kind` is the LSP CompletionItemKind (1-25).
 */
export interface CodeCompletionItem {
  label: string;
  labelDetail?: string;
  labelDescription?: string;
  kind?: number;
  detail?: string;
  documentation?: CodeMarkupText;
  sortText?: string;
  filterText?: string;
  insertText?: string;
  edit?: CodeTextEdit;
  snippet?: boolean;
  extraEdits?: CodeTextEdit[];
  deprecated?: boolean;
}
/**
 * The answer for the document at `version`. `resultId` names it for
 * resolveCodeCompletion while it is the document's last answer, typing on
 * included; an
 * incomplete answer is asked again as the user types on.
 */
export interface CodeCompletionResult {
  documentId: string;
  version: number;
  resultId?: string;
  /**
   * The answering server resolves items, so one may gain extra edits
   * (imports) then. Absent from hosts before CC-20, whose answers are taken
   * as not resolving.
   */
  resolves?: boolean;
  items: CodeCompletionItem[];
  isIncomplete?: boolean;
  truncated?: boolean;
  /**
   * The answering server's trigger characters; a character it did not name
   * is answered with nothing. Empty when it names none; absent from hosts
   * before CC-20.
   */
  triggerCharacters?: string;
}
/**
 * An item's detail and documentation, from completionItem/resolve. Resolved
 * at a version, also the item's extra edits (imports) the server computes
 * only then, for that version's text: `version` and `extraEdits` come
 * together or not at all. A host from before CC-10 never sends them.
 */
export interface CodeCompletionDetails {
  documentId: string;
  resultId: string;
  index: number;
  detail?: string;
  documentation?: CodeMarkupText;
  truncated?: boolean;
  version?: number;
  extraEdits?: CodeTextEdit[];
}

/** What the bridge confirmed for a displayed document; the store keeps no text. */
export interface LiveDocumentBasis { documentId: string; version: number; synced: boolean }

/**
 * Whether a document-basis finding may be drawn over the displayed text.
 * `current`: same lifecycle, version equals the confirmed unchanged version and the
 * producer verified it. `observed`: an unversioned TypeScript position bound to
 * the mirror at receipt, drawable but still uncertain. `uncertain`: panel only.
 * `hidden`: another lifecycle or an older/newer version (never reuse stale ranges).
 */
export function liveDocumentEligibility(d: CodeDiagnostic, doc: LiveDocumentBasis): 'current' | 'observed' | 'uncertain' | 'hidden' {
  const b = d.basis;
  if (b.kind !== 'document' || b.documentId !== doc.documentId) return 'hidden';
  if (b.version === undefined) {
    if (b.observedVersion === undefined) return 'uncertain';
    if (b.observedVersion !== doc.version) return 'hidden';
    return doc.synced && d.freshness === 'uncertain' && !!d.range && !d.locationIssue ? 'observed' : 'uncertain';
  }
  if (b.version !== doc.version) return b.version < doc.version ? 'hidden' : 'uncertain';
  if (!doc.synced || d.freshness !== 'current' || !d.range || d.locationIssue) return 'uncertain';
  return 'current';
}

/** All attached calls carry stable host identity, never a picker alias. */
export interface CodeDiagnosticsAPI {
  /** disabledLanguages are the language IDs that are off in Settings. */
  attachCodeDiagnostics(dir: string, contractVersion: 1, expectedHostId: string, disabledLanguages?: string[]): Promise<CodeDiagnosticsAttachment>;
  detachCodeDiagnostics(context: CodeDiagnosticsContext, expectedHostId: string): Promise<CodeDiagnosticsAck>;
  getCodeDiagnosticsSnapshot(context: CodeDiagnosticsContext, expectedHostId: string): Promise<CodeDiagnosticsSnapshot>;
  approveCodeDiagnostics(context: CodeDiagnosticsContext, mode: 'session' | 'persistent', fingerprint: string, expectedHostId: string): Promise<CodeDiagnosticsSnapshot>;
  revokeCodeDiagnostics(context: CodeDiagnosticsContext, expectedHostId: string): Promise<CodeDiagnosticsSnapshot>;
  runCodeDiagnostics(context: CodeDiagnosticsContext, providerIds: string[] | undefined, expectedHostId: string): Promise<CodeDiagnosticsRuns>;
  cancelCodeDiagnostics(context: CodeDiagnosticsContext, runs: CodeDiagnosticsRun[], expectedHostId: string): Promise<CodeDiagnosticsAck>;
  invalidateCodeDiagnosticsPath?(context: CodeDiagnosticsContext, path: string, expectedHostId: string): Promise<CodeDiagnosticsSnapshot>;
  /** Stage 2 (liveCodeDiagnostics only). Bodies are never logged or persisted. */
  openCodeDiagnosticDocument(context: CodeDiagnosticsContext, path: string, documentId: string, version: 1, text: string, revision: string | undefined, expectedHostId: string): Promise<CodeDiagnosticsDocumentReceipt>;
  changeCodeDiagnosticDocument(context: CodeDiagnosticsContext, documentId: string, version: number, text: string, expectedHostId: string): Promise<CodeDiagnosticsDocumentReceipt>;
  saveCodeDiagnosticDocument(context: CodeDiagnosticsContext, documentId: string, savedVersion: number, revision: string, expectedHostId: string): Promise<CodeDiagnosticsDocumentReceipt>;
  closeCodeDiagnosticDocument(context: CodeDiagnosticsContext, documentId: string, expectedHostId: string): Promise<CodeDiagnosticsDocumentReceipt>;
  /** codeNavigation only: asks about the open document at exactly `version`. */
  navigateCode(context: CodeDiagnosticsContext, documentId: string, version: number, op: CodeNavigationOp, position: CodeDiagnosticPosition, expectedHostId: string): Promise<CodeNavigationResult>;
  /**
   * codeNavigation only: reads, for viewing, an external file (absolute host
   * path) a navigation answer of this attachment pointed to. Never editable.
   */
  readNavigationFile(context: CodeDiagnosticsContext, path: string, expectedHostId: string): Promise<FileContent>;
  /**
   * codeCompletion only: suggestions for the open document at exactly
   * `version`. `requestId` names the request for cancelCodeCompletion; a new
   * request under the same ID cancels the old one.
   */
  completeCode(context: CodeDiagnosticsContext, requestId: string, documentId: string, version: number, position: CodeDiagnosticPosition, trigger: CodeCompletionTrigger, expectedHostId: string): Promise<CodeCompletionResult>;
  /**
   * codeCompletion only: item `index` of the document's last answer,
   * `resultId`, kept through later changes. `version` 0 asks for details
   * only; the document's current version also asks for the item's extra
   * edits, and rejects with `staleVersion` when the document is at another.
   */
  resolveCodeCompletion(context: CodeDiagnosticsContext, documentId: string, resultId: string, index: number, version: number, expectedHostId: string): Promise<CodeCompletionDetails>;
  /** Ends the request started under `requestId`, which rejects with `canceled`. A finished or unknown ID is a no-op. */
  cancelCodeCompletion(requestId: string): Promise<void>;
}

export const CODE_DIAGNOSTICS_EVENT = 'codeDiagnostics:updated';
export const CODE_DIAGNOSTICS_LIMITS = Object.freeze({ snapshotBytes: 8 * 1024 * 1024, partBytes: 1024 * 1024, bufferBytes: 16 * 1024 * 1024, findings: 10000, parts: 32, updates: 2, timeout: 5000, retries: 2 });

export class CodeDiagnosticsError extends Error {
  code: string;
  constructor(code: string, message: string) { super(message); this.code = code; }
}

/** Wails may serialize Go errors as strings; retain stable codes where provided. */
export function codeDiagnosticsError(error: unknown): CodeDiagnosticsError {
  if (error instanceof CodeDiagnosticsError) return error;
  const e = error as { code?: string; message?: string; data?: { code?: string } };
  const message = typeof error === 'string' ? error : e?.message ?? 'Code diagnostics request failed';
  return new CodeDiagnosticsError(e?.data?.code ?? e?.code ?? (/^(unsupported|disconnected|unauthorized|approvalRequired|approvalChanged|staleContext|invalidArgument|notFound|busy|limitExceeded|resyncRequired|toolFailed|canceled|notRunning|staleVersion|timeout):/.exec(message)?.[1]) ?? 'toolFailed', message);
}

export function sameAttachment(a: CodeDiagnosticsContext, b: CodeDiagnosticsContext): boolean {
  return a.connectionId === b.connectionId && a.attachmentId === b.attachmentId;
}
export function sameContext(a: CodeDiagnosticsContext, b: CodeDiagnosticsContext): boolean {
  return sameAttachment(a, b) && a.generation === b.generation;
}
/** Account for Go encoding/json HTML escaping, not JS's smaller wire estimate. */
export function encodedBytes(value: unknown): number {
  return new TextEncoder().encode(JSON.stringify(value).replace(/[<>&\u2028\u2029]/g, c => '\\u' + c.charCodeAt(0).toString(16).padStart(4, '0'))).length;
}
export function resultSetKey(set: CodeDiagnosticsResultSet): string {
  return JSON.stringify([set.providerId, set.origin, set.scope.kind, set.scope.kind === 'document' ? set.scope.path : '', set.scope.kind === 'document' ? set.scope.documentId : '', set.runId]);
}
export function diagnosticKey(d: CodeDiagnostic): string {
  return JSON.stringify([d.path, d.range ? [d.range.start.line, d.range.start.character, d.range.end.line, d.range.end.character] : null, d.severity, d.message, d.code]);
}
export interface AttributedCodeDiagnostic { diagnostic: CodeDiagnostic; entries: { providerId: string; diagnostic: CodeDiagnostic }[] }
export function groupCodeDiagnostics(sets: readonly CodeDiagnosticsResultSet[]): AttributedCodeDiagnostic[] {
  const groups = new Map<string, AttributedCodeDiagnostic>();
  for (const set of sets) for (const diagnostic of set.diagnostics) {
    const key = diagnosticKey(diagnostic);
    const group = groups.get(key) ?? { diagnostic, entries: [] };
    if (!groups.has(key)) groups.set(key, group);
    // Keep provider-owned originals (including differing bases/freshness) intact.
    // Only the visible equivalence group is deduplicated, not its attribution.
    group.entries.push({ providerId: set.providerId, diagnostic });
  }
  return [...groups.values()];
}
/** Eligibility only; renderer must additionally validate ranges against basis text. */
export function inlineCodeDiagnostic(d: CodeDiagnostic, saved: { dirty: boolean; revision?: string }): boolean {
  return !!d.range && !d.locationIssue && d.freshness === 'current' && d.basis.kind === 'saved' && !saved.dirty && !!saved.revision && d.basis.revision === saved.revision;
}

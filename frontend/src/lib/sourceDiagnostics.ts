import type { AttributedCodeDiagnostic } from './codeDiagnostics';
import { fileLevelFinding, problemLocation, problemLocationStatus, sourceRangeOffsets, type DiagnosticDisplayBasis, type LocationKind } from './problemNavigation';

export interface SourceDiagnosticSegment { line: number; start: number; end: number; finding: number }
/** `kind` lets the list tell file-level findings apart from results that are actually stale. */
export interface SourceDiagnosticMessage { finding: AttributedCodeDiagnostic; notice: string; kind?: LocationKind }
export interface SourceDiagnosticProjection { messages: SourceDiagnosticMessage[]; segments: SourceDiagnosticSegment[]; text?: string }

/** Project verified saved or current versioned live ranges. Ends are exclusive; zero-width and empty
 * lines retain a gutter location without inventing source characters. */
export function projectSourceDiagnostics(text: string, findings: AttributedCodeDiagnostic[], saved: DiagnosticDisplayBasis, current = true): SourceDiagnosticProjection {
  if (!findings.length) return { messages: [], segments: [], text };
  const offsets = sourceRangeOffsets(text);
  const lines = text.split(/\r\n|\n/);
  const segments: SourceDiagnosticSegment[] = [];
  const messages = findings.map((f): SourceDiagnosticMessage => {
    const eligible = current && f.entries.find(e => !problemLocation(e.diagnostic, saved));
    const finding = eligible ? { ...f, diagnostic: eligible.diagnostic } : f;
    const d = finding.diagnostic;
    // A whole-file finding stays file-level whatever the attachment state; it never had a position.
    if (!current && !fileLevelFinding(d)) return { finding, kind: 'unverified', notice: 'Code checks are reconnecting or updating; position not shown until results are current.' };
    let { kind, notice } = problemLocationStatus(d, saved);
    if (!notice && (!d.range || !offsets(d.range))) { kind = 'invalidRange'; notice = 'Position is outside the displayed text; shown in the list only.'; }
    return { finding, notice, kind };
  });
  messages.forEach(({ finding, notice }, findingIndex) => {
    if (notice) return;
    const range = finding.diagnostic.range!;
    for (let line = range.start.line; line <= range.end.line; line++) {
      const start = line === range.start.line ? range.start.character : 0;
      const end = line === range.end.line ? range.end.character : lines[line].length;
      // An exclusive end at column zero belongs only to the preceding line.
      if (line === range.end.line && end === 0 && line !== range.start.line) break;
      segments.push({ line, start, end, finding: findingIndex });
    }
  });
  return { messages, segments, text };
}

/** CSS Highlights paint DOM text ranges without changing syntax tokens, find
 * offsets or native selection. Unsupported WebViews retain gutters/messages. */
export function paintSourceDiagnostics(root: HTMLElement, projection: SourceDiagnosticProjection, prefix: string): () => void {
  const view = root.ownerDocument.defaultView as (Window & { Highlight?: new (...ranges: Range[]) => unknown; CSS?: { highlights?: Map<string, unknown> } }) | null;
  const registry = view?.CSS?.highlights;
  const HighlightClass = view?.Highlight;
  if (!registry || !HighlightClass) return () => {};
  const names: string[] = [];
  const expectedLines = projection.text?.split('\n');
  // Multiple findings can cover the same syntax-highlighted line. Resolve its
  // element and text nodes once per paint, not once for every overlapping range.
  const linePoints = new Map<number, ((offset: number) => [Node, number] | undefined) | undefined>();
  const pointsFor = (line: number) => {
    if (linePoints.has(line)) return linePoints.get(line);
    const el = root.querySelector<HTMLElement>(`[data-source-line="${line + 1}"]`);
    const expected = expectedLines?.[line];
    if (!el || (expected !== undefined && el.textContent !== expected)) {
      linePoints.set(line, undefined);
      return undefined;
    }
    const walker = root.ownerDocument.createTreeWalker(el, 4);
    const nodes: Node[] = [];
    let node: Node | null;
    while ((node = walker.nextNode())) nodes.push(node);
    const point = (offset: number): [Node, number] | undefined => {
      for (const n of nodes) {
        const length = n.textContent?.length ?? 0;
        if (offset <= length) return [n, offset];
        offset -= length;
      }
    };
    linePoints.set(line, point);
    return point;
  };
  for (const severity of ['error', 'warning', 'information', 'hint']) {
    const ranges: Range[] = [];
    // A severity has one paint style, regardless of how many findings overlap.
    // Keep attribution in messages/gutters, not duplicate native DOM ranges:
    // WebKit gets very slow replacing source nodes with thousands of live ranges.
    const segments = projection.segments
      .filter(s => projection.messages[s.finding].finding.diagnostic.severity === severity && s.start !== s.end)
      .sort((a, b) => a.line - b.line || a.start - b.start || a.end - b.end);
    const merged: { line: number; start: number; end: number }[] = [];
    for (const segment of segments) {
      const previous = merged.at(-1);
      if (previous && previous.line === segment.line && segment.start <= previous.end) previous.end = Math.max(previous.end, segment.end);
      else merged.push({ line: segment.line, start: segment.start, end: segment.end });
    }
    for (const segment of merged) {
      const point = pointsFor(segment.line);
      if (!point) continue;
      const start = point(segment.start), end = point(segment.end);
      if (!start || !end) continue;
      const range = root.ownerDocument.createRange();
      range.setStart(...start); range.setEnd(...end); ranges.push(range);
    }
    const name = `${prefix}-${severity}`;
    names.push(name); registry.set(name, new HighlightClass(...ranges));
  }
  return () => { for (const name of names) registry.delete(name); };
}

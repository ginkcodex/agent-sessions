import type { CodeDiagnosticRange } from './codeDiagnostics';
import type { SourceDiagnosticProjection } from './sourceDiagnostics';

export type ProblemSeverity = 'error' | 'warning' | 'information' | 'hint';

/** A problem drawn in the source: in the viewer's paint or the editor's lint state. */
export interface OverviewProblem {
  range: CodeDiagnosticRange;
  severity: ProblemSeverity;
  message: string;
  /** The checker, and the code it gave, for the tooltip. */
  source: string;
}

/** One mark on the overview ruler: the problems starting on one line. */
export interface OverviewMark {
  line: number;
  /** The worst of its problems. */
  severity: ProblemSeverity;
  /** Worst first. */
  problems: OverviewProblem[];
}

export const severityRank: Record<ProblemSeverity, number> = { error: 0, warning: 1, information: 2, hint: 3 };

/** Problems the viewer paints; file-level, stale and unverified ones have no place in the source. */
export function sourceProblems(projection: SourceDiagnosticProjection): OverviewProblem[] {
  const painted = new Set(projection.segments.map(s => s.finding));
  return projection.messages.flatMap(({ finding }, i) => {
    const d = finding.diagnostic;
    if (!painted.has(i) || !d.range) return [];
    const sources = [...new Set(finding.entries.map(e => e.diagnostic.source).filter(Boolean))].join(', ');
    return [{ range: d.range, severity: d.severity, message: d.message, source: [sources, d.code].filter(Boolean).join(' · ') }];
  });
}

/** One mark per line a problem starts on, in line order. */
export function overviewMarks(problems: OverviewProblem[]): OverviewMark[] {
  const lines = new Map<number, OverviewProblem[]>();
  for (const p of problems) {
    const list = lines.get(p.range.start.line);
    if (list) list.push(p);
    else lines.set(p.range.start.line, [p]);
  }
  return [...lines].sort(([a], [b]) => a - b).map(([line, list]) => {
    const sorted = [...list].sort((a, b) => severityRank[a.severity] - severityRank[b.severity]);
    return { line, severity: sorted[0].severity, problems: sorted };
  });
}

/** Problems whose range touches columns [start, end) of a line; a zero-width one, its point. */
export function problemsAt(problems: OverviewProblem[], line: number, start: number, end: number): OverviewProblem[] {
  return problems.filter(({ range: r }) => {
    if (line < r.start.line || line > r.end.line) return false;
    // An exclusive end at column zero belongs only to the lines before.
    if (line === r.end.line && line !== r.start.line && r.end.character === 0) return false;
    const from = line === r.start.line ? r.start.character : 0;
    const to = line === r.end.line ? r.end.character : Infinity;
    return from === to ? from >= start && from <= end : from < end && to > start;
  }).sort((a, b) => severityRank[a.severity] - severityRank[b.severity]);
}

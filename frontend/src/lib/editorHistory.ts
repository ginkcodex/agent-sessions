import type { CodeDiagnosticRange } from './codeDiagnostics';

/**
 * Where the user was in Go to File, for back and forward: the caret or
 * selection when there is one, else the file as it was shown, rendered
 * (preview) or as source.
 */
export interface EditorPlace { path: string; external?: boolean; range?: CodeDiagnosticRange; preview?: boolean }

export const SOURCE_HISTORY_LIMIT = 50;

/**
 * Back and forward through the places the user went, for one project.
 * `visit` records where the user left from; going back records where they
 * are, so forward returns there.
 */
export class SourceHistory {
  private back: EditorPlace[] = [];
  private forward: EditorPlace[] = [];
  private readonly limit: number;

  constructor(limit = SOURCE_HISTORY_LIMIT) { this.limit = limit; }

  get canGoBack(): boolean { return this.back.length > 0; }
  get canGoForward(): boolean { return this.forward.length > 0; }

  /** Leaving from for somewhere else: forward is dropped. */
  visit(from: EditorPlace): void {
    const last = this.back[this.back.length - 1];
    if (!last || !samePlace(last, from)) this.back.push(from);
    if (this.back.length > this.limit) this.back.shift();
    this.forward = [];
  }

  goBack(current: EditorPlace | undefined): EditorPlace | undefined {
    return this.step(this.back, this.forward, current);
  }

  goForward(current: EditorPlace | undefined): EditorPlace | undefined {
    return this.step(this.forward, this.back, current);
  }

  /** A path closed for good (deleted): its places go too. */
  forget(keep: (location: EditorPlace) => boolean): void {
    this.back = mergeRuns(this.back.filter(keep));
    this.forward = mergeRuns(this.forward.filter(keep));
  }

  /** A path renamed: its places follow it. External places never move. */
  remap(move: (path: string) => string): void {
    const remap = (places: EditorPlace[]) => mergeRuns(places.map(p => {
      const path = p.external ? p.path : move(p.path);
      return path === p.path ? p : { ...p, path };
    }));
    this.back = remap(this.back);
    this.forward = remap(this.forward);
  }

  clear(): void { this.back = []; this.forward = []; }

  private step(from: EditorPlace[], to: EditorPlace[], current: EditorPlace | undefined): EditorPlace | undefined {
    const next = from.pop();
    if (next && current) {
      to.push(current);
      if (to.length > this.limit) to.shift();
    }
    return next;
  }
}

function samePlace(a: EditorPlace, b: EditorPlace): boolean {
  return a.path === b.path && Boolean(a.external) === Boolean(b.external) && Boolean(a.preview) === Boolean(b.preview)
    && a.range?.start.line === b.range?.start.line && a.range?.start.character === b.range?.start.character;
}

/** Neighbours that became the same place (a forgotten one between them) merge. */
function mergeRuns(places: EditorPlace[]): EditorPlace[] {
  return places.filter((p, i) => i === 0 || !samePlace(places[i - 1], p));
}

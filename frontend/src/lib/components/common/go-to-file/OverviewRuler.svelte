<script lang="ts">
  import { severityLabels } from '../../../codeCheckLabels';
  import { severityRank, type OverviewMark, type OverviewProblem } from '../../../problemOverview';

  // The problems of the whole file at a glance, as in VS Code and Zed: a mark
  // on the right edge of the scrolled source for each line with a problem, at
  // the line's place in the file. It lies over the vertical scrollbar and
  // lets the scrollbar have every pointer event its marks do not take.
  let { marks, scroller, measurer, onpick }: {
    marks: OverviewMark[];
    /** The element that scrolls the source. */
    scroller: HTMLElement | undefined;
    /** Called once per layout: where a 0-based line starts, in px from the top of the scrolled content. */
    measurer: () => (line: number) => number | undefined;
    onpick: (problem: OverviewProblem) => void;
  } = $props();

  const TOOLTIP_PROBLEMS = 5;
  let host: HTMLDivElement | undefined = $state();
  let box = $state({ top: 0, height: 0, right: 0, width: 0 });
  // Each mark's offset in the ruler, by line; a line not measured has none.
  let tops = $state.raw(new Map<number, number>());
  let hovered = $state<OverviewMark>();
  // Errors drawn last, on top of a warning that lands on the same pixels.
  let drawn = $derived([...marks].sort((a, b) => severityRank[b.severity] - severityRank[a.severity]));

  function layout() {
    const parent = host?.offsetParent;
    if (!scroller || !host || !(parent instanceof HTMLElement)) return;
    const outer = parent.getBoundingClientRect(), rect = scroller.getBoundingClientRect();
    // A classic scrollbar's width; an overlay scrollbar (macOS) takes none.
    const bar = scroller.offsetWidth - scroller.clientWidth - scroller.clientLeft * 2;
    box = { top: rect.top - outer.top + scroller.clientTop, height: scroller.clientHeight, right: outer.right - rect.right + scroller.clientLeft, width: Math.max(10, bar) };
    const total = scroller.scrollHeight;
    const measure = measurer();
    const next = new Map<number, number>();
    for (const mark of marks) {
      const y = total > 0 ? measure(mark.line) : undefined;
      if (y !== undefined) next.set(mark.line, Math.min(1, Math.max(0, y / total)) * box.height);
    }
    tops = next;
  }

  $effect(() => {
    void marks; void measurer;
    const el = scroller;
    if (!el || !host) return;
    let frame = 0;
    const schedule = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(layout); };
    schedule();
    // The content's height changes with wrapping, highlighting and the editor's measuring.
    const observer = new ResizeObserver(schedule);
    observer.observe(el);
    for (const child of el.children) observer.observe(child);
    return () => { cancelAnimationFrame(frame); observer.disconnect(); };
  });

  function describe(mark: OverviewMark) {
    return `Line ${mark.line + 1}: ${mark.problems.map(p => `${severityLabels[p.severity]}: ${p.message}`).join('; ')}`;
  }
</script>

{#if marks.length}
  <div class="overview-ruler" bind:this={host} role="group" aria-label="Problems in this file"
    style:top="{box.top}px" style:height="{box.height}px" style:right="{box.right}px" style:width="{box.width}px">
    {#each drawn as mark (mark.line)}
      {@const top = tops.get(mark.line)}
      {#if top !== undefined}
        <button type="button" tabindex="-1" class="mark sev-{mark.severity}" style:top="{top}px" aria-label={describe(mark)}
          onpointerenter={() => (hovered = mark)} onpointerleave={() => { if (hovered === mark) hovered = undefined; }}
          onclick={() => onpick(mark.problems[0])}></button>
      {/if}
    {/each}
    {#if hovered && tops.has(hovered.line)}
      {@const top = tops.get(hovered.line)!}
      <div class="ruler-tip" class:below={top < 60} style:top="{top}px" aria-hidden="true">
        {#each hovered.problems.slice(0, TOOLTIP_PROBLEMS) as problem, i (i)}
          <div class="tip-row">
            <span class="sev sev-{problem.severity}">{severityLabels[problem.severity]}</span>
            <span class="loc">Ln {problem.range.start.line + 1}, Col {problem.range.start.character + 1}</span>
            <span class="msg">{problem.message}</span>
            {#if problem.source}<span class="src">{problem.source}</span>{/if}
          </div>
        {/each}
        {#if hovered.problems.length > TOOLTIP_PROBLEMS}
          <div class="more">+{hovered.problems.length - TOOLTIP_PROBLEMS} more on this line</div>
        {/if}
      </div>
    {/if}
  </div>
{/if}

<style>
  .overview-ruler {
    position: absolute;
    z-index: 3;
    pointer-events: none;
  }

  .mark {
    position: absolute;
    left: 1px;
    right: 1px;
    height: 4px;
    margin-top: -1px;
    padding: 0;
    border: 0;
    border-radius: 1px;
    pointer-events: auto;
    cursor: pointer;
    background: var(--mark);
  }

  .mark:hover {
    left: 0;
    right: 0;
    height: 6px;
    margin-top: -2px;
  }

  .mark.sev-error { --mark: #f85149; }
  .mark.sev-warning { --mark: #d29922; }
  .mark.sev-information { --mark: #2f81f7; }
  .mark.sev-hint { --mark: #8250df; }
  :global(:root:not([data-theme='dark'])) .mark.sev-error { --mark: #cf222e; }
  :global(:root:not([data-theme='dark'])) .mark.sev-warning { --mark: #bf8700; }

  /* Left of the ruler, at the mark: the scrollbar keeps its own pointer. */
  .ruler-tip {
    position: absolute;
    right: calc(100% + 6px);
    transform: translateY(-100%);
    margin-top: -4px;
    width: max-content;
    max-width: min(480px, 60vw);
    padding: 6px 8px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md, 6px);
    background: var(--bg-primary);
    box-shadow: var(--shadow-popover, 0 4px 16px rgba(0, 0, 0, 0.2));
    font-family: var(--font-sans);
    font-size: 12px;
    line-height: 1.45;
    color: var(--text-primary);
  }

  .ruler-tip.below {
    transform: none;
    margin-top: 8px;
  }

  .tip-row {
    display: grid;
    grid-template-columns: auto auto minmax(0, 1fr);
    column-gap: 8px;
    align-items: baseline;
  }

  .tip-row + .tip-row {
    margin-top: 4px;
  }

  .sev {
    padding: 0 6px;
    border-radius: 8px;
    font-size: 11px;
    font-weight: 600;
    line-height: 16px;
    white-space: nowrap;
  }

  /* The pills only: a mark shares the severity class. */
  .sev.sev-error { color: #f85149; background: rgba(248, 81, 73, 0.12); }
  .sev.sev-warning { color: #d29922; background: rgba(210, 153, 34, 0.12); }
  .sev.sev-information { color: #2f81f7; background: rgba(47, 129, 247, 0.12); }
  .sev.sev-hint { color: #8250df; background: rgba(130, 80, 223, 0.12); }
  :global(:root:not([data-theme='dark'])) .sev.sev-error { color: #cf222e; }
  :global(:root:not([data-theme='dark'])) .sev.sev-warning { color: #9a6700; }

  .loc {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-secondary);
    white-space: nowrap;
  }

  .msg {
    overflow-wrap: anywhere;
    white-space: pre-wrap;
    display: -webkit-box;
    -webkit-line-clamp: 4;
    line-clamp: 4;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }

  .src {
    grid-column: 3;
    font-size: 11px;
    color: var(--text-muted);
  }

  .more {
    margin-top: 4px;
    font-size: 11px;
    color: var(--text-secondary);
  }
</style>

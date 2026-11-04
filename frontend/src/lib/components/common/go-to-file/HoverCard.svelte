<script lang="ts">
  import { handleCopyCodeClick } from '../../../copycode';
  import { severityLabels } from '../../../codeCheckLabels';
  import type { OverviewProblem } from '../../../problemOverview';

  // What the language server says about the word under the pointer, below it
  // (above, near the bottom of the window). The text is the transcript's
  // Markdown, escaped; a long one scrolls. Problems on the word come first,
  // as in VS Code.
  let { html, truncated, problems = [], x, y, maxHeight, onenter, onleave }: {
    html: string;
    problems?: OverviewProblem[];
    truncated: boolean;
    /** The pointer, in viewport coordinates. */
    x: number;
    y: number;
    maxHeight: number;
    onenter: () => void;
    onleave: () => void;
  } = $props();

  const MARGIN = 4, GAP = 14;
  let card: HTMLDivElement | undefined = $state();
  let left = $state(0);
  let top = $state(0);

  $effect(() => {
    void html; void problems; void x; void y;
    if (!card) return;
    const { width, height } = card.getBoundingClientRect();
    left = Math.max(MARGIN, Math.min(x, window.innerWidth - width - MARGIN));
    const below = y + GAP;
    top = below + height > window.innerHeight - MARGIN ? Math.max(MARGIN, y - GAP / 2 - height) : below;
  });
</script>

<!-- Copy buttons in code blocks are buttons; the card itself only delegates their clicks. -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<div
  class="hover-card"
  role="tooltip"
  bind:this={card}
  style:left="{left}px"
  style:top="{top}px"
  style:max-height="{Math.max(80, maxHeight)}px"
  onpointerenter={onenter}
  onpointerleave={onleave}
  onclick={handleCopyCodeClick}
>
  {#each problems as problem, i (i)}
    <div class="hover-problem">
      <span class="sev sev-{problem.severity}">{severityLabels[problem.severity]}</span>
      <span class="msg">{problem.message}</span>
      {#if problem.source}<span class="src">{problem.source}</span>{/if}
    </div>
  {/each}
  {#if html}<div class="hover-body" class:after-problems={problems.length > 0}>{@html html}</div>{/if}
  {#if truncated}<div class="hover-truncated">The description was cut short.</div>{/if}
</div>

<style>
  .hover-card {
    position: fixed;
    z-index: 1000;
    width: max-content;
    max-width: min(560px, calc(100vw - 8px));
    overflow: auto;
    padding: 6px 10px;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: var(--bg-primary);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
    font-size: 12.5px;
    line-height: 1.5;
    color: var(--text-primary);
    word-break: break-word;
  }

  .hover-problem {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    column-gap: 8px;
    align-items: baseline;
  }

  .hover-problem + .hover-problem { margin-top: 4px; }

  .hover-problem .sev {
    padding: 0 6px;
    border-radius: 8px;
    font-size: 11px;
    font-weight: 600;
    line-height: 16px;
    white-space: nowrap;
  }

  .sev-error { color: #f85149; background: rgba(248, 81, 73, 0.12); }
  .sev-warning { color: #d29922; background: rgba(210, 153, 34, 0.12); }
  .sev-information { color: #2f81f7; background: rgba(47, 129, 247, 0.12); }
  .sev-hint { color: #8250df; background: rgba(130, 80, 223, 0.12); }
  :global(:root:not([data-theme='dark'])) .sev-error { color: #cf222e; }
  :global(:root:not([data-theme='dark'])) .sev-warning { color: #9a6700; }

  .hover-problem .msg { white-space: pre-wrap; }

  .hover-problem .src {
    grid-column: 2;
    font-size: 11px;
    color: var(--text-muted);
  }

  .hover-body.after-problems {
    margin-top: 6px;
    padding-top: 6px;
    border-top: 1px solid var(--border-color);
  }

  .hover-body :global(p) { margin: 0.4em 0; }
  .hover-body :global(p:first-child),
  .hover-body :global(.code-block-container:first-child) { margin-top: 0; }
  .hover-body :global(p:last-child),
  .hover-body :global(.code-block-container:last-child) { margin-bottom: 0; }
  .hover-body :global(.code-block-container) { margin: 0.4em 0; }
  .hover-body :global(ul),
  .hover-body :global(ol) { margin: 0.4em 0; padding-left: 1.4em; }
  .hover-body :global(.inline-code) {
    font-family: var(--font-mono);
    font-size: 0.9em;
    padding: 0 3px;
    border-radius: 3px;
    background-color: var(--bg-tertiary);
  }
  .hover-body :global(blockquote) {
    margin: 0.4em 0;
    padding-left: 0.8em;
    border-left: 2px solid var(--border-color);
  }
  .hover-body :global(a) { color: var(--accent-color); }
  .hover-body :global(.hover-plain) {
    margin: 0;
    font-family: var(--font-mono);
    white-space: pre-wrap;
  }

  .hover-truncated {
    margin-top: 4px;
    font-size: 11px;
    color: var(--text-secondary);
  }
</style>

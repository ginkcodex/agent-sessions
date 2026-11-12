<script lang="ts">
  import { renderMarkdown } from '../../../markdown';
  import { handleMessageClick } from '../../../messageFileClick';
  import { renderMermaid } from '../../../mermaid';
  import { theme } from '../../../stores/theme.svelte';

  interface Props {
    text: string;
    onFileLink?: (value: string) => void;
    /** Runs a shell block's command; shell blocks get a Run button when set. */
    onRunCommand?: (command: string) => void;
  }

  let { text, onFileLink, onRunCommand }: Props = $props();

  let renderedHtml = $derived(renderMarkdown(text, 'transcript', undefined, { runnable: !!onRunCommand }));
  let container = $state<HTMLDivElement>();

  $effect(() => {
    void renderedHtml;
    renderMermaid(container, theme.resolved);
  });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="text-part-container" bind:this={container} onclick={(event) => handleMessageClick(event, onFileLink, onRunCommand)}>
  {@html renderedHtml}
</div>

<style>
  .text-part-container {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-body));
    line-height: 1.6;
    color: var(--text-primary);
    word-break: break-word;
  }

  :global(.text-part-container p) {
    margin: 0.5em 0;
  }

  :global(.text-part-container p:first-child) {
    margin-top: 0;
  }

  :global(.text-part-container p:last-child) {
    margin-bottom: 0;
  }

  :global(.text-part-container h1, .text-part-container h2, .text-part-container h3, .text-part-container h4) {
    margin: 1.2em 0 0.45em 0;
    color: var(--text-primary);
    font-weight: 650;
    letter-spacing: -0.012em;
    line-height: 1.3;
  }

  :global(.text-part-container h1) { font-size: calc(var(--session-font-size, 14.5px) * var(--type-h1)); }
  :global(.text-part-container h2) { font-size: calc(var(--session-font-size, 14.5px) * var(--type-h2)); }
  :global(.text-part-container h3) { font-size: calc(var(--session-font-size, 14.5px) * var(--type-h3)); }

  :global(.text-part-container ul, .text-part-container ol) {
    margin: 0.5em 0;
    padding-left: 1.5em;
  }

  :global(.text-part-container li) {
    margin: 0.25em 0;
  }

  :global(.text-part-container blockquote) {
    margin: 0.8em 0;
    padding: 6px 12px;
    border-left: 3px solid var(--accent-color);
    background-color: var(--bg-secondary);
    color: var(--text-secondary);
    border-radius: 0 4px 4px 0;
  }

  :global(.text-part-container .inline-code) {
    font-family: var(--font-mono);
    font-size: calc(var(--type-inline-code) * 1em);
    padding: 1px 5px;
    border-radius: 5px;
    background-color: var(--segment-track);
    color: var(--text-primary);
  }

  :global(.text-part-container a) {
    color: var(--accent-color);
    text-decoration: underline;
  }

  :global(.text-part-container hr.markdown-divider) {
    border: none;
    border-top: 1px solid var(--border-color);
    margin: 1.2em 0;
  }

  :global(.text-part-container .table-container) {
    overflow-x: auto;
    margin: 0.8em 0;
  }

  :global(.text-part-container table) {
    width: 100%;
    border-collapse: collapse;
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-code));
    line-height: 1.5;
    font-variant-numeric: tabular-nums;
  }

  /* Hairlines between rows and columns, with no outer frame. Cells sit at the
     top, so a short cell lines up with the first line of a long one. */
  :global(.text-part-container th, .text-part-container td) {
    border-bottom: 1px solid var(--border-color);
    padding: 7px 12px;
    text-align: left;
    vertical-align: top;
    /* The inherited break-word lets columns shrink to one letter; keep whole
       words so each column fits its longest one and the table scrolls. */
    word-break: normal;
    overflow-wrap: normal;
  }

  :global(.text-part-container :is(th, td) + :is(th, td)) {
    border-left: 1px solid var(--border-color);
  }

  :global(.text-part-container th) {
    border-bottom-color: var(--border-strong);
    background-color: color-mix(in srgb, var(--text-primary) 5%, transparent);
    color: var(--text-secondary);
    font-weight: 600;
    vertical-align: bottom;
  }

  /* Faint banding helps the eye follow a row across wide columns. */
  :global(.text-part-container tbody tr:nth-child(even)) {
    background-color: color-mix(in srgb, var(--text-primary) 2.5%, transparent);
  }

  :global(.text-part-container tr:last-child td) {
    border-bottom: none;
  }

  :global(.text-part-container :is(th, td).nowrap) {
    white-space: nowrap;
  }

  :global(.text-part-container :is(th, td).align-center) {
    text-align: center;
  }

  :global(.text-part-container :is(th, td).align-right) {
    text-align: right;
  }

  :global(.text-part-container .code-block-container) {
    margin: 0.8em 0;
    border-radius: var(--radius-md);
    background-color: var(--code-bg);
    overflow: hidden;
  }

  :global(.text-part-container .code-block-header) {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 4px 6px 0 12px;
  }

  :global(.text-part-container .code-lang-tag) {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
    font-family: var(--font-mono);
    font-weight: 500;
    color: var(--text-muted);
    text-transform: lowercase;
  }

  :global(.text-part-container .copy-code-btn, .text-part-container .run-code-btn, .text-part-container .mermaid-toggle-btn, .text-part-container .mermaid-expand-btn) {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
    font-weight: 500;
    padding: 2px 8px;
    border-radius: var(--radius-sm);
    background-color: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  :global(.text-part-container .copy-code-btn:hover, .text-part-container .run-code-btn:hover, .text-part-container .mermaid-toggle-btn:hover, .text-part-container .mermaid-expand-btn:hover) {
    background-color: var(--hover-bg);
    color: var(--text-primary);
  }

  :global(.text-part-container .copy-code-btn.copied) {
    background-color: color-mix(in srgb, var(--status-live) 14%, transparent);
    color: var(--status-live);
  }

  :global(.text-part-container .code-block-actions) {
    display: flex;
    gap: 4px;
  }

  /* Mermaid sizes the SVG to the diagram; wide ones shrink to the column. */
  :global(.text-part-container .mermaid-diagram) {
    padding: 10px 12px;
    overflow-x: auto;
    text-align: center;
    background-color: transparent;
    cursor: zoom-in;
    /* Mermaid sizes labels without the text's break-word, which would
       otherwise wrap them narrower and clip them. */
    word-break: normal;
  }

  :global(.text-part-container .mermaid-diagram svg) {
    max-width: 100%;
    height: auto;
  }

  :global(.text-part-container pre) {
    margin: 0;
    padding: 8px 12px 12px;
    overflow-x: auto;
    background-color: transparent;
  }

  :global(.text-part-container code.hljs) {
    font-family: var(--font-mono);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-code));
    line-height: 1.45;
  }
</style>

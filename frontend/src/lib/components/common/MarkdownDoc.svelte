<script lang="ts">
  import { renderDocumentMarkdown } from '../../documentMarkdown';
  import { handleCopyCodeClick } from '../../copycode';
  import { renderMermaid } from '../../mermaid';
  import { theme } from '../../stores/theme.svelte';

  interface Props {
    source: string;
    onFileLink?: (href: string) => void;
    loadImage?: (href: string) => Promise<string | null>;
  }

  let { source, onFileLink, loadImage }: Props = $props();

  let renderedHtml = $derived(renderDocumentMarkdown(source));

  $effect(() => {
    void renderedHtml;
    const root = container;
    const load = loadImage;
    let cancelled = false;
    if (root && load) {
      for (const img of root.querySelectorAll<HTMLImageElement>('img[data-document-image]')) {
        void load(img.dataset.documentImage!).then((src) => {
          if (!cancelled && src) img.src = src;
        }).catch(() => { /* Missing or unavailable images retain their alt text. */ });
      }
    }
    return () => { cancelled = true; };
  });

  function scrollToAnchor(root: HTMLElement | undefined, id: string) {
    if (!root || !id) return;
    let target: HTMLElement | null = null;
    try {
      target = root.querySelector<HTMLElement>(`#${CSS.escape(decodeURIComponent(id))}`);
    } catch {
      target = null;
    }
    if (target) target.scrollIntoView({ block: 'start' });
  }

  function handleClick(e: MouseEvent) {
    const anchor = e.target instanceof Element ? e.target.closest<HTMLAnchorElement>('a[data-file-link]') : null;
    if (anchor && onFileLink) {
      e.preventDefault();
      const href = anchor.dataset.fileLink!;
      // A fragment-only link points inside this document; scroll to it here
      // rather than asking the viewer to open a file.
      if (href.startsWith('#')) {
        scrollToAnchor(container, href.slice(1));
        return;
      }
      onFileLink(href);
      return;
    }
    handleCopyCodeClick(e);
  }
  let container = $state<HTMLDivElement>();

  $effect(() => {
    void renderedHtml;
    renderMermaid(container, theme.resolved);
  });
</script>

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="markdown-doc" bind:this={container} onclick={handleClick}>
  {@html renderedHtml}
</div>

<style>
  .markdown-doc {
    font-size: var(--markdown-font-size, 13px);
    line-height: 1.6;
    color: var(--text-primary);
    word-break: break-word;
  }

  :global(.markdown-doc p) {
    margin: 0.5em 0;
  }

  :global(.markdown-doc p:first-child) {
    margin-top: 0;
  }

  :global(.markdown-doc p:last-child) {
    margin-bottom: 0;
  }

  :global(.markdown-doc h1, .markdown-doc h2, .markdown-doc h3, .markdown-doc h4) {
    margin: 1em 0 0.4em 0;
    color: var(--text-primary);
  }

  :global(.markdown-doc h1) { font-size: 1.428em; }
  :global(.markdown-doc h2) { font-size: 1.255em; }
  :global(.markdown-doc h3) { font-size: 1.083em; }

  :global(.markdown-doc ul, .markdown-doc ol) {
    margin: 0.5em 0;
    padding-left: 1.5em;
  }

  :global(.markdown-doc li) {
    margin: 0.25em 0;
  }

  :global(.markdown-doc blockquote) {
    margin: 0.8em 0;
    padding: 6px 12px;
    border-left: 3px solid var(--accent-color);
    background-color: var(--bg-secondary);
    color: var(--text-secondary);
    border-radius: 0 4px 4px 0;
  }

  /* Marked writes inline code as a bare <code> and sanitizing strips classes,
     so match any code outside a code block. Same look as the transcript's. */
  :global(.markdown-doc :not(pre) > code) {
    font-family: var(--font-mono);
    font-size: 0.825em;
    padding: 1px 5px;
    border-radius: 5px;
    background-color: var(--segment-track);
    color: var(--text-primary);
  }

  :global(.markdown-doc img) {
    max-width: 100%;
    object-fit: contain;
  }

  :global(.markdown-doc [align="center"]) { text-align: center; }
  :global(.markdown-doc [align="right"]) { text-align: right; }
  :global(.markdown-doc [align="left"]) { text-align: left; }

  :global(.markdown-doc a) {
    color: var(--accent-color);
    text-decoration: underline;
  }

  :global(.markdown-doc hr.markdown-divider) {
    border: none;
    border-top: 1px solid var(--border-color);
    margin: 1.2em 0;
  }

  :global(.markdown-doc .table-container) {
    overflow-x: auto;
    margin: 0.8em 0;
  }

  :global(.markdown-doc table) {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.923em;
  }

  :global(.markdown-doc th, .markdown-doc td) {
    border: 1px solid var(--border-color);
    padding: 4px 8px;
    text-align: left;
    /* A short cell lines up with the first line of a long one beside it. */
    vertical-align: top;
    /* The inherited break-word lets columns shrink to one letter; keep whole
       words so each column fits its longest one and the table scrolls. */
    word-break: normal;
    overflow-wrap: normal;
  }

  :global(.markdown-doc th) {
    background-color: var(--bg-secondary);
    font-weight: 600;
    vertical-align: bottom;
  }

  :global(.markdown-doc :is(th, td).nowrap) {
    white-space: nowrap;
  }

  :global(.markdown-doc .code-block-container) {
    margin: 0.8em 0;
    border-radius: 6px;
    border: 1px solid var(--border-color);
    background-color: var(--bg-secondary);
    overflow: hidden;
  }

  :global(.markdown-doc .code-block-header) {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 4px 10px;
    background-color: var(--bg-tertiary);
    border-bottom: 1px solid var(--border-color);
  }

  :global(.markdown-doc .code-lang-tag) {
    font-size: 10.88px;
    font-family: var(--font-mono);
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
  }

  :global(.markdown-doc .copy-code-btn, .markdown-doc .mermaid-toggle-btn, .markdown-doc .mermaid-expand-btn) {
    font-size: 10.88px;
    padding: 2px 6px;
    border-radius: 3px;
    background-color: var(--bg-secondary);
    border: 1px solid var(--border-color);
    color: var(--text-secondary);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  :global(.markdown-doc .copy-code-btn:hover, .markdown-doc .mermaid-toggle-btn:hover, .markdown-doc .mermaid-expand-btn:hover) {
    color: var(--text-primary);
  }

  :global(.markdown-doc .copy-code-btn.copied) {
    background-color: rgba(16, 185, 129, 0.15);
    color: var(--status-live);
    border-color: var(--status-live);
  }

  :global(.markdown-doc .code-block-actions) {
    display: flex;
    gap: 4px;
  }

  /* Mermaid sizes the SVG to the diagram; wide ones shrink to the column. */
  :global(.markdown-doc .mermaid-diagram) {
    padding: 8px 12px;
    overflow-x: auto;
    text-align: center;
    background-color: var(--bg-primary);
    cursor: zoom-in;
    /* Mermaid sizes labels without the text's break-word, which would
       otherwise wrap them narrower and clip them. */
    word-break: normal;
  }

  :global(.markdown-doc .mermaid-diagram svg) {
    max-width: 100%;
    height: auto;
  }

  :global(.markdown-doc pre) {
    margin: 0;
    padding: 8px 12px;
    overflow-x: auto;
    background-color: var(--bg-primary);
  }

  :global(.markdown-doc code.hljs) {
    font-family: var(--font-mono);
    font-size: 0.923em;
    line-height: 1.45;
  }
</style>
<script lang="ts">
  import Icon from '../../common/Icon.svelte';
  import type { ToolCall, SessionRef } from '../../../types';
  import { highlightCode } from '../../../highlight';
  import { reachesLineCount } from '../../../layout';
  import { api } from '../../../api';
  import SubagentLink from './SubagentLink.svelte';

  // An output this long always fills .output-content's max-height (350px at
  // 12px/1.4 lines), so its height is known without laying the text out. 160
  // columns is more than the 900px transcript fits in any monospace font.
  const FULL_BOX_LINES = 24;
  const MAX_COLUMNS = 160;

  interface Props {
    tool: ToolCall;
    sessionRef: SessionRef;
    defaultExpanded?: boolean;
  }

  let { tool, sessionRef, defaultExpanded = false }: Props = $props();

  let isFetchingBlob = $state(false);
  let fullOutput = $state<string | null>(null);
  let blobError = $state<string | null>(null);
  let copiedInput = $state(false);
  let copiedOutput = $state(false);

  let formattedInput = $derived.by(() => {
    if (!tool.input) return '';
    if (typeof tool.input === 'string') return tool.input;
    try {
      return JSON.stringify(tool.input, null, 2);
    } catch {
      return String(tool.input);
    }
  });

  let inputExpanded = $state(false);
  // Closed parameters need neither syntax highlighting nor a large hidden DOM.
  // Search expansion renders synchronously, before the jump's landing tick.
  let showInput = $derived(defaultExpanded || inputExpanded);
  let highlightedInput = $derived(showInput ? highlightCode(formattedInput, 'json') : '');

  let displayOutput = $derived(fullOutput !== null ? fullOutput : (tool.output || ''));
  let fillsOutputBox = $derived(reachesLineCount(displayOutput, FULL_BOX_LINES, MAX_COLUMNS));

  async function handleLoadFullOutput() {
    if (!tool.outputRef || isFetchingBlob) return;
    isFetchingBlob = true;
    blobError = null;
    try {
      const res = await api.getBlob(sessionRef, tool.outputRef);
      fullOutput = res.data;
    } catch (err: any) {
      blobError = err?.message || 'Failed to fetch complete tool output';
    } finally {
      isFetchingBlob = false;
    }
  }

  async function copyText(text: string, type: 'input' | 'output') {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      await navigator.clipboard.writeText(text);
      if (type === 'input') {
        copiedInput = true;
        setTimeout(() => (copiedInput = false), 1500);
      } else {
        copiedOutput = true;
        setTimeout(() => (copiedOutput = false), 1500);
      }
    }
  }
</script>

<div class="tool-card" class:status-error={tool.status === 'error'}>
  <div class="tool-header">
    <div class="tool-id-group">
      <Icon name="wrench" size={14} class="tool-icon" />
      <span class="tool-name">{tool.name}</span>
      <span class="tool-call-id">{tool.id}</span>
    </div>

    <div class="tool-status-badge status-{tool.status}">
      {#if tool.status === 'completed'}
        <Icon name="check" size={12} strokeWidth={2.5} /> completed
      {:else if tool.status === 'error'}
        <Icon name="x" size={12} strokeWidth={2.5} /> error
      {:else if tool.status === 'pending'}
        <Icon name="loader-circle" size={12} strokeWidth={2.5} class="spin" /> running
      {:else}
        <span class="status-indicator">?</span> {tool.status}
      {/if}
    </div>
  </div>

  {#if formattedInput}
    <details class="tool-section input-section" open={defaultExpanded} ontoggle={(event) => inputExpanded = event.currentTarget.open}>
      <summary class="section-summary">
        <span>Parameters</span>
        <button
          type="button"
          class="mini-copy-btn"
          onclick={(e) => {
            e.stopPropagation();
            copyText(formattedInput, 'input');
          }}
        >
          {copiedInput ? '✓ Copied' : 'Copy'}
        </button>
      </summary>
      <div class="code-wrapper">
        <pre><code class="hljs language-json">{@html highlightedInput}</code></pre>
      </div>
    </details>
  {/if}

  {#if displayOutput || tool.outputTruncated}
    <div class="tool-section output-section">
      <div class="output-header">
        <span class="output-label">Result:</span>
        {#if displayOutput}
          <button
            type="button"
            class="mini-copy-btn"
            onclick={() => copyText(displayOutput, 'output')}
          >
            {copiedOutput ? '✓ Copied' : 'Copy'}
          </button>
        {/if}
      </div>

      {#if displayOutput}
        <pre class="output-content" class:fills-box={fillsOutputBox}><code>{displayOutput}</code></pre>
      {/if}

      {#if tool.outputTruncated && fullOutput === null}
        <div class="truncated-banner">
          <span>Output truncated for display (exceeds 64 KiB)</span>
          <button
            type="button"
            class="load-blob-btn"
            disabled={isFetchingBlob}
            onclick={handleLoadFullOutput}
          >
            {isFetchingBlob ? 'Loading full output…' : 'Load full output'}
          </button>
        </div>
      {/if}

      {#if blobError}
        <div class="blob-error-msg"><Icon name="triangle-alert" size={12} /> {blobError}</div>
      {/if}
    </div>
  {/if}

  {#if tool.child}
    <div class="tool-child-section">
      <SubagentLink child={tool.child} />
    </div>
  {/if}
</div>

<style>
  /* One tinted surface, edged by its tone against the transcript; the
     parameters and result are lighter wells inside it, lined up with the
     header's text. */
  .tool-card {
    margin: 8px 0;
    border-radius: var(--radius-md);
    background-color: var(--bg-secondary);
    overflow: hidden;
  }

  .tool-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 7px 10px;
  }

  .tool-id-group {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  :global(.tool-icon) {
    color: var(--text-muted);
  }

  .tool-name {
    font-size: 12.8px;
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-primary);
  }

  .tool-call-id {
    font-size: 11.2px;
    font-family: var(--font-mono);
    color: var(--text-muted);
  }

  .tool-status-badge {
    font-size: 10.88px;
    font-weight: 600;
    padding: 1px 6px;
    border-radius: 4px;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .tool-status-badge.status-completed {
    background: rgba(16, 185, 129, 0.15);
    color: var(--status-live);
  }

  .tool-status-badge.status-error {
    background: color-mix(in srgb, var(--danger) 15%, transparent);
    color: var(--danger);
  }

  .tool-status-badge.status-pending {
    background: rgba(245, 158, 11, 0.15);
    color: #f59e0b;
  }

  .tool-section {
    border-top: 1px solid var(--border-color);
  }

  .section-summary {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 10px;
    font-size: 11.52px;
    color: var(--text-secondary);
    font-weight: 500;
    cursor: pointer;
    user-select: none;
  }

  .section-summary:hover {
    color: var(--text-primary);
  }

  .code-wrapper {
    margin: 0 10px 8px;
    padding: 8px;
    border-radius: var(--radius-sm);
    background: var(--bg-primary);
    overflow-x: auto;
  }

  .code-wrapper pre {
    margin: 0;
    font-size: 12px;
    font-family: var(--font-mono);
  }

  .output-section {
    padding: 6px 10px 10px;
  }

  .output-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 4px;
  }

  .output-label {
    font-size: 11.52px;
    font-weight: 600;
    color: var(--text-secondary);
  }

  .output-content {
    margin: 0;
    padding: 8px;
    border-radius: var(--radius-sm);
    background: var(--bg-primary);
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.4;
    white-space: pre-wrap;
    word-break: break-all;
    max-height: 350px;
    overflow-y: auto;
    /* Monospace output needs no kerning or ligatures; skipping them lets
       WebKit lay out large outputs about three times faster. */
    text-rendering: optimizeSpeed;
  }

  /* Off-screen outputs skip layout. The placeholder height is clamped by
     max-height, so it matches the real height and nothing shifts. */
  .output-content.fills-box {
    content-visibility: auto;
    contain-intrinsic-block-size: 350px;
  }

  .truncated-banner {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-top: 6px;
    padding: 4px 8px;
    background: rgba(245, 158, 11, 0.1);
    border: 1px dashed rgba(245, 158, 11, 0.4);
    border-radius: 4px;
    font-size: 11.52px;
    color: var(--text-secondary);
  }

  .load-blob-btn {
    padding: 2px 8px;
    font-size: 11.2px;
    font-weight: 500;
    border-radius: 4px;
    background: var(--accent-color);
    color: white;
    cursor: pointer;
  }

  .load-blob-btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .mini-copy-btn {
    padding: 1px 8px;
    font-size: 10.88px;
    font-weight: 500;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-muted);
    cursor: pointer;
    transition: background-color 0.15s ease, color 0.15s ease;
  }

  .mini-copy-btn:hover {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  .blob-error-msg {
    margin-top: 6px;
    font-size: 11.52px;
    color: var(--danger);
  }

  .tool-child-section {
    padding: 6px 10px;
    border-top: 1px solid var(--border-color);
  }
</style>

<script lang="ts">
  import Icon from '../../common/Icon.svelte';
  import type { ToolCall, SessionRef } from '../../../types';
  import { highlightCode } from '../../../highlight';
  import { readableToolOutput, toolOutputPreview, toolInputSummary, toolFileTarget, type ToolFileTarget } from '../../../toolPresentation';
  import { api } from '../../../api';
  import SubagentLink from './SubagentLink.svelte';

  interface Props {
    tool: ToolCall;
    sessionRef: SessionRef;
    defaultExpanded?: boolean;
    /** Opens a file tool's file; without it the path stays text. */
    onOpenFile?: (target: ToolFileTarget) => void;
  }

  let { tool, sessionRef, defaultExpanded = false, onOpenFile }: Props = $props();

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
  let readableOutput = $derived(readableToolOutput(displayOutput));
  let preview = $derived(toolOutputPreview(readableOutput));
  let outputExpanded = $state(false);
  let showFullOutput = $derived(defaultExpanded || outputExpanded);
  let summary = $derived(toolInputSummary(tool.input));
  let fileTarget = $derived(onOpenFile ? toolFileTarget(tool.name, tool.input) : null);

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
      <span class="tool-name" title={tool.id ? `${tool.name}\n${tool.id}` : tool.name}>{tool.name}</span>
      {#if fileTarget}
        <button type="button" class="tool-summary tool-file-link" title={`Open ${fileTarget.path}`}
          onclick={() => { if (fileTarget) onOpenFile?.(fileTarget); }}>{fileTarget.path}</button>
      {:else if summary}<span class="tool-summary" title={summary}>{summary}</span>{/if}
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
        <span class="disclosure-label"><Icon name={showInput ? 'chevron-down' : 'chevron-right'} size={14} /> Parameters</span>
        <button
          type="button"
          class="mini-copy-btn"
          onclick={(e) => {
            e.preventDefault();
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
        <span class="output-label">Output</span>
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
        <pre class="output-content"><code>{showFullOutput ? readableOutput : preview.text}</code></pre>
        {#if preview.shortened && !defaultExpanded}
          <button type="button" class="expand-output-btn" aria-expanded={showFullOutput} onclick={() => outputExpanded = !outputExpanded}>
            <Icon name={showFullOutput ? 'chevron-down' : 'chevron-right'} size={14} />
            {showFullOutput ? 'Show less' : 'Show more output'}
          </button>
        {/if}
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
    margin: 10px 0;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    background-color: var(--bg-secondary);
    overflow: hidden;
  }

  .tool-header {
    /* ACP may put the entire command in tool.name. Reserve the badge's
       column before sizing either the name or the optional summary. */
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    padding: 10px 12px;
    gap: 12px;
  }

  .tool-id-group {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    overflow: hidden;
  }

  :global(.tool-icon) {
    color: var(--text-muted);
    flex-shrink: 0;
  }

  .tool-name {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-code));
    font-weight: 600;
    font-family: var(--font-mono);
    color: var(--text-primary);
  }

  .tool-summary {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-code));
    font-family: var(--font-mono);
    color: var(--text-secondary);
  }

  /* A button keeps the summary's look; only hover and focus show the link. */
  .tool-file-link {
    padding: 0;
    border: none;
    background: transparent;
    font-weight: inherit;
    line-height: inherit;
    text-align: left;
    cursor: pointer;
  }

  .tool-file-link:hover {
    color: var(--accent-color);
    text-decoration: underline;
  }

  .tool-name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tool-status-badge {
    white-space: nowrap;
  }

  .disclosure-label, .expand-output-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .expand-output-btn {
    margin-top: 8px;
    padding: 4px 0;
    background: transparent;
    color: var(--accent-color);
    cursor: pointer;
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-small));
  }

  button:focus-visible, summary:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .section-summary::-webkit-details-marker {
    display: none;
  }

  .tool-status-badge {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
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
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
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
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-code));
    font-family: var(--font-mono);
    line-height: 1.6;
  }

  /* The browser gives <code> its own generic monospace; take the pre's font. */
  pre code {
    font: inherit;
  }

  .output-section {
    padding: 8px 12px 12px;
  }

  .output-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 4px;
  }

  .output-label {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    font-weight: 600;
    color: var(--text-secondary);
  }

  .output-content {
    margin: 0;
    padding: 8px;
    border-radius: var(--radius-sm);
    background: var(--bg-primary);
    font-family: var(--font-mono);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-code));
    line-height: 1.6;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    word-break: normal;
    /* Monospace output needs no kerning or ligatures; skipping them lets
       WebKit lay out large outputs about three times faster. */
    text-rendering: optimizeSpeed;
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
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    color: var(--text-secondary);
  }

  .load-blob-btn {
    padding: 2px 8px;
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
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
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
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
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    color: var(--danger);
  }

  .tool-child-section {
    padding: 6px 10px;
    border-top: 1px solid var(--border-color);
  }
</style>

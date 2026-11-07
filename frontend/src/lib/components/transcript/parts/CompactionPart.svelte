<script lang="ts">
  import Icon from '../../common/Icon.svelte';
  interface Props {
    text?: string;
  }

  let { text = '' }: Props = $props();

  let isExpanded = $state(false);
</script>

<div class="compaction-divider">
  <div class="divider-line"></div>
  <button
    type="button"
    class="compaction-pill"
    title={text ? 'Click to toggle compaction summary' : 'Context was compacted here'}
    onclick={() => {
      if (text) isExpanded = !isExpanded;
    }}
  >
    <Icon name="scissors" size={12} class="pill-icon" />
    <span class="pill-text">Context Compacted</span>
    {#if text}
      <span class="pill-chevron">{isExpanded ? '▲' : '▼'}</span>
    {/if}
  </button>
  <div class="divider-line"></div>
</div>

{#if text && isExpanded}
  <div class="compaction-details">
    <div class="summary-header">Summary:</div>
    <div class="summary-text">{text}</div>
  </div>
{/if}

<style>
  .compaction-divider {
    display: flex;
    align-items: center;
    gap: 12px;
    margin: 16px 0;
    user-select: none;
  }

  .divider-line {
    flex: 1;
    height: 1px;
    background-color: var(--border-color);
  }

  .compaction-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    border-radius: 12px;
    background-color: var(--bg-tertiary);
    border: 1px solid var(--border-color);
    color: var(--text-muted);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .compaction-pill:hover {
    color: var(--text-primary);
    border-color: var(--text-secondary);
  }

  :global(.pill-icon) {
    color: var(--text-muted);
  }

  .pill-chevron {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
    opacity: 0.7;
  }

  .compaction-details {
    margin: -8px 0 16px 0;
    padding: 10px 14px;
    border-radius: 6px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-small));
    color: var(--text-secondary);
    line-height: 1.45;
  }

  .summary-header {
    font-weight: 600;
    text-transform: uppercase;
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-tag));
    color: var(--text-muted);
    margin-bottom: 4px;
  }

  .summary-text {
    white-space: pre-wrap;
  }
</style>

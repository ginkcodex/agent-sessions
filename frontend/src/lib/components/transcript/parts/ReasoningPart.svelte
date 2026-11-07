<script lang="ts">
  import Icon from '../../common/Icon.svelte';
  import { renderMarkdown } from '../../../markdown';
  import { handleMessageClick } from '../../../messageFileClick';
  import { renderMermaid } from '../../../mermaid';
  import { theme } from '../../../stores/theme.svelte';

  interface Props {
    text: string;
    onFileLink?: (value: string) => void;
    defaultExpanded?: boolean;
  }

  let { text, defaultExpanded = false, onFileLink }: Props = $props();

  let expanded = $state(false);
  let renderedHtml = $derived(defaultExpanded || expanded ? renderMarkdown(text, 'transcript') : '');
  let container = $state<HTMLDivElement>();

  $effect(() => {
    void renderedHtml;
    renderMermaid(container, theme.resolved);
  });
</script>

<details class="reasoning-block" open={defaultExpanded} ontoggle={(event) => expanded = event.currentTarget.open}>
  <summary class="reasoning-summary">
    <Icon name="brain" size={14} class="reasoning-icon" />
    <span class="reasoning-title">Thinking process…</span>
  </summary>
  <div class="reasoning-body">
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <!-- svelte-ignore a11y_no_static_element_interactions -->
    <div class="markdown-content text-part-container" bind:this={container} onclick={(event) => handleMessageClick(event, onFileLink)}>
      {@html renderedHtml}
    </div>
  </div>
</details>

<style>
  .reasoning-block {
    margin: 8px 0;
    border-radius: 6px;
    background-color: var(--bg-secondary);
    border: 1px solid var(--border-color);
    overflow: hidden;
  }

  .reasoning-summary {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 10px;
    cursor: pointer;
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-small));
    font-weight: 500;
    color: var(--text-secondary);
    user-select: none;
    background-color: var(--bg-secondary);
  }

  .reasoning-summary:hover {
    color: var(--text-primary);
  }

  :global(.reasoning-icon) {
    color: var(--text-muted);
  }

  .reasoning-body {
    padding: 10px 14px;
    border-top: 1px solid var(--border-color);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-code));
    color: var(--text-secondary);
    line-height: 1.5;
    background-color: var(--bg-primary);
  }
</style>

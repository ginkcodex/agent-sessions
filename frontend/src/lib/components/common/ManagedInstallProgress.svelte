<script lang="ts">
  // An app-managed install in progress, with its Cancel button. Code checks
  // and the chat setup show the same card.
  import type { Snippet } from 'svelte';

  interface Props {
    /** What is downloading, and where. */
    text: string;
    cancelDisabled?: boolean;
    onCancel: () => void;
    /** More lines under the status. */
    children?: Snippet;
  }

  let { text, cancelDisabled = false, onCancel, children }: Props = $props();
</script>

<div class="card" role="group" aria-label="Installation progress">
  <p role="status">{text}</p>
  {@render children?.()}
  <button type="button" disabled={cancelDisabled} onclick={onCancel}>Cancel download</button>
</div>

<style>
  .card { border: 1px solid var(--border-color); border-radius: 6px; padding: 6px 8px; margin: 6px 0; background: var(--bg-secondary); font-size: 12px; line-height: 1.45; }
  p { margin: 4px 0; }
  button { padding: 3px 8px; border: 1px solid var(--border-color); border-radius: 4px; cursor: pointer; font-size: 12px; }
  button:disabled { opacity: 0.5; cursor: default; }
</style>

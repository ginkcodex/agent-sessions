<script lang="ts">
  import { gitHistory } from '../../../stores/gitHistory.svelte';
  import type { GitHistoryChange } from '../../../gitHistory';
  import HistoryChangedFiles from './HistoryChangedFiles.svelte';
  let { onOpen, selectedPath = null }: {
    onOpen: (change: GitHistoryChange, keep: boolean) => void;
    selectedPath?: string | null;
  } = $props();
</script>

<section class="comparison" aria-label="Working tree comparison">
  <header>
    <h3>{gitHistory.comparisonLabel}</h3>
    <button type="button" disabled={gitHistory.comparisonLoading || !!gitHistory.blockedReason} onclick={() => void gitHistory.refreshComparison()}>Refresh comparison</button>
    <button type="button" onclick={() => gitHistory.closeComparison()}>Close comparison</button>
  </header>
  <p class="notice">Saved files on disk, including staged and unstaged changes. Unsaved editor buffers are not compared. Refresh to read newer disk changes.</p>
  {#if gitHistory.comparisonLoading}
    <p class="notice" role="status">Comparing working tree…</p>
  {:else if gitHistory.comparisonError}
    <p class="notice" role="alert">{gitHistory.comparisonError}</p>
  {:else if gitHistory.comparison}
    <HistoryChangedFiles changes={gitHistory.comparison.changes} truncated={gitHistory.comparison.truncated} {selectedPath} {onOpen} />
  {/if}
</section>

<style>
  .comparison { display: flex; flex-direction: column; height: 100%; min-height: 0; font-size: .8rem; }
  header { display: flex; flex-wrap: wrap; align-items: center; gap: 6px; padding: 10px; }
  h3 { flex: 1 1 100%; margin: 0; font-size: .85rem; overflow-wrap: anywhere; }
  button { font: inherit; border: 1px solid var(--border-color); border-radius: 4px; background: var(--bg-secondary); color: var(--text-primary); padding: 4px 6px; cursor: pointer; }
  button:disabled { opacity: .5; cursor: default; }
  .notice { color: var(--text-secondary); margin: 0; padding: 8px 10px; line-height: 1.4; }
</style>

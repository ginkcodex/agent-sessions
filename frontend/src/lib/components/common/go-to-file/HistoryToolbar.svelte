<script lang="ts">
  import { gitHistory, HISTORY_QUERY_DELAY_MS } from '../../../stores/gitHistory.svelte';
  import HistoryBranches from './HistoryBranches.svelte';
  import Dropdown from '../Dropdown.svelte';
  import { repositoryOptions } from '../../../gitHistory';
  /** True while a typed query waits for its debounced read. */
  let { pending = $bindable(false) }: { pending?: boolean } = $props();
  let timer: ReturnType<typeof setTimeout> | undefined;
  function cancel() { clearTimeout(timer); timer = undefined; pending = false; }
  $effect(() => { void gitHistory.contextKey; return cancel; });
  function search(value: string) {
    cancel();
    // Invalidate in-flight pages immediately; only the replacement read is delayed.
    void gitHistory.setQuery(value, { defer: true });
    pending = true;
    timer = setTimeout(() => { pending = false; void gitHistory.ensureLoaded(); }, HISTORY_QUERY_DELAY_MS);
  }
</script>

<div class="toolbar">
  <div class="repository"><span aria-hidden="true">Repository</span>
    <Dropdown ariaLabel="History repository" options={repositoryOptions(gitHistory.repositories)} value={gitHistory.selectedRepository ?? ''}
      placeholder="Choose a repository…" disabled={!!gitHistory.blockedReason || gitHistory.repositoriesLoading || !gitHistory.repositories.length}
      onChange={(repo) => { cancel(); void gitHistory.selectRepository(repo); }} />
  </div>
  {#if gitHistory.repository?.scope}<span class="scope" title={gitHistory.repository.scope}>Scoped history: {gitHistory.repository.scope}</span>{/if}
  <div class="narrow-branches"><HistoryBranches compact /></div>
  <label class="search text-field"><span class="sr-only">Search commit message, author or OID</span><input type="search" aria-label="Search commit message, author or OID" placeholder="Message, author or hash…" autocomplete="off" autocorrect="off" autocapitalize="off" spellcheck="false" value={gitHistory.query} disabled={!!gitHistory.blockedReason || gitHistory.selectedRepository === null} oninput={(e) => search(e.currentTarget.value)} /></label>
  <button type="button" aria-label="Refresh Git history" disabled={!!gitHistory.blockedReason || gitHistory.loading || pending} onclick={() => { cancel(); void gitHistory.refresh(); }}>Refresh</button>
</div>

<style>
  /* The repository and branch dropdowns, the search field and Refresh share one height. */
  .toolbar { --control-height: 26px; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; width: 100%; min-width: 0; font-size: .8rem; line-height: 1.4; }
  label, .repository { display: flex; align-items: center; gap: 6px; min-width: 0; }
  .repository { gap: 8px; max-width: 320px; color: var(--text-secondary); }
  button { color: var(--text-primary); background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 4px; height: var(--control-height); padding: 0 8px; font: inherit; min-width: 0; }
  button { cursor: pointer; } :disabled { opacity: .6; cursor: default; }
  .search { flex: 1; min-width: 160px; height: var(--control-height); padding-block: 0; } .scope { color: var(--text-secondary); overflow-wrap: anywhere; }
  .narrow-branches { display: none; } .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
  @container (max-width: 850px) { .narrow-branches { display: block; } }
</style>

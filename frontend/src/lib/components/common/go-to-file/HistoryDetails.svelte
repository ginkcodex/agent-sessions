<script lang="ts">
  import { gitHistory } from '../../../stores/gitHistory.svelte';
  import type { GitHistoryChange, GitHistoryPerson } from '../../../gitHistory';
  import { formatAbsoluteTime } from '../../../date';
  import { copyToClipboard } from '../../../portable';
  import { toast } from '../../../stores/toast.svelte';
  import HistoryChangedFiles from './HistoryChangedFiles.svelte';

  let { onOpen, selectedPath = null }: {
    onOpen: (change: GitHistoryChange, keep: boolean) => void;
    selectedPath?: string | null;
  } = $props();
  function person(value: GitHistoryPerson) {
    const date = new Date(value.time * 1000);
    const time = Number.isNaN(date.getTime()) ? 'Unknown date' : formatAbsoluteTime(date.toISOString());
    return `${value.name} <${value.email}> · ${time}`;
  }
  async function copyHash(oid: string) {
    if (await copyToClipboard(oid)) toast.show({ title: 'Commit hash copied', body: oid });
    else toast.show({ title: 'Could not copy', body: 'Could not write to the clipboard.', tone: 'error' });
  }
</script>

{#if gitHistory.detailLoading}
  <p class="state" role="status">Loading commit details…</p>
{:else if gitHistory.detailError}
  <div class="state" role="alert">{gitHistory.detailError}
    <button type="button" onclick={() => { if (gitHistory.selectedCommit) void gitHistory.selectCommit(gitHistory.selectedCommit); }}>Retry details</button>
  </div>
{:else if gitHistory.detail}
  {@const detail = gitHistory.detail}
  <div class="details">
    <HistoryChangedFiles changes={detail.changes} truncated={detail.truncated} {selectedPath} {onOpen} />
    <section class="metadata" aria-label="Commit details">
      <h3>Commit details</h3>
      <dl>
        <dt>Commit</dt><dd class="oid">{detail.oid} <button type="button" class="copy" onclick={() => copyHash(detail.oid)}>Copy hash</button></dd>
        <dt>Author</dt><dd>{person(detail.author)}</dd>
        <dt>Committer</dt><dd>{person(detail.committer)}</dd>
        <dt>Parents</dt><dd class="oid">{detail.parents.length ? detail.parents.join('\n') : 'None (root commit)'}</dd>
        <dt>Comparison</dt><dd>{detail.base ? detail.parents.length > 1 ? 'First parent (merge; other parents are not compared)' : 'First parent' : 'Empty tree (root commit)'}{#if detail.base}<span class="oid">{detail.base}</span>{/if}</dd>
      </dl>
      <pre class="message">{detail.message}</pre>
    </section>
  </div>
{:else}
  <p class="state">Select a commit to inspect its changed files.</p>
{/if}

<style>
  button { font: inherit; }
  .details { font-size: .8rem; line-height: 1.4; display: flex; flex-direction: column; height: 100%; min-height: 0; }
  h3 { margin: 0; padding: 12px; font-size: .85rem; }
  .copy { margin-left: 6px; font: inherit; font-size: .75rem; cursor: pointer; background: transparent; color: var(--accent-color); border: 1px solid var(--border-color); border-radius: 4px; padding: 1px 6px; }
  .metadata { flex: 1 1 50%; min-height: 100px; overflow: auto; border-top: 1px solid var(--border-color); }
  dl { margin: 0 10px; font-size: .75rem; }
  dt { color: var(--text-secondary); margin-top: 6px; }
  dd { margin: 2px 0; overflow-wrap: anywhere; }
  .oid { font-family: monospace; white-space: pre-wrap; display: block; }
  .message { white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; font-size: .8rem; margin: 12px 10px; }
  .state { padding: 8px 10px; color: var(--text-secondary); font-size: .8rem; line-height: 1.4; }
  .state button { display: block; margin-top: 8px; }
</style>

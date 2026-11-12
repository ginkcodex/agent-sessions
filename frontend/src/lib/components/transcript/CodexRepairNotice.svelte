<script lang="ts">
  import type { SessionMeta } from '../../types';
  import { needsCodexRepair } from '../../codexRepair';
  import { CodexRepair } from '../../stores/codexRepair.svelte';
  import { link } from '../../stores/link.svelte';
  import { connectionStore } from '../../stores/connection.svelte';

  let { meta, disabledReason = '' }: { meta: SessionMeta; disabledReason?: string } = $props();
  const repair = new CodexRepair();
  let supported = $derived(connectionStore.capabilities.repairCodexSession === true);
  $effect(() => { void link.dataHost; void link.phase; repair.setSession(meta); });
</script>

{#if needsCodexRepair(meta)}
  <aside class="repair-notice" aria-label="Imported session repair" data-codex-repair>
    <div class="repair-copy">
      <strong>{repair.result ? 'Repair complete' : 'This imported session needs a repair'}</strong>
      <p>{repair.result ? 'Your conversation is unchanged. You can resume this session now; a backup was saved.' : 'An older import used a filename Codex cannot reopen. Repair keeps your conversation and creates a backup.'}</p>
      {#if repair.error}<p class="error" role="alert">{repair.error}</p>
      {:else if !supported}<p class="hint">Update the app or connected host to enable in-app repair.</p>
      {:else if disabledReason}<p class="hint">{disabledReason}</p>{/if}
    </div>
    <button type="button" disabled={repair.busy || !!repair.result || !supported || !!disabledReason || link.stale || link.locked}
      onclick={() => void repair.run()}>{repair.busy ? 'Repairing…' : repair.result ? 'Repaired' : 'Repair session'}</button>
  </aside>
{/if}

<style>
  .repair-notice { display: flex; align-items: center; gap: 16px; padding: 12px 14px; border: 1px solid var(--border-color); border-radius: var(--radius-md); background: var(--bg-primary); }
  .repair-copy { flex: 1; min-width: 0; }
  strong { color: var(--text-primary); font-size: 13px; font-weight: 600; }
  p { margin: 4px 0 0; color: var(--text-secondary); font-size: 12px; line-height: 1.5; overflow-wrap: anywhere; }
  .error { color: var(--danger); }
  button { flex-shrink: 0; padding: 7px 12px; border-radius: var(--radius-md); background: var(--accent-color); color: white; font-size: 12px; font-weight: 600; cursor: pointer; }
  button:disabled { opacity: .5; cursor: default; }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 3px; }
  @container (max-width: 440px) { .repair-notice { flex-direction: column; align-items: flex-start; gap: 10px; } }
</style>

<script lang="ts">
  import { agentSettings } from '../../stores/agentSettings.svelte';
  import { link } from '../../stores/link.svelte';
  import { hostLabel } from '../../hosts';

  $effect(() => {
    void link.dataHost;
    void link.phase;
    if (!link.stale && !link.locked) void agentSettings.refresh();
  });
</script>

<section class="agents-section">
  <h3>Coding agents</h3>
  <p class="hint">Choose the agents to discover and offer for new sessions on {link.dataHost ? hostLabel(link.dataHost) : 'this computer'}. At least one agent must stay enabled. Session files are never deleted.</p>
  {#if agentSettings.loading}
    <p class="hint" role="status">Loading coding agents…</p>
  {:else if agentSettings.settings}
    <fieldset disabled={agentSettings.saving || link.stale || link.locked}>
      <legend>Enabled coding agents</legend>
      {#each agentSettings.settings.supported as agent (agent.id)}
        <label>
          <input type="checkbox" checked={agentSettings.selected.includes(agent.id)}
            disabled={agentSettings.selected.length === 1 && agentSettings.selected.includes(agent.id)}
            onchange={event => agentSettings.toggle(agent.id, event.currentTarget.checked)} />
          <span>{agent.name}</span>
        </label>
      {/each}
    </fieldset>
    <p class="hint">Changes apply after {link.dataHost ? 'reconnecting to this host' : 'restarting the app'}. Other hosts keep their own selection.</p>
    {#if agentSettings.settings.restartRequired}
      <p class="notice" role="status">Saved. {link.dataHost ? 'Reconnect to this host' : 'Restart the app'} to apply the enabled agents. Until then, the current agents remain active.</p>
    {/if}
    <div class="actions"><button type="button" class="primary" disabled={!agentSettings.dirty || !agentSettings.selected.length || agentSettings.saving || link.stale || link.locked} onclick={() => void agentSettings.save()}>{agentSettings.saving ? 'Saving…' : 'Save'}</button></div>
  {/if}
  {#if agentSettings.error}
    <p class="error" role="alert">{agentSettings.error}</p>
  {/if}
</section>

<style>
  h3 { margin: 0 0 10px; font-size: 14px; }
  .hint { color: var(--text-secondary); font-size: 12px; line-height: 1.5; }
  fieldset { border: 0; padding: 0; margin: 18px 0; }
  legend { font-size: 12px; font-weight: 600; margin-bottom: 10px; }
  label { display: flex; align-items: center; gap: 8px; margin: 12px 0; font-size: 13px; }
  input { accent-color: var(--accent-color); }
  .actions { display: flex; justify-content: flex-end; margin-top: 16px; }
  button { padding: 7px 14px; border: 1px solid var(--accent-color); border-radius: 4px; cursor: pointer; }
  button.primary { background: var(--accent-color); color: white; }
  button.primary:hover:not(:disabled) { background: var(--accent-hover); border-color: var(--accent-hover); }
  button:disabled { opacity: .5; cursor: default; }
  .error { color: var(--error-color, #cf222e); font-size: 12px; }
  .notice { padding: 10px; background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 4px; font-size: 12px; line-height: 1.5; }
</style>

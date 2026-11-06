<script lang="ts">
  import { agentSettings } from '../../stores/agentSettings.svelte';
  import { link } from '../../stores/link.svelte';
  import { hostLabel } from '../../hosts';
  import { api } from '../../api';
  import { epoch } from '../../link';
  import { filePrompt } from '../../stores/filePrompt.svelte';
  import { fileActions } from '../../stores/fileActions.svelte';

  /** How long the restart prompt stands in for the close confirm. */
  const RESTART_CONFIRMED_MS = 15_000;

  let applying = $state(false);
  let applyError = $state('');

  async function applyChanges() {
    if (applying || agentSettings.dirty || link.stale || link.locked) return;
    const owner = epoch();
    const host = link.dataHost;
    applying = true;
    applyError = '';
    try {
      const choice = await filePrompt.ask(host ? 'Reconnect to apply coding agents?' : 'Restart to apply coding agents?',
        host ? 'Reconnecting ends the current host connection. Running remote terminal processes may end.' : 'Restarting closes this window and ends its running terminal processes, including coding agents. Unsaved files will be checked before closing.',
        [{ value: 'apply', label: host ? 'Reconnect' : 'Restart', danger: true }]);
      if (choice !== 'apply' || owner !== epoch() || link.stale || link.locked) return;
      if (host) {
        await fileActions.changeHost(async () => {
          await api.disconnect();
          await api.connect(host);
        });
      } else {
        if (!api.restartApp) throw new Error('Please restart the app manually to apply coding agents.');
        // The prompt above already warned that the terminals end.
        fileActions.closeConfirmedUntil = Date.now() + RESTART_CONFIRMED_MS;
        try { await api.restartApp(); }
        catch (err) { fileActions.closeConfirmedUntil = 0; throw err; }
      }
    } catch (err) { applyError = err instanceof Error ? err.message : String(err); }
    finally { applying = false; }
  }

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
    <div class="actions">
      {#if agentSettings.settings.restartRequired}
        <button type="button" class="primary" disabled={applying || agentSettings.dirty || agentSettings.saving || link.stale || link.locked} onclick={() => void applyChanges()}>{applying ? 'Applying…' : link.dataHost ? 'Reconnect' : 'Restart'}</button>
      {/if}
      <button type="button" class="primary" disabled={!agentSettings.dirty || !agentSettings.selected.length || agentSettings.saving || applying || link.stale || link.locked} onclick={() => void agentSettings.save()}>{agentSettings.saving ? 'Saving…' : 'Save'}</button>
    </div>
    {#if applyError}<p class="error" role="alert">{applyError}</p>{/if}
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
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }
  button { padding: 5px 12px; border-radius: var(--radius-sm); background: var(--btn-bg); color: var(--text-primary); font-weight: 500; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out); }
  button:hover:not(:disabled) { background: var(--btn-bg-hover); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button.primary { background: var(--accent-color); color: white; }
  button.primary:hover:not(:disabled) { background: var(--accent-hover); }
  button:disabled { opacity: .5; cursor: default; }
  .error { color: var(--error-color, #cf222e); font-size: 12px; }
  .notice { padding: 10px 12px; background: var(--bg-secondary); border-radius: var(--radius-md); font-size: 12px; line-height: 1.5; }
</style>

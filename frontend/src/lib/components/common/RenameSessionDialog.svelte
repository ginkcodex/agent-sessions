<script lang="ts">
  import { tick } from 'svelte';
  import { modalFocus } from '../../modalFocus';
  import { link } from '../../stores/link.svelte';
  import { sessionRename } from '../../stores/sessionRename.svelte';
  import DialogCloseButton from './DialogCloseButton.svelte';

  let input: HTMLInputElement | undefined = $state();
  $effect(() => {
    if (!sessionRename.target) return;
    void tick().then(() => { input?.focus(); input?.select(); });
  });
  $effect(() => {
    void link.dataHost; void link.phase;
    sessionRename.reconcileHost();
  });
</script>

{#if sessionRename.target}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="rename-backdrop" role="presentation" onkeydown={event => {
    event.stopPropagation();
    if (event.key === 'Escape') { event.preventDefault(); if (!sessionRename.busy) sessionRename.dismiss(); }
  }}>
    <div class="rename-dialog" role="dialog" aria-modal="true" aria-labelledby="rename-session-title" use:modalFocus>
      <header><h2 id="rename-session-title">Rename session</h2><DialogCloseButton label="Close rename dialog" onclick={() => { if (!sessionRename.busy) sessionRename.dismiss(); }} /></header>
      <p>The name is saved in {sessionRename.target.ref.agent}, not just in Agent Sessions.</p>
      <form onsubmit={event => { event.preventDefault(); void sessionRename.save(); }}>
        <label for="session-name">Session name</label>
        <input id="session-name" bind:this={input} bind:value={sessionRename.title} placeholder={sessionRename.target.title} autocomplete="off" disabled={sessionRename.busy} />
        {#if sessionRename.error}<p class="error" role="alert">{sessionRename.error}</p>{/if}
        <footer><button type="button" disabled={sessionRename.busy} onclick={() => sessionRename.dismiss()}>Cancel</button><button type="submit" class="primary" disabled={sessionRename.busy || !sessionRename.valid}>{sessionRename.busy ? 'Renaming…' : 'Rename'}</button></footer>
      </form>
    </div>
  </div>
{/if}

<style>
  .rename-backdrop { position: fixed; inset: 0; z-index: 1100; display: grid; place-items: center; padding: 20px; background: #0008; }
  .rename-dialog { width: min(480px, 100%); padding: 20px; border: 1px solid var(--border-color); border-radius: 8px; background: var(--bg-primary); color: var(--text-primary); box-shadow: 0 16px 36px #0005; }
  header, footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  h2 { margin: 0; font-size: 18px; }
  p, label { font-size: 13px; }
  p { color: var(--text-secondary); }
  label { display: block; margin-bottom: 6px; }
  input { box-sizing: border-box; width: 100%; padding: 8px; border: 1px solid var(--border-color); border-radius: 4px; background: var(--bg-secondary); color: var(--text-primary); }
  footer { justify-content: flex-end; margin-top: 20px; }
  button { padding: 7px 12px; border: 1px solid var(--border-color); border-radius: 4px; background: var(--bg-secondary); color: var(--text-primary); cursor: pointer; }
  button:hover:not(:disabled) { background: var(--hover-bg); }
  button:disabled { opacity: .5; cursor: default; }
  .primary { background: var(--accent-color); border-color: var(--accent-color); color: white; }
  .primary:hover:not(:disabled) { background: var(--accent-hover); }
  .error { color: var(--error-color, #cf222e); }
</style>

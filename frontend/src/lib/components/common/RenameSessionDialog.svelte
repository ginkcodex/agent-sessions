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
        <input id="session-name" class="text-field" bind:this={input} bind:value={sessionRename.title} placeholder={sessionRename.target.title} autocomplete="off" disabled={sessionRename.busy} />
        {#if sessionRename.error}<p class="error" role="alert">{sessionRename.error}</p>{/if}
        <footer><button type="button" disabled={sessionRename.busy} onclick={() => sessionRename.dismiss()}>Cancel</button><button type="submit" class="primary" disabled={sessionRename.busy || !sessionRename.valid}>{sessionRename.busy ? 'Renaming…' : 'Rename'}</button></footer>
      </form>
    </div>
  </div>
{/if}

<style>
  .rename-backdrop { position: fixed; inset: 0; z-index: 1100; display: grid; place-items: center; padding: 20px; background: var(--scrim); animation: -global-scrim-in var(--dur-med) var(--ease-out); }
  .rename-dialog { width: min(460px, 100%); padding: 20px 22px 18px; border: 1px solid var(--dialog-border); border-radius: var(--radius-dialog); background: var(--bg-primary); color: var(--text-primary); box-shadow: var(--shadow-dialog); animation: -global-dialog-in var(--dur-med) var(--ease-out); }
  header, footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  header { margin: -4px -8px 0 0; }
  h2 { margin: 0; font-size: 16px; font-weight: 600; }
  p { margin: 6px 0 18px; font-size: 13px; line-height: 1.5; color: var(--text-secondary); }
  label { display: block; margin-bottom: 6px; font-size: 12px; font-weight: 600; color: var(--text-secondary); }
  input { box-sizing: border-box; width: 100%; padding: 8px 10px; font-size: 13px; }
  footer { justify-content: flex-end; margin-top: 22px; }
  button { height: 32px; padding: 0 14px; border-radius: var(--radius-md); background: var(--btn-bg); color: var(--text-primary); font-size: 13px; font-weight: 500; transition: background-color var(--dur-fast) var(--ease-out); }
  button:hover:not(:disabled) { background: var(--btn-bg-hover); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button:disabled { opacity: .5; cursor: default; }
  .primary { background: var(--accent-color); color: white; }
  .primary:hover:not(:disabled) { background: var(--accent-hover); }
  .error { margin: 8px 0 0; color: var(--danger); }
</style>

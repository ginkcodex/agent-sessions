<script lang="ts">
  import { tick } from 'svelte';
  import { files } from '../../../stores/files.svelte';
  import { link } from '../../../stores/link.svelte';
  import { fileActions } from '../../../stores/fileActions.svelte';
  // One prompt for New Folder and New File; only the wording and the action differ.
  let { path, kind = 'folder', onClose }: { path: string; kind?: 'folder' | 'file'; onClose: () => void } = $props();
  const noun = $derived(kind === 'folder' ? 'Folder' : 'File');
  const dir = files.dir;
  const host = link.dataHost;
  const context = files.contextVersion;
  let name = $state('');
  let error = $state('');
  let busy = $state(false);
  let panel: HTMLDivElement | undefined = $state();
  let input: HTMLInputElement | undefined = $state();
  const invalid = $derived(!name || name === '.' || name === '..' || /[\/\\\0]/.test(name));
  $effect(() => {
    const previous = document.activeElement;
    void tick().then(() => { input?.focus(); input?.select(); });
    const contain = (e: FocusEvent) => {
      if (panel && e.target instanceof Node && !panel.contains(e.target)) input?.focus();
    };
    document.addEventListener('focusin', contain);
    return () => {
      document.removeEventListener('focusin', contain);
      if (previous instanceof HTMLElement) previous.focus();
    };
  });
  async function submit() {
    if (busy || invalid) return;
    if (dir !== files.dir || host !== link.dataHost || context !== files.contextVersion) {
      error = 'The project or connection changed. Cancel and try again.';
      return;
    }
    busy = true;
    error = '';
    try {
      if (kind === 'folder') await fileActions.createDirectory(path, name);
      else await fileActions.createFile(path, name);
      onClose();
    }
    catch (err) { error = err instanceof Error ? err.message : String(err); }
    finally { busy = false; }
  }
  function key(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'Escape') { e.preventDefault(); if (!busy) onClose(); }
    if ((e.metaKey || e.ctrlKey) && ['s', 'w', 'f'].includes(e.key.toLowerCase())) e.preventDefault();
    if (e.key === 'Tab') {
      const controls = [...(panel?.querySelectorAll<HTMLElement>('input:not(:disabled), button:not(:disabled)') ?? [])];
      const index = controls.indexOf(document.activeElement as HTMLElement);
      e.preventDefault();
      controls[(index + (e.shiftKey ? -1 : 1) + controls.length) % controls.length]?.focus();
    }
  }
</script>

<div class="folder-backdrop" role="presentation">
  <div class="folder-prompt" role="dialog" tabindex="-1" aria-modal="true" aria-labelledby="folder-title" bind:this={panel} onkeydown={key}>
    <form onsubmit={(e) => { e.preventDefault(); void submit(); }}>
      <h2 id="folder-title">New {noun}</h2>
      <p>Create a {kind} in {path || dir}</p>
      <label for="folder-name">{noun} name</label>
      <input class="text-field" id="folder-name" bind:this={input} bind:value={name} readonly={busy} autocomplete="off" spellcheck="false" aria-invalid={invalid} aria-describedby={error ? 'folder-error' : undefined} />
      {#if error}<p id="folder-error" class="error" role="alert">{error}</p>{/if}
      <div class="actions">
        <button type="button" disabled={busy} onclick={onClose}>Cancel</button>
        <button type="submit" disabled={busy || invalid}>{busy ? 'Creating…' : `Create ${noun}`}</button>
      </div>
    </form>
  </div>
</div>

<style>
  .folder-backdrop { position: fixed; inset: 0; z-index: 1200; display: grid; place-items: center; background: var(--scrim); padding: 24px; animation: -global-scrim-in var(--dur-med) var(--ease-out); }
  .folder-prompt { width: min(480px, 100%); box-sizing: border-box; background: var(--bg-primary); color: var(--text-primary); padding: 20px 22px 18px; border: 1px solid var(--dialog-border); border-radius: var(--radius-dialog); box-shadow: var(--shadow-dialog); animation: -global-dialog-in var(--dur-med) var(--ease-out); }
  h2 { margin: 0 0 6px; font-size: 16px; font-weight: 600; }
  p { margin: 0 0 18px; overflow-wrap: anywhere; font-size: 13px; line-height: 1.5; color: var(--text-secondary); }
  label { display: block; margin-bottom: 6px; font-size: 12px; font-weight: 600; color: var(--text-secondary); }
  input { box-sizing: border-box; width: 100%; padding: 8px 10px; font-size: 13px; }
  .error { margin: 8px 0 0; color: var(--danger); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; }
  button { height: 32px; padding: 0 14px; border: 0; border-radius: var(--radius-md); background: var(--btn-bg); color: var(--text-primary); font-size: 13px; font-weight: 500; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out); }
  button:hover:not(:disabled) { background: var(--btn-bg-hover); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button:disabled { opacity: .5; cursor: default; }
  /* The submit button is the prompt's action. */
  button[type="submit"] { background: var(--accent-color); color: white; }
  button[type="submit"]:hover:not(:disabled) { background: var(--accent-hover); }
</style>

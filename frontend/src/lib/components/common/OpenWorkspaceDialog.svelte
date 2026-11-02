<script lang="ts">
  import { api } from '../../api';
  import { epoch } from '../../link';
  import { modalFocus } from '../../modalFocus';
  import { hostLabel } from '../../hosts';
  import { link } from '../../stores/link.svelte';
  import { workspaces } from '../../stores/workspaces.svelte';
  import DialogCloseButton from './DialogCloseButton.svelte';

  let path = $state('');
  let folder = $state<{ path: string; parent: string; directories: string[] } | null>(null);
  let browsing = $state(false);
  let error = $state('');
  let request = 0;
  let input: HTMLInputElement | undefined = $state();
  let owner = 0;

  $effect(() => {
    if (workspaces.open) {
      owner = epoch();
      request++;
      browsing = false;
      path = '';
      folder = null;
      error = '';
    }
  });
  $effect(() => {
    void link.dataHost; void link.phase;
    if (workspaces.open && (owner !== epoch() || link.stale || link.locked)) workspaces.dismiss();
  });

  async function browse(target = path) {
    const id = ++request;
    const started = epoch();
    browsing = true;
    error = '';
    try {
      const result = await api.browseWorkspace(target.trim());
      if (id !== request || started !== epoch() || !workspaces.open) return;
      folder = result;
      path = result.path;
    } catch (err) {
      if (id === request && started === epoch()) error = err instanceof Error ? err.message : String(err);
    } finally { if (id === request) browsing = false; }
  }
  async function nativeBrowse() {
    const started = epoch();
    error = '';
    try {
      const result = await api.pickImportDirectory();
      if (started === epoch() && workspaces.open && result) { path = result; folder = null; }
    } catch (err) { if (started === epoch()) error = err instanceof Error ? err.message : String(err); }
  }
</script>

{#if workspaces.open}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="workspace-backdrop" role="presentation" onkeydown={event => {
    if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); if (!workspaces.busy) workspaces.dismiss(); }
  }}>
    <div class="workspace-dialog" role="dialog" aria-modal="true" aria-labelledby="workspace-title" use:modalFocus>
      <header><h2 id="workspace-title">Open workspace</h2><DialogCloseButton label="Close workspace dialog" onclick={() => { if (!workspaces.busy) workspaces.dismiss(); }} /></header>
      <p class="host">Host: {link.dataHost ? hostLabel(link.dataHost) : 'Local'}</p>
      <p>Choose an existing directory. It does not need to contain any agent sessions.</p>
      <form onsubmit={event => { event.preventDefault(); void workspaces.add(path); }}>
        <label for="workspace-path">Directory</label>
        <div class="path-row">
          <input id="workspace-path" bind:this={input} bind:value={path} placeholder="Absolute path to your project" autocomplete="off" spellcheck="false" disabled={workspaces.busy} />
          {#if !link.dataHost}<button type="button" disabled={workspaces.busy} onclick={nativeBrowse}>Browse…</button>{/if}
          <button type="button" disabled={browsing || workspaces.busy} onclick={() => void browse()}>{link.dataHost ? 'Browse…' : 'List folders'}</button>
        </div>
        {#if folder}
          <div class="folders" aria-label="Workspace directories">
            <div class="folder-heading"><span>{folder.path}</span><button type="button" disabled={!folder.parent || browsing || workspaces.busy} onclick={() => void browse(folder!.parent)}>Up</button></div>
            {#each folder.directories as dir (dir)}
              <button type="button" class="folder" disabled={browsing || workspaces.busy} onclick={() => void browse(dir)}>📁 {dir.split(/[\\/]/).filter(Boolean).at(-1) ?? dir}</button>
            {:else}<p>No subdirectories. You can open this directory.</p>{/each}
          </div>
        {/if}
        {#if error || workspaces.error}<p class="error" role="alert">{error || workspaces.error}</p>{/if}
        <footer><button type="button" disabled={workspaces.busy} onclick={() => workspaces.dismiss()}>Cancel</button><button type="submit" class="primary" disabled={!path.trim() || workspaces.busy}>{workspaces.busy ? 'Opening…' : 'Open workspace'}</button></footer>
      </form>
    </div>
  </div>
{/if}

<style>
  .workspace-backdrop { position: fixed; inset: 0; z-index: 1010; background: #0008; display: grid; place-items: center; padding: 20px; }
  .workspace-dialog { width: min(620px, 100%); max-height: 85vh; overflow: auto; padding: 20px; border: 1px solid var(--border-color); border-radius: 8px; background: var(--bg-primary); color: var(--text-primary); box-shadow: 0 16px 36px #0005; }
  header, .path-row, .folder-heading, footer { display: flex; align-items: center; gap: 8px; }
  header { justify-content: space-between; } h2 { margin: 0; font-size: 18px; }
  p { font-size: 13px; color: var(--text-secondary); } .host { font-weight: 600; }
  label { display: block; font-size: 13px; margin-bottom: 6px; }
  input { min-width: 0; flex: 1; padding: 8px; border: 1px solid var(--border-color); border-radius: 4px; background: var(--bg-secondary); color: var(--text-primary); }
  button { padding: 7px 10px; border: 1px solid var(--border-color); border-radius: 4px; color: var(--text-primary); background: var(--bg-secondary); cursor: pointer; white-space: nowrap; }
  button:disabled { opacity: .5; cursor: default; } button:hover:not(:disabled) { background: var(--hover-bg); }
  button.primary { border-color: var(--accent-color); }
  button.primary:hover:not(:disabled) { background: var(--accent-hover); border-color: var(--accent-hover); color: white; }
  .folders { margin-top: 12px; max-height: 280px; overflow: auto; border: 1px solid var(--border-color); border-radius: 4px; padding: 8px; }
  .folder-heading { justify-content: space-between; font-size: 12px; overflow-wrap: anywhere; margin-bottom: 8px; }
  .folder { display: block; width: 100%; text-align: left; border: 0; background: transparent; }
  footer { justify-content: flex-end; margin-top: 20px; } .primary { background: var(--accent-color); color: white; }
  .error { color: var(--error-color, #cf222e); }
</style>

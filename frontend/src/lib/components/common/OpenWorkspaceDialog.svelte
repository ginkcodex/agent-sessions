<script lang="ts">
  import { api } from '../../api';
  import { epoch } from '../../link';
  import { modalFocus } from '../../modalFocus';
  import { hostLabel } from '../../hosts';
  import { link } from '../../stores/link.svelte';
  import { workspaces } from '../../stores/workspaces.svelte';
  import DialogCloseButton from './DialogCloseButton.svelte';
  import FileIcon from './FileIcon.svelte';

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
    <div class="workspace-dialog" role="dialog" aria-modal="true" aria-labelledby="workspace-title" aria-describedby="workspace-hint" use:modalFocus>
      <header>
        <div class="badge" aria-hidden="true">
          <svg width="22" height="22" viewBox="0 0 24 24"><path d="M3 7.5A2.5 2.5 0 0 1 5.5 5h3.6a2 2 0 0 1 1.4.6l1.4 1.4h6.6A2.5 2.5 0 0 1 21 9.5v7a2.5 2.5 0 0 1-2.5 2.5h-13A2.5 2.5 0 0 1 3 16.5z" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linejoin="round" /></svg>
        </div>
        <div class="titles">
          <div class="title-row">
            <h2 id="workspace-title">Open workspace</h2>
            <span class="host-chip" title="Host"><span class="visually-hidden">Host: </span><span class="host-dot" aria-hidden="true"></span>{link.dataHost ? hostLabel(link.dataHost) : 'Local'}</span>
          </div>
          <p id="workspace-hint" class="subtitle">Choose an existing directory. It does not need to contain any agent sessions.</p>
        </div>
        <DialogCloseButton label="Close workspace dialog" onclick={() => { if (!workspaces.busy) workspaces.dismiss(); }} />
      </header>
      <form onsubmit={event => { event.preventDefault(); void workspaces.add(path); }}>
        <div class="body">
          <div class="path-row">
            <div class="path-field text-field">
              <svg class="path-icon" width="16" height="16" viewBox="0 0 16 16" aria-hidden="true"><path d="M2 4.5A1.5 1.5 0 0 1 3.5 3h2.4l1.5 1.5h5.1A1.5 1.5 0 0 1 14 6v5.5a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 2 11.5z" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linejoin="round" /></svg>
              <input id="workspace-path" aria-label="Directory" bind:this={input} bind:value={path} placeholder="Absolute path to your project" autocomplete="off" spellcheck="false" disabled={workspaces.busy} />
            </div>
            {#if !link.dataHost}<button type="button" disabled={workspaces.busy} onclick={nativeBrowse}>Browse…</button>{/if}
            <button type="button" disabled={browsing || workspaces.busy} onclick={() => void browse()}>{link.dataHost ? 'Browse…' : 'List folders'}</button>
          </div>
          {#if folder}
            <div class="folders" role="group" aria-label="Workspace directories" aria-busy={browsing}>
              <div class="folder-heading">
                <button type="button" class="up" title="Parent directory" disabled={!folder.parent || browsing || workspaces.busy} onclick={() => void browse(folder!.parent)}>
                  <svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true"><path d="M8 13V3M3.5 7.5 8 3l4.5 4.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                  Up
                </button>
                <span class="current" title={folder.path}><bdi>{folder.path}</bdi></span>
              </div>
              <div class="folder-list">
                {#each folder.directories as dir (dir)}
                  <button type="button" class="folder" disabled={browsing || workspaces.busy} onclick={() => void browse(dir)}>
                    <FileIcon path={dir} folder />
                    <span class="name">{dir.split(/[\\/]/).filter(Boolean).at(-1) ?? dir}</span>
                    <svg class="chevron" width="14" height="14" viewBox="0 0 16 16" aria-hidden="true"><path d="M6 3.5 10.5 8 6 12.5" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" /></svg>
                  </button>
                {:else}<p class="empty">No subdirectories. You can open this directory.</p>{/each}
              </div>
            </div>
          {/if}
          {#if error || workspaces.error}<p class="error" role="alert">{error || workspaces.error}</p>{/if}
        </div>
        <footer><button type="button" disabled={workspaces.busy} onclick={() => workspaces.dismiss()}>Cancel</button><button type="submit" class="primary" disabled={!path.trim() || workspaces.busy}>{workspaces.busy ? 'Opening…' : 'Open workspace'}</button></footer>
      </form>
    </div>
  </div>
{/if}

<style>
  .workspace-backdrop { position: fixed; inset: 0; z-index: 1010; display: grid; place-items: center; padding: 20px; background: rgb(8 12 20 / 0.5); backdrop-filter: blur(3px); animation: fade-in 140ms ease-out; }
  .workspace-dialog {
    display: flex; flex-direction: column; width: min(640px, 100%); max-height: 85vh; overflow: hidden;
    border: 1px solid var(--border-color); border-radius: var(--radius-lg);
    background: var(--bg-primary); color: var(--text-primary);
    box-shadow: 0 24px 60px rgb(0 0 0 / 0.28), 0 2px 8px rgb(0 0 0 / 0.12);
    animation: pop-in 180ms cubic-bezier(.2, .9, .3, 1.15);
  }
  form { display: flex; flex-direction: column; min-height: 0; }
  p { margin: 0; }

  header { display: flex; align-items: flex-start; gap: 14px; padding: 22px 22px 18px; }
  .badge {
    display: grid; place-items: center; flex-shrink: 0; width: 44px; height: 44px; border-radius: var(--radius-lg);
    color: var(--accent-color); background: linear-gradient(140deg, color-mix(in srgb, var(--accent-color) 22%, transparent), color-mix(in srgb, var(--accent-color) 8%, transparent));
    box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--accent-color) 25%, transparent);
  }
  .titles { flex: 1; min-width: 0; }
  .title-row { display: flex; align-items: center; flex-wrap: wrap; gap: 4px 10px; }
  h2 { margin: 0; font-size: 1.1rem; font-weight: 650; letter-spacing: -0.01em; }
  .subtitle { margin-top: 4px; font-size: 0.78rem; line-height: 1.45; color: var(--text-muted); }
  .host-chip {
    display: inline-flex; align-items: center; gap: 6px; padding: 2px 9px; border-radius: 999px;
    font-size: 0.68rem; font-weight: 600; color: var(--text-secondary); background: var(--bg-secondary); border: 1px solid var(--border-color);
  }
  .host-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--status-live, #16a34a); box-shadow: 0 0 0 3px color-mix(in srgb, var(--status-live, #16a34a) 20%, transparent); }
  .visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

  .body { display: flex; flex-direction: column; gap: 12px; min-height: 0; padding: 0 22px 20px; overflow: auto; }
  .path-row { display: flex; align-items: stretch; gap: 8px; }
  /* The shared .text-field gives it the app's input look and focus ring. */
  .path-field { flex: 1; gap: 10px; padding: 0 12px; }
  .path-icon { flex-shrink: 0; color: var(--text-muted); }
  .path-field input { padding: 10px 0; font-family: var(--font-mono); }
  .path-field input::placeholder { font-family: var(--font-sans); }
  input:disabled { opacity: .6; }

  button {
    display: inline-flex; align-items: center; justify-content: center; gap: 6px; padding: 9px 16px;
    border: 1px solid var(--border-color); border-radius: var(--radius-md); background: var(--bg-primary); color: var(--text-primary);
    font-size: 0.8rem; font-weight: 500; white-space: nowrap; cursor: pointer;
    box-shadow: var(--shadow-card); transition: background-color 120ms, border-color 120ms, box-shadow 120ms, transform 80ms;
  }
  button:hover:not(:disabled) { background: var(--hover-bg); }
  button:active:not(:disabled) { transform: translateY(1px); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button:disabled { opacity: .5; cursor: default; }
  button.primary { border-color: transparent; background: var(--accent-color); color: white; font-weight: 600; box-shadow: 0 1px 2px rgb(0 0 0 / 0.15), 0 4px 12px color-mix(in srgb, var(--accent-color) 30%, transparent); }
  button.primary:hover:not(:disabled) { background: var(--accent-hover); }
  button.primary:disabled { box-shadow: none; }

  .folders { display: flex; flex-direction: column; min-height: 0; border: 1px solid var(--border-color); border-radius: var(--radius-md); overflow: hidden; }
  .folder-heading { display: flex; align-items: center; gap: 10px; padding: 8px 10px; border-bottom: 1px solid var(--border-color); background: var(--bg-secondary); }
  .folder-heading .up { padding: 5px 10px; font-size: 0.72rem; }
  .current { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; direction: rtl; text-align: left; font-family: var(--font-mono); font-size: 0.72rem; color: var(--text-secondary); }
  .folder-list { max-height: 280px; overflow: auto; padding: 6px; }
  .folder {
    display: flex; justify-content: flex-start; gap: 10px; width: 100%; padding: 7px 10px; text-align: left;
    border: 0; border-radius: var(--radius-sm); background: transparent; box-shadow: none; font-weight: 400;
  }
  .folder .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .folder .chevron { flex-shrink: 0; color: var(--text-muted); opacity: 0; transform: translateX(-4px); transition: opacity 120ms, transform 120ms; }
  .folder:hover:not(:disabled) { background: var(--active-bg); }
  .folder:hover:not(:disabled) .chevron, .folder:focus-visible .chevron { opacity: 1; transform: none; }
  .folder:active:not(:disabled) { transform: none; }
  .folder:focus-visible { outline-offset: -2px; }
  .empty { padding: 18px 10px; text-align: center; font-size: 0.75rem; color: var(--text-muted); }

  footer { display: flex; justify-content: flex-end; gap: 10px; padding: 14px 22px; border-top: 1px solid var(--border-color); background: var(--bg-secondary); }
  .error { padding: 8px 12px; border-radius: var(--radius-md); font-size: 0.75rem; color: var(--danger, #cf222e); background: color-mix(in srgb, var(--danger, #cf222e) 10%, transparent); }

  @keyframes fade-in { from { opacity: 0; } }
  @keyframes pop-in { from { opacity: 0; transform: translateY(8px) scale(.97); } }
  @media (prefers-reduced-motion: reduce) { .workspace-backdrop, .workspace-dialog { animation: none; } }
</style>

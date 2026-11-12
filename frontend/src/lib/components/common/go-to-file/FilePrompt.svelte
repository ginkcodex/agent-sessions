<script lang="ts">
  import { tick } from 'svelte';
  import { filePrompt } from '../../../stores/filePrompt.svelte';
  import Icon from '../Icon.svelte';
  import AgentIcon from '../AgentIcon.svelte';
  let panel: HTMLDivElement | undefined = $state();
  $effect(() => {
    if (!filePrompt.request) return;
    const previous = document.activeElement;
    const cancel = () => panel?.querySelector<HTMLButtonElement>('[data-cancel]')?.focus();
    void tick().then(cancel);
    // Keep focus in the prompt, even if something behind it (such as an
    // editor that just finished loading) takes focus.
    const contain = (e: FocusEvent) => {
      if (panel && e.target instanceof Node && !panel.contains(e.target)) cancel();
    };
    document.addEventListener('focusin', contain);
    return () => {
      document.removeEventListener('focusin', contain);
      if (previous instanceof HTMLElement) previous.focus();
    };
  });
  const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform);
  function key(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'Escape') { e.preventDefault(); filePrompt.answer(null); }
    // Shortcuts the dialog and editor own (find, save, close) must not reach
    // the browser either while the prompt decides.
    const mod = isMac ? e.metaKey : e.ctrlKey;
    if (mod && ['f', 's', 'w'].includes(e.key.toLowerCase())) e.preventDefault();
    if (e.key === 'Tab') {
      const buttons = [...(panel?.querySelectorAll<HTMLButtonElement>('button') ?? [])];
      const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
      e.preventDefault();
      buttons[(index + (e.shiftKey ? -1 : 1) + buttons.length) % buttons.length]?.focus();
    }
  }
</script>

{#if filePrompt.request}
  <div class="file-prompt-backdrop" role="presentation">
    <div class="file-prompt" role="alertdialog" aria-modal="true" aria-labelledby="file-prompt-title" aria-describedby="file-prompt-message" tabindex="-1" bind:this={panel} onkeydown={key}>
      <h2 id="file-prompt-title">{filePrompt.request.title}</h2>
      <div class="body">
        <p id="file-prompt-message">{filePrompt.request.message}</p>
        {#if filePrompt.request.groups.length}
          <div class="groups">
            {#each filePrompt.request.groups as group}
              <section class="group">
                <h3 class="group-head">
                  <Icon name={group.icon === 'chat' ? 'message-square' : 'square-terminal'} size={13} />
                  <span class="group-title">{group.title}</span>
                  <span class="group-count">{group.items.length + (group.more ?? 0)}</span>
                </h3>
                <ul class="items">
                  {#each group.items as item}
                    <li class="item">
                      <span class="item-icon">
                        {#if item.agent}
                          <AgentIcon agent={item.agent} size={14} />
                        {:else}
                          <Icon name={item.icon === 'terminal' ? 'square-terminal' : item.icon === 'chat' ? 'message-square' : 'bot'} size={14} />
                        {/if}
                      </span>
                      <span class="item-name">{item.name}</span>
                      {#if item.detail}<span class="item-detail">{item.detail}</span>{/if}
                      {#if item.badge}<span class="badge">{item.badge}</span>{/if}
                    </li>
                  {/each}
                  {#if group.more}
                    <li class="more">and {group.more} more</li>
                  {/if}
                </ul>
              </section>
            {/each}
          </div>
        {/if}
      </div>
      <div class="actions">
        <button type="button" class="cancel" data-cancel onclick={() => filePrompt.answer(null)}>Cancel</button>
        {#each filePrompt.request.choices as choice}
          <button type="button" class:danger={choice.danger} onclick={() => filePrompt.answer(choice.value)}>{choice.label}</button>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  .file-prompt-backdrop { position: fixed; inset: 0; z-index: 1400; display: grid; place-items: center; background: var(--scrim); padding: 24px; animation: -global-scrim-in var(--dur-med) var(--ease-out); }
  .file-prompt { width: min(480px, 100%); box-sizing: border-box; background: var(--bg-primary); color: var(--text-primary); padding: 22px 24px 18px; border: 1px solid var(--dialog-border); border-radius: var(--radius-dialog); box-shadow: var(--shadow-dialog); animation: -global-dialog-in var(--dur-med) var(--ease-out); outline: none; }
  h2 { margin: 0 0 10px; font-size: 16px; font-weight: 600; letter-spacing: -.01em; }
  .body { max-height: 50vh; overflow: auto; }
  p { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 13px; line-height: 1.55; color: var(--text-secondary); }

  /* What the prompt ends or discards, grouped so each kind reads as one. */
  .groups { display: flex; flex-direction: column; gap: 10px; margin-top: 14px; }
  .group { background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: var(--radius-md); padding: 8px 12px 6px; }
  .group-head { display: flex; align-items: center; gap: 6px; margin: 0 0 4px; color: var(--text-muted); font-size: 11px; font-weight: 600; letter-spacing: .05em; text-transform: uppercase; }
  .group-title { overflow-wrap: anywhere; }
  .group-count { margin-left: auto; flex: none; min-width: 18px; padding: 1px 6px; box-sizing: border-box; border-radius: 999px; background: var(--badge-bg); color: var(--text-secondary); font-size: 10px; font-weight: 600; letter-spacing: 0; text-align: center; }
  .items { list-style: none; margin: 0; padding: 0; }
  .item { display: flex; align-items: center; gap: 8px; padding: 6px 2px; font-size: 13px; border-top: 1px solid var(--border-color); }
  .item:first-child { border-top: 0; }
  .item-icon { display: inline-flex; flex: none; align-items: center; color: var(--text-muted); }
  .item-name { color: var(--text-primary); font-weight: 500; overflow-wrap: anywhere; }
  .item-detail { color: var(--text-muted); font-size: 12px; overflow-wrap: anywhere; }
  .badge { flex: none; margin-left: auto; padding: 1px 7px; border-radius: 999px; background: color-mix(in srgb, var(--accent-color) 14%, transparent); color: var(--accent-color); font-size: 10px; font-weight: 600; }
  .more { padding: 6px 2px 4px; border-top: 1px solid var(--border-color); color: var(--text-muted); font-size: 12px; font-style: italic; }

  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 20px; }
  button { height: 32px; padding: 0 14px; border: 0; border-radius: var(--radius-md); background: var(--btn-bg); color: var(--text-primary); font-size: 13px; font-weight: 500; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out); }
  button:hover:not(:disabled) { background: var(--btn-bg-hover); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button:disabled { opacity: .5; cursor: default; }
  button.cancel { background: transparent; color: var(--text-secondary); }
  button.cancel:hover:not(:disabled) { background: var(--btn-bg); color: var(--text-primary); }
  button.danger { background: var(--danger); color: white; }
  button.danger:hover:not(:disabled) { background: color-mix(in srgb, var(--danger) 85%, black); }
</style>

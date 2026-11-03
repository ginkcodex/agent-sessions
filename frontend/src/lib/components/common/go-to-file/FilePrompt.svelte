<script lang="ts">
  import { tick } from 'svelte';
  import { filePrompt } from '../../../stores/filePrompt.svelte';
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
      <p id="file-prompt-message">{filePrompt.request.message}</p>
      <div class="actions">
        <button type="button" data-cancel onclick={() => filePrompt.answer(null)}>Cancel</button>
        {#each filePrompt.request.choices as choice}
          <button type="button" class:danger={choice.danger} onclick={() => filePrompt.answer(choice.value)}>{choice.label}</button>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  .file-prompt-backdrop { position: fixed; inset: 0; z-index: 1400; display: grid; place-items: center; background: var(--scrim); padding: 24px; animation: -global-scrim-in var(--dur-med) var(--ease-out); }
  .file-prompt { width: min(480px, 100%); box-sizing: border-box; background: var(--bg-primary); color: var(--text-primary); padding: 20px 22px 18px; border: 1px solid var(--dialog-border); border-radius: var(--radius-dialog); box-shadow: var(--shadow-dialog); animation: -global-dialog-in var(--dur-med) var(--ease-out); }
  h2 { margin: 0 0 6px; font-size: 16px; font-weight: 600; }
  .file-prompt { outline: none; }
  h2 { margin-bottom: 8px; }
  p { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; max-height: 45vh; overflow: auto; font-size: 13px; line-height: 1.5; color: var(--text-secondary); }
  .actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 22px; }
  button { height: 32px; padding: 0 14px; border: 0; border-radius: var(--radius-md); background: var(--btn-bg); color: var(--text-primary); font-size: 13px; font-weight: 500; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out); }
  button:hover:not(:disabled) { background: var(--btn-bg-hover); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button:disabled { opacity: .5; cursor: default; }
  button.danger { background: var(--danger); color: white; }
  button.danger:hover:not(:disabled) { background: color-mix(in srgb, var(--danger) 85%, black); }
</style>

<script lang="ts">
  import { toast } from '../../stores/toast.svelte';
  import Icon from './Icon.svelte';
</script>

<!-- Bottom right, above every dialog (they top out at z-index 1200). -->
<div class="toaster" role="status" aria-live="polite">
  {#if toast.current}
    {#key toast.current.id}
      {@const message = toast.current}
      <div
        class="toast"
        class:error={message.tone === 'error'}
        class:wide={message.choices && !!message.actions?.length}
        role="presentation"
        onmouseenter={() => toast.pause('pointer')}
        onmouseleave={() => toast.resume('pointer')}
        onfocusin={() => toast.pause('focus')}
        onfocusout={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node | null)) toast.resume('focus');
        }}
      >
        <span class="toast-icon" aria-hidden="true">
          <Icon name={message.tone === 'error' ? 'circle-alert' : 'info'} size={16} strokeWidth={2.25} />
        </span>
        <div class="toast-text">
          <strong class="toast-title">{message.title}</strong>
          <p class="toast-body">{message.body}</p>
          {#if message.code}
            <code class="toast-code">{message.code}</code>
          {/if}
          {#if message.actions?.length}
            <div class="toast-actions" class:choices={message.choices} role="group" aria-label="Toast actions">
              {#each message.actions as action, index}
                <button
                  type="button"
                  class="toast-action"
                  class:primary={!message.choices && index === 0}
                  onclick={() => toast.invoke(message.id, index)}
                >
                  {action.label}
                </button>
              {/each}
            </div>
          {/if}
        </div>
        <button type="button" class="toast-close" aria-label="Dismiss" onclick={() => toast.dismiss(message.id)}>
          <Icon name="x" size={14} strokeWidth={2.25} />
        </button>
      </div>
    {/key}
  {/if}
</div>

<style>
  .toaster {
    position: fixed;
    right: 16px;
    bottom: 16px;
    z-index: 1300;
    pointer-events: none;
  }

  /* A floating card, like the menus: a tone icon, the text, and a close
     button, each in its own column. */
  .toast {
    --toast-tone: var(--accent-color);
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) auto;
    column-gap: 10px;
    align-items: start;
    box-sizing: border-box;
    width: min(360px, calc(100vw - 32px));
    max-height: calc(100dvh - 32px);
    overflow-y: auto;
    padding: 12px 10px 12px 12px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-lg);
    background: var(--bg-primary);
    color: var(--text-primary);
    box-shadow: var(--shadow-popover);
    pointer-events: auto;
    animation: toast-in var(--dur-fast, 0.15s) var(--ease-out, ease-out);
  }

  .toast.error {
    --toast-tone: var(--danger);
  }

  /* Room for full paths. */
  .toast.wide {
    width: min(460px, calc(100vw - 32px));
  }

  .toast-icon {
    display: grid;
    place-items: center;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    color: var(--toast-tone);
    background: color-mix(in srgb, var(--toast-tone) 14%, transparent);
  }

  .toast-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
    padding-top: 3px;
  }

  .toast-title {
    font-size: 0.8rem;
    font-weight: 600;
    line-height: 1.35;
  }

  .toast-body {
    margin: 3px 0 0;
    font-size: 0.75rem;
    line-height: 1.45;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .toast-code {
    align-self: flex-start;
    max-width: 100%;
    box-sizing: border-box;
    margin-top: 8px;
    padding: 3px 7px;
    border-radius: var(--radius-sm);
    background: var(--bg-secondary);
    font-family: var(--font-mono, monospace);
    font-size: 0.72rem;
    /* Fill each line rather than strand a piece of a hyphenated name. */
    word-break: break-all;
    user-select: all;
  }

  /* Buttons sit at the end, the first one filled. */
  .toast-actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 6px;
    margin-top: 10px;
  }

  .toast-action {
    padding: 5px 12px;
    border-radius: var(--radius-sm);
    background: var(--segment-track);
    color: var(--text-primary);
    font-size: 0.75rem;
    font-weight: 500;
    white-space: nowrap;
    transition: background-color var(--dur-fast, 0.15s) var(--ease-out, ease-out);
  }

  .toast-action:hover {
    background: var(--hover-bg);
  }

  .toast-action.primary {
    background: var(--accent-color);
    color: #fff;
  }

  .toast-action.primary:hover {
    background: var(--accent-hover, var(--accent-color));
  }

  /* Choices are listed one per line, in full. */
  .toast-actions.choices {
    flex-direction: column;
    align-items: stretch;
    gap: 4px;
    margin-top: 8px;
  }

  .toast-actions.choices .toast-action {
    padding: 6px 10px;
    border: 1px solid transparent;
    background: var(--bg-secondary);
    font-family: var(--font-mono, monospace);
    font-size: 0.72rem;
    text-align: left;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .toast-actions.choices .toast-action:hover {
    border-color: color-mix(in srgb, var(--accent-color) 45%, transparent);
    background: color-mix(in srgb, var(--accent-color) 10%, var(--bg-secondary));
  }

  .toast-action:focus-visible,
  .toast-close:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .toast-close {
    display: grid;
    place-items: center;
    width: 22px;
    height: 22px;
    border-radius: var(--radius-sm);
    color: var(--text-muted);
  }

  .toast-close:hover {
    background: var(--hover-bg);
    color: var(--text-primary);
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
</style>

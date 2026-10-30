<script lang="ts">
  import { toast } from '../../stores/toast.svelte';
</script>

<!-- Bottom right, above every dialog (they top out at z-index 1200). -->
<div class="toaster" role="status" aria-live="polite">
  {#if toast.current}
    {#key toast.current.id}
      {@const message = toast.current}
      <div
        class="toast"
        class:error={message.tone === 'error'}
        class:actionable={!!message.actions?.length}
        role="presentation"
        onmouseenter={() => toast.pause('pointer')}
        onmouseleave={() => toast.resume('pointer')}
        onfocusin={() => toast.pause('focus')}
        onfocusout={(event) => {
          if (!event.currentTarget.contains(event.relatedTarget as Node | null)) toast.resume('focus');
        }}
      >
        <div class="toast-text">
          <strong class="toast-title">{message.title}</strong>
          <p class="toast-body">{message.body}</p>
          {#if message.code}
            <code class="toast-code">{message.code}</code>
          {/if}
          {#if message.actions?.length}
            <div class="toast-actions" role="group" aria-label="Toast actions">
              {#each message.actions as action, index}
                <button type="button" class="toast-action" onclick={() => toast.invoke(message.id, index)}>
                  {action.label}
                </button>
              {/each}
            </div>
          {/if}
        </div>
        <button type="button" class="toast-close" aria-label="Dismiss" onclick={() => toast.dismiss(message.id)}>
          ×
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

  .toast {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    box-sizing: border-box;
    width: min(380px, calc(100vw - 32px));
    max-height: calc(100dvh - 32px);
    overflow-y: auto;
    padding: 10px 12px;
    border: 1px solid var(--border-color);
    border-left: 3px solid var(--accent-color);
    border-radius: var(--radius-md);
    background: var(--bg-tertiary);
    color: var(--text-primary);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.25);
    pointer-events: auto;
    animation: toast-in 0.15s ease-out;
  }

  .toast.error {
    border-left-color: var(--danger);
  }

  .toast-text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .toast-title {
    font-size: 0.8rem;
  }

  .toast-body {
    margin: 0;
    font-size: 0.75rem;
    line-height: 1.4;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .toast-code {
    align-self: flex-start;
    padding: 2px 6px;
    border-radius: var(--radius-sm);
    background: var(--bg-secondary);
    font-family: var(--font-mono, monospace);
    font-size: 0.75rem;
    user-select: all;
  }

  .toast.actionable {
    width: min(480px, calc(100vw - 32px));
  }

  .toast-actions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 4px;
  }

  .toast-action {
    padding: 6px 8px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-sm);
    background: var(--bg-secondary);
    color: var(--text-primary);
    font-size: 0.75rem;
    text-align: left;
    white-space: normal;
    overflow-wrap: anywhere;
  }

  .toast-action:hover {
    border-color: var(--accent-color);
  }

  .toast-action:focus-visible,
  .toast-close:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .toast-close {
    flex-shrink: 0;
    padding: 0 4px;
    font-size: 1rem;
    line-height: 1;
    color: var(--text-muted);
  }

  .toast-close:hover {
    color: var(--text-primary);
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
  }
</style>

<script lang="ts">
  import Icon from './Icon.svelte';
  import { modalFocus } from '../../modalFocus';
  import { connectionStore } from '../../stores/connection.svelte';

  let answer = $state('');
  let showPassword = $state(false);
  let submitting = $state(false);
  let inputEl = $state<HTMLInputElement | null>(null);

  let isYesNo = $derived(
    connectionStore.pendingAskpass?.prompt.toLowerCase().includes('(yes/no') ?? false
  );

  $effect(() => {
    if (connectionStore.pendingAskpass) {
      answer = '';
      showPassword = false;
      submitting = false;
      setTimeout(() => {
        inputEl?.focus();
      }, 50);
    }
  });

  async function handleSubmit(e?: Event) {
    if (e) e.preventDefault();
    if (!connectionStore.pendingAskpass || submitting) return;
    submitting = true;
    try {
      await connectionStore.askpassReply(connectionStore.pendingAskpass.id, answer);
    } finally {
      submitting = false;
      answer = '';
    }
  }

  function handleCancel() {
    if (!connectionStore.pendingAskpass) return;
    connectionStore.cancelAskpass(connectionStore.pendingAskpass.id);
    answer = '';
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      handleCancel();
    }
  }
</script>

{#if connectionStore.pendingAskpass}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="dialog-backdrop"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) handleCancel();
    }}
    onkeydown={handleKeydown}
  >
    <div
      class="dialog"
      use:modalFocus role="dialog"
      aria-modal="true"
      aria-labelledby="askpass-dialog-title"
    >
      <header class="dialog-header">
        <div class="header-title-group">
          <span class="auth-icon" aria-hidden="true"><Icon name="key-round" size={16} /></span>
          <h2 id="askpass-dialog-title">SSH Authentication</h2>
        </div>
        {#if connectionStore.host}
          <span class="host-badge">{connectionStore.host}</span>
        {/if}
      </header>

      <form onsubmit={handleSubmit}>
        <div class="dialog-body">
          <p class="prompt-text">{connectionStore.pendingAskpass.prompt}</p>

          <div class="input-wrapper">
            <input
              bind:this={inputEl}
              type={isYesNo || showPassword ? 'text' : 'password'}
              class="auth-input text-field"
              bind:value={answer}
              placeholder={isYesNo ? 'yes or no' : 'Credentials…'}
              autocomplete="off"
              autocorrect="off"
              autocapitalize="off"
              spellcheck="false"
              disabled={submitting}
            />
            {#if !isYesNo}
              <button
                type="button"
                class="toggle-reveal-btn"
                title={showPassword ? 'Hide password' : 'Show password'}
                aria-label={showPassword ? 'Hide password' : 'Show password'}
                onclick={() => (showPassword = !showPassword)}
                tabindex="-1"
              >
                {#if showPassword}<Icon name="eye-off" size={14} />{:else}<Icon name="eye" size={14} />{/if}
              </button>
            {/if}
          </div>
          <p class="security-note">
            Credentials are sent securely over a local Unix domain socket and never stored on disk.
          </p>
        </div>

        <footer class="dialog-footer">
          <button
            type="button"
            class="btn btn-secondary"
            onclick={handleCancel}
            disabled={submitting}
          >
            Cancel
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            disabled={submitting}
          >
            {submitting ? 'Authenticating…' : 'Continue'}
          </button>
        </footer>
      </form>
    </div>
  </div>
{/if}

<style>
  .dialog-backdrop {
    position: fixed;
    inset: 0;
    z-index: 120;
    background: var(--scrim);
    display: grid;
    place-items: center;
    animation: -global-scrim-in var(--dur-med) var(--ease-out);
  }

  .dialog {
    width: min(440px, 92vw);
    background: var(--bg-primary);
    color: var(--text-primary);
    border: 1px solid var(--dialog-border);
    border-radius: var(--radius-dialog);
    box-shadow: var(--shadow-dialog);
    overflow: hidden;
    animation: -global-dialog-in var(--dur-med) var(--ease-out);
  }

  .dialog-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 16px 14px 4px 20px;
  }

  .header-title-group {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .auth-icon {
    display: inline-flex;
    color: var(--text-muted);
  }

  h2 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }

  .host-badge {
    font-size: 0.75rem;
    font-family: var(--font-mono, monospace);
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    padding: 2px 8px;
    border-radius: var(--radius-sm);
  }

  .dialog-body {
    padding: 12px 20px 8px;
  }

  .prompt-text {
    margin: 0 0 12px;
    font-size: 0.85rem;
    line-height: 1.4;
    word-break: break-word;
    font-weight: 500;
  }

  .input-wrapper {
    position: relative;
    display: flex;
    align-items: center;
  }

  /* Room for the reveal button at the end. */
  .auth-input {
    width: 100%;
    padding-right: 36px;
  }

  .toggle-reveal-btn {
    position: absolute;
    right: 6px;
    background: transparent;
    border: none;
    cursor: pointer;
    display: inline-flex;
    padding: 4px;
    color: var(--text-muted);
  }

  .toggle-reveal-btn:hover {
    color: var(--text-primary);
  }

  .security-note {
    margin: 10px 0 0;
    font-size: 0.72rem;
    color: var(--text-muted);
    line-height: 1.4;
  }

  .dialog-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 20px 18px;
  }

  .btn {
    height: 32px;
    padding: 0 14px;
    font-size: 0.82rem;
    font-weight: 500;
    border-radius: var(--radius-md);
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out);
  }

  .btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .btn-secondary {
    background: var(--btn-bg);
    color: var(--text-primary);
  }

  .btn-secondary:hover:not(:disabled) {
    background: var(--btn-bg-hover);
  }

  .btn-primary {
    background: var(--accent-color);
    color: white;
  }

  .btn-primary:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
</style>

<script lang="ts">
  import DialogCloseButton from './DialogCloseButton.svelte';
  import { modalFocus } from '../../modalFocus';
  import {
    connectionStore,
    getSavedHostEnv,
    saveHostEnv,
  } from '../../stores/connection.svelte';
  import { api } from '../../api';
  import { hostLabel } from '../../hosts';

  interface Props {
    open: boolean;
    host: string;
    onClose: () => void;
  }

  let { open = false, host = '', onClose }: Props = $props();

  let envMap = $state<Record<string, string>>({});
  let customKey = $state('');
  let customVal = $state('');

  const commonKeys = [
    { key: 'CLAUDE_CONFIG_DIR', desc: 'Claude Code configuration and session directory' },
    { key: 'CODEX_HOME', desc: 'Codex sessions directory' },
    { key: 'XDG_CONFIG_HOME', desc: 'Config base directory (~/.config)' },
    { key: 'XDG_DATA_HOME', desc: 'Data base directory (~/.local/share)' },
  ];

  $effect(() => {
    if (open && host) {
      envMap = { ...getSavedHostEnv(host) };
      customKey = '';
      customVal = '';
    }
  });

  function handleAddCustom() {
    const k = customKey.trim().toUpperCase();
    if (!k) return;
    envMap[k] = customVal.trim();
    customKey = '';
    customVal = '';
  }

  function handleRemoveKey(key: string) {
    const next = { ...envMap };
    delete next[key];
    envMap = next;
  }

  async function handleSave() {
    if (!host) return;
    saveHostEnv(host, envMap);
    if (connectionStore.isRemote && connectionStore.host === host) {
      await api.setHostEnv(envMap);
    }
    onClose();
  }
</script>

{#if open}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="dialog-backdrop"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onClose();
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        onClose();
      }
    }}
  >
    <div
      class="dialog"
      use:modalFocus role="dialog"
      aria-modal="true"
      aria-labelledby="host-env-title"
    >
      <header class="dialog-header">
        <h2 id="host-env-title">Environment Overrides: {hostLabel(host)}</h2>
        <DialogCloseButton label="Close dialog" title="Close" onclick={onClose} />
      </header>

      <div class="dialog-body">
        <p class="body-intro">
          Set environment variable overrides passed to <code>agent-sessions</code> on <strong>{hostLabel(host)}</strong>.
          These are applied during session startup.
        </p>

        <div class="env-list">
          {#each commonKeys as item}
            <div class="env-row">
              <div class="env-info">
                <span class="env-key">{item.key}</span>
                <span class="env-desc">{item.desc}</span>
              </div>
              <input
                type="text"
                class="env-input text-field"
                placeholder="Default"
                bind:value={envMap[item.key]}
                autocomplete="off"
                autocorrect="off"
                autocapitalize="off"
                spellcheck="false"
              />
            </div>
          {/each}

          {#each Object.entries(envMap) as [key, val]}
            {#if !commonKeys.some((c) => c.key === key)}
              <div class="env-row custom">
                <div class="env-info">
                  <span class="env-key">{key}</span>
                </div>
                <div class="custom-input-group">
                  <input
                    type="text"
                    class="env-input text-field"
                    bind:value={envMap[key]}
                    autocomplete="off"
                    autocorrect="off"
                    autocapitalize="off"
                    spellcheck="false"
                  />
                  <button
                    type="button"
                    class="remove-key-btn"
                    title="Remove override"
                    onclick={() => handleRemoveKey(key)}
                  >
                    ✕
                  </button>
                </div>
              </div>
            {/if}
          {/each}
        </div>

        <div class="add-custom-section">
          <h4>Add Override</h4>
          <div class="custom-form">
            <input
              type="text"
              class="env-input text-field"
              placeholder="VARIABLE_NAME"
              bind:value={customKey}
              autocomplete="off"
              autocorrect="off"
              autocapitalize="off"
              spellcheck="false"
            />
            <input
              type="text"
              class="env-input text-field"
              placeholder="Value"
              bind:value={customVal}
              autocomplete="off"
              autocorrect="off"
              autocapitalize="off"
              spellcheck="false"
            />
            <button
              type="button"
              class="btn btn-secondary"
              disabled={!customKey.trim()}
              onclick={handleAddCustom}
            >
              Add
            </button>
          </div>
        </div>
      </div>

      <footer class="dialog-footer">
        <button type="button" class="btn btn-secondary" onclick={onClose}>
          Cancel
        </button>
        <button type="button" class="btn btn-primary" onclick={handleSave}>
          Save Overrides
        </button>
      </footer>
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
    width: min(540px, 92vw);
    max-height: 85vh;
    display: flex;
    flex-direction: column;
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

  h2 {
    margin: 0;
    font-size: 15px;
    font-weight: 600;
  }


  .dialog-body {
    padding: 12px 20px 8px;
    overflow-y: auto;
  }

  .body-intro {
    margin: 0 0 16px;
    font-size: 0.8rem;
    color: var(--text-secondary);
    line-height: 1.4;
  }

  .env-list {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .env-row {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding-bottom: 10px;
    border-bottom: 1px solid var(--border-color);
  }

  .env-info {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }

  .env-key {
    font-size: 0.78rem;
    font-family: var(--font-mono, monospace);
    font-weight: 600;
  }

  .env-desc {
    font-size: 0.7rem;
    color: var(--text-muted);
  }

  .custom-input-group {
    display: flex;
    gap: 8px;
    align-items: center;
  }

  .custom-input-group .env-input {
    flex: 1;
  }

  .remove-key-btn {
    background: var(--btn-bg);
    color: var(--text-secondary);
    border-radius: var(--radius-sm);
    padding: 4px 8px;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out);
  }

  .remove-key-btn:hover {
    color: var(--danger);
    background: color-mix(in srgb, var(--danger) 12%, transparent);
  }

  .remove-key-btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .add-custom-section {
    margin-top: 16px;
    padding-top: 12px;
  }

  h4 {
    margin: 0 0 8px;
    font-size: 0.8rem;
    color: var(--text-secondary);
  }

  .custom-form {
    display: flex;
    gap: 8px;
  }

  .custom-form input {
    flex: 1;
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

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
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
</style>

<script lang="ts">
  import Icon from './Icon.svelte';
  import DialogCloseButton from './DialogCloseButton.svelte';
  import { exporter } from '../../stores/export.svelte';
  import { BUDGET_PRESETS } from '../../portable';
  import { formatBytes, formatTokens } from '../../format';
  import type { ExportProfile, RedactionCounts } from '../../types';

  const PROFILES: { id: ExportProfile; title: string; detail: string }[] = [
    {
      id: 'complete',
      title: 'Complete',
      detail: 'Restorable. Includes the original session records.',
    },
    {
      id: 'share-safe',
      title: 'Share-safe',
      detail: 'Redacted handoff only. Secrets are removed and nothing native is included.',
    },
  ];

  function redactionTotal(counts: RedactionCounts | undefined): number {
    if (!counts) return 0;
    return (counts.pem ?? 0) + (counts.token ?? 0) + (counts.jwt ?? 0) + (counts.assignment ?? 0) + (counts.home ?? 0);
  }

  function handleClose() {
    exporter.close();
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && exporter.dialogOpen) handleClose();
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if exporter.dialogOpen && exporter.session}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div
    class="dialog-backdrop"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) handleClose();
    }}
  >
    <div class="dialog" role="dialog" aria-modal="true" aria-labelledby="export-dialog-title">
      <header class="dialog-header">
        <div class="header-title-group">
          <h2 id="export-dialog-title">Export Session</h2>
          <span class="source-badge">
            From <span class="agent-name">{exporter.session.ref.agent}</span>
          </span>
        </div>
        <DialogCloseButton label="Close dialog" onclick={handleClose} />
      </header>

      <div class="dialog-body">
        <section class="section-group">
          <span class="group-label">Profile</span>
          <div class="profile-cards">
            {#each PROFILES as card (card.id)}
              <button
                type="button"
                class="profile-card"
                class:selected={exporter.profile === card.id}
                aria-pressed={exporter.profile === card.id}
                onclick={() => exporter.setProfile(card.id)}
              >
                <span class="profile-title">{card.title}</span>
                <span class="profile-detail">{card.detail}</span>
              </button>
            {/each}
          </div>
        </section>

        {#if exporter.profile === 'complete'}
          <div class="warning-banner" role="note">
            {exporter.preview?.warning ||
              "A complete bundle contains the session's original records and can include secrets such as tokens, keys, and passwords."}
          </div>
        {/if}

        <div class="controls-row">
          <div class="control-col">
            <span class="group-label">Handoff Budget</span>
            <div class="budget-buttons">
              {#each BUDGET_PRESETS as preset (preset.id)}
                <button
                  type="button"
                  class="budget-btn"
                  class:selected={exporter.budget === preset.tokens}
                  title={preset.description}
                  onclick={() => exporter.setBudget(preset.tokens)}
                >
                  {preset.label}
                </button>
              {/each}
            </div>
          </div>

          <div class="control-col">
            <span class="group-label">Options</span>
            <div class="toggles-group">
              <label class="checkbox-label">
                <input
                  type="checkbox"
                  checked={exporter.includeReasoning}
                  onchange={(e) => exporter.setIncludeReasoning(e.currentTarget.checked)}
                />
                <span>Include reasoning</span>
              </label>
              <label class="checkbox-label" class:forced={exporter.redactionForced}>
                <input
                  type="checkbox"
                  checked={exporter.redactSecrets || exporter.redactionForced}
                  disabled={exporter.redactionForced}
                  onchange={(e) => exporter.setRedactSecrets(e.currentTarget.checked)}
                />
                <span>Redact secrets{exporter.redactionForced ? ' (required)' : ''}</span>
              </label>
            </div>
          </div>
        </div>

        {#if exporter.error}
          <div class="error-banner" role="alert">{exporter.error}</div>
        {/if}

        {#if exporter.savedPath}
          <div class="success-banner" role="status">
            ✓ Bundle saved to: <code>{exporter.savedPath}</code>
          </div>
        {/if}

        {#if exporter.preview}
          {@const total = redactionTotal(exporter.preview.redaction)}
          <div class="report-summary">
            <div class="stat-pill">
              <span class="stat-label">Sessions:</span>
              <strong class="stat-value">{exporter.preview.sessions}</strong>
            </div>
            <div class="stat-pill">
              <span class="stat-label">Size:</span>
              <strong class="stat-value">
                {exporter.preview.nativeFiles > 0
                  ? `${exporter.preview.nativeFiles} files, ${formatBytes(exporter.preview.nativeBytes)}`
                  : 'handoff only'}
              </strong>
            </div>
            <div class="stat-pill">
              <span class="stat-label">Handoff:</span>
              <strong class="stat-value">{formatTokens(exporter.preview.handoffTokens)} tokens</strong>
            </div>
            {#if total > 0}
              <div class="stat-pill redact-pill">
                <span><Icon name="shield-check" size={12} /> {total} redacted</span>
              </div>
            {/if}
            {#if exporter.preview.fidelity?.overflow?.length}
              <div class="stat-pill warn-pill" title={exporter.preview.fidelity.overflow.join('\n')}>
                <span><Icon name="triangle-alert" size={12} /> {exporter.preview.fidelity.overflow.length} outputs over the 64 MiB cap</span>
              </div>
            {/if}
          </div>
        {:else if exporter.loading}
          <div class="loading-state"><p>Estimating export…</p></div>
        {/if}
      </div>

      <footer class="dialog-footer">
        <button type="button" class="btn secondary-btn" onclick={handleClose}>Cancel</button>
        <button
          type="button"
          class="btn primary-btn"
          disabled={exporter.exporting || !exporter.preview}
          onclick={() => exporter.save()}
        >
          {exporter.exporting ? 'Exporting…' : 'Export…'}
        </button>
      </footer>
    </div>
  </div>
{/if}

<style>
  .dialog-backdrop {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--scrim);
    animation: -global-scrim-in var(--dur-med) var(--ease-out);
  }

  .dialog {
    width: 640px;
    max-width: calc(100vw - 32px);
    max-height: calc(100vh - 48px);
    display: flex;
    flex-direction: column;
    background: var(--bg-primary);
    border: 1px solid var(--dialog-border);
    border-radius: var(--radius-dialog);
    box-shadow: var(--shadow-dialog);
    overflow: hidden;
    animation: -global-dialog-in var(--dur-med) var(--ease-out);
  }

  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 16px 12px 20px;
  }

  .header-title-group {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .header-title-group h2 {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .source-badge {
    font-size: 0.75rem;
    color: var(--text-muted);
  }

  .agent-name {
    font-weight: 500;
    color: var(--text-secondary);
  }



  .dialog-body {
    flex: 1;
    overflow-y: auto;
    padding: 4px 20px 16px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .section-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .group-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-secondary);
  }

  .profile-cards {
    display: flex;
    gap: 8px;
  }

  .profile-card {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
    padding: 12px 14px;
    border-radius: var(--radius-md);
    background: var(--btn-bg);
    color: var(--text-secondary);
    text-align: left;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
  }

  .profile-card:hover {
    background: var(--btn-bg-hover);
  }

  .profile-card.selected {
    background: var(--active-bg);
    box-shadow: inset 0 0 0 1.5px var(--accent-color);
  }

  .profile-card:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .profile-title {
    font-size: 0.85rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .profile-detail {
    font-size: 0.75rem;
    line-height: 1.4;
  }

  .warning-banner {
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    background: rgba(245, 158, 11, 0.15);
    border: 1px solid rgba(245, 158, 11, 0.4);
    color: #f59e0b;
    font-size: 0.8rem;
    line-height: 1.4;
  }

  .controls-row {
    display: flex;
    gap: 16px;
  }

  .control-col {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .budget-buttons {
    display: flex;
    gap: 4px;
  }

  .budget-btn {
    flex: 1;
    padding: 6px 8px;
    border-radius: var(--radius-md);
    background: var(--btn-bg);
    color: var(--text-secondary);
    font-size: 0.75rem;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), box-shadow var(--dur-fast) var(--ease-out);
  }

  .budget-btn:hover {
    background: var(--btn-bg-hover);
    color: var(--text-primary);
  }

  .budget-btn.selected {
    background: var(--active-bg);
    box-shadow: inset 0 0 0 1.5px var(--accent-color);
    color: var(--text-primary);
    font-weight: 600;
  }

  .budget-btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .toggles-group {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .checkbox-label {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 0.8rem;
    color: var(--text-secondary);
    cursor: pointer;
    user-select: none;
  }

  .checkbox-label.forced {
    cursor: default;
  }

  .error-banner {
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    background: rgba(239, 68, 68, 0.15);
    border: 1px solid var(--danger);
    color: var(--danger);
    font-size: 0.8rem;
  }

  .success-banner {
    padding: 8px 12px;
    border-radius: var(--radius-sm);
    background: rgba(16, 185, 129, 0.15);
    border: 1px solid var(--success, #10b981);
    color: var(--success, #10b981);
    font-size: 0.8rem;
    word-break: break-all;
  }

  .report-summary {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }

  .stat-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 999px;
    background: var(--btn-bg);
    border: 1px solid transparent;
    font-size: 0.75rem;
    color: var(--text-secondary);
  }

  .stat-value {
    color: var(--text-primary);
  }

  .warn-pill {
    background: rgba(245, 158, 11, 0.15);
    border-color: rgba(245, 158, 11, 0.4);
    color: #f59e0b;
  }

  .redact-pill {
    background: rgba(139, 92, 246, 0.15);
    border-color: rgba(139, 92, 246, 0.4);
    color: #a78bfa;
  }

  .loading-state {
    padding: 12px;
    text-align: center;
    color: var(--text-muted);
    font-size: 0.85rem;
  }

  .dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 20px 18px;
  }

  .btn {
    height: 32px;
    padding: 0 16px;
    border-radius: var(--radius-md);
    font-size: 0.8rem;
    font-weight: 500;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out);
  }

  .btn:focus-visible {
    outline: 2px solid var(--accent-color);
    outline-offset: 2px;
  }

  .primary-btn {
    background: var(--accent-color);
    color: white;
  }

  .primary-btn:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .secondary-btn {
    background: var(--btn-bg);
    color: var(--text-primary);
  }

  .secondary-btn:hover:not(:disabled) {
    background: var(--btn-bg-hover);
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>

<script lang="ts">
  import { severityLabels, staleKind } from '../../../codeCheckLabels';
  import { severityRank } from '../../../problemOverview';
  import type { SourceDiagnosticProjection } from '../../../sourceDiagnostics';

  // Problems of this file the source cannot mark: file-level findings and
  // results that may predate the text shown. One line above the source, with
  // the recovery that applies; Problems lists each of them.
  let { diagnostics, recovery }: {
    diagnostics: SourceDiagnosticProjection;
    /** Explicit, described recovery for results that are actually stale (never offered for file-level findings). */
    recovery?: { label: string; description: string; disabled?: boolean; run: () => void };
  } = $props();

  let unplaced = $derived(diagnostics.messages.filter(m => m.notice));
  let stale = $derived(unplaced.filter(m => staleKind(m.kind)).length);
  let fileLevel = $derived(unplaced.filter(m => m.kind === 'fileLevel').length);
  let other = $derived(unplaced.length - stale - fileLevel);
  let worst = $derived(unplaced.map(m => m.finding.diagnostic.severity).sort((a, b) => severityRank[a] - severityRank[b])[0]);
  let summary = $derived([
    fileLevel && `${fileLevel} file-level`,
    stale && `${stale} not current`,
    other && `${other} outside the text shown`,
  ].filter(Boolean).join(' · '));
  let details = $derived(unplaced.map(m => `${severityLabels[m.finding.diagnostic.severity]}: ${m.finding.diagnostic.message}\n  ${m.notice}`).join('\n'));
</script>

{#if unplaced.length}
  <div class="unplaced" role="status">
    <span class="dot sev-{worst}" aria-hidden="true"></span>
    <span class="text" title={details}>{unplaced.length === 1 ? '1 problem' : `${unplaced.length} problems`} not marked in the source ({summary}); see Problems.</span>
    {#if stale && recovery}
      <button type="button" title={recovery.description} disabled={recovery.disabled} onclick={recovery.run}>{recovery.label}</button>
    {/if}
  </div>
{/if}

<style>
  .unplaced {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-shrink: 0;
    min-height: 26px;
    padding: 2px 12px;
    border-bottom: 1px solid var(--border-color);
    background: var(--bg-secondary);
    font-size: 12px;
    color: var(--text-secondary);
  }

  .text {
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .dot {
    width: 7px;
    height: 7px;
    flex-shrink: 0;
    border-radius: 50%;
    background: var(--text-muted);
  }

  .dot.sev-error { background: #f85149; }
  .dot.sev-warning { background: #d29922; }
  .dot.sev-information { background: #2f81f7; }
  .dot.sev-hint { background: #8250df; }

  button {
    flex-shrink: 0;
    margin-left: auto;
    padding: 1px 8px;
    border: 1px solid var(--border-color);
    border-radius: var(--radius-sm);
    background: var(--btn-bg);
    font: inherit;
    color: var(--text-primary);
    cursor: pointer;
  }

  button:hover:not(:disabled) {
    background: var(--btn-bg-hover);
  }

  button:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>

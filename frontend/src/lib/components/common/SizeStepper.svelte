<script lang="ts">
  // − size + and Reset for a body text size (Settings → Appearance).
  import { FONT_SIZES, stepSize, type FontSizeKind } from '../../appearance';

  interface Props {
    kind: FontSizeKind;
    /** e.g. "Session text size"; names the group and its buttons. */
    label: string;
    /** The chosen size, or null for the default. */
    value: number | null;
    onChange: (size: number | null) => void;
  }

  let { kind, label, value, onChange }: Props = $props();

  let range = $derived(FONT_SIZES[kind]);
  let current = $derived(value ?? range.default);
  // The editor's default (0.78rem) is not a whole or half pixel.
  let shown = $derived(`${Math.round(current * 2) / 2}px${value === null ? ' (default)' : ''}`);
  let name = $derived(label.toLowerCase());
</script>

<div class="stepper" role="group" aria-label={label}>
  <button type="button" class="step" aria-label="Decrease {name}" title="Smaller" disabled={current <= range.min} onclick={() => onChange(stepSize(kind, value, -1))}>−</button>
  <span class="value" aria-live="polite">{shown}</span>
  <button type="button" class="step" aria-label="Increase {name}" title="Larger" disabled={current >= range.max} onclick={() => onChange(stepSize(kind, value, 1))}>+</button>
  <button type="button" class="reset" aria-label="Reset {name}" disabled={value === null} onclick={() => onChange(null)}>Reset</button>
</div>

<style>
  .stepper { display: flex; align-items: center; gap: 6px; }
  button { flex-shrink: 0; height: 24px; border: none; border-radius: var(--radius-sm); color: var(--text-primary); background: var(--btn-bg); font-size: 0.72rem; font-weight: 500; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out); }
  button:hover:not(:disabled) { background: var(--btn-bg-hover); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button:disabled { opacity: 0.45; cursor: default; }
  .step { width: 28px; font-size: 0.85rem; line-height: 1; }
  .reset { margin-left: 6px; padding: 0 10px; }
  .value { min-width: 8.5em; text-align: center; font-size: 0.78rem; font-variant-numeric: tabular-nums; color: var(--text-primary); }
</style>

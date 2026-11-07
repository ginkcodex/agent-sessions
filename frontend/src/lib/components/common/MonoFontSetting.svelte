<script lang="ts">
  // A monospace font picker (installed fonts, the app default, Refresh and a
  // sample), shared by the terminal and editor font settings.
  import { onMount } from 'svelte';
  import { installedFonts } from '../../stores/installedFonts.svelte';
  import { monoFontFamily } from '../../terminalFont';
  import Dropdown from './Dropdown.svelte';

  interface Props {
    heading: string;
    ariaLabel: string;
    /** The chosen family; '' is the app default. */
    value: string;
    onChange: (font: string) => void;
    hint: string;
    sampleLabel: string;
    sampleLines: string[];
    /** The sample's font size, a CSS length; 13px by default. */
    sampleSize?: string;
  }

  let { heading, ariaLabel, value, onChange, hint, sampleLabel, sampleLines, sampleSize }: Props = $props();

  let fonts = $derived(installedFonts.fonts);
  let loading = $derived(installedFonts.loading);
  let error = $derived(installedFonts.error);
  let selectedMissing = $derived(Boolean(value) && !loading && !error && !fonts.includes(value));
  let fontOptions = $derived([
    { value: '', label: 'App default (JetBrains Mono)' },
    ...(value && !fonts.includes(value) ? [{ value, label: selectedMissing ? `${value} (unavailable)` : value }] : []),
    ...fonts.map(font => ({ value: font, label: font })),
  ]);

  onMount(() => { void installedFonts.ensure(); });
</script>

<div class="font-setting">
  <h4>{heading}</h4>
  <div class="font-row">
    <Dropdown ariaLabel={ariaLabel} options={fontOptions} {value} disabled={loading} {onChange} />
    <button type="button" onclick={() => void installedFonts.refresh()} disabled={loading} aria-label="Refresh installed fonts">{loading ? 'Loading…' : 'Refresh'}</button>
  </div>
  <p class="hint">{hint}</p>
  {#if selectedMissing}<p class="error">The selected font is no longer installed. A system monospace fallback will be used.</p>{/if}
  {#if error}<p class="error" role="alert">Could not list installed fonts: {error}</p>{/if}
  <div class="sample" style:font-family={monoFontFamily(value, 'var(--font-mono)')} style:font-size={sampleSize} aria-label={sampleLabel}>
    {#each sampleLines as line, i}{#if i > 0}<br />{/if}{line}{/each}
  </div>
</div>

<style>
  .font-setting { margin-top: 20px; }
  h4 { margin: 0 0 8px; font-size: 0.85rem; }
  .font-row { display: flex; align-items: center; gap: 8px; min-width: 0; }
  button { flex-shrink: 0; height: 24px; padding: 0 10px; border: none; border-radius: var(--radius-sm); color: var(--text-primary); background: var(--btn-bg); font-size: 0.72rem; font-weight: 500; cursor: pointer; transition: background-color var(--dur-fast) var(--ease-out); }
  button:hover:not(:disabled) { background: var(--btn-bg-hover); }
  button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: 2px; }
  button:disabled { opacity: 0.6; }
  .hint, .error { font-size: 0.78rem; line-height: 1.5; margin: 8px 0; }
  .hint { color: var(--text-muted); } .error { color: var(--danger, #ef4444); }
  .sample { padding: 12px; border-radius: var(--radius-md); background: var(--bg-secondary); font-size: 13px; line-height: 1.6; overflow-x: auto; white-space: nowrap; }
</style>

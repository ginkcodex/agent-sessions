<script lang="ts">
  import type { Snippet } from 'svelte';
  import Splitter from '../Splitter.svelte';
  let { toolbar, branches, commits, details, diff, diffOpen = false, onCloseDiff = () => {} }: {
    toolbar?: Snippet; branches?: Snippet; commits?: Snippet; details?: Snippet; diff?: Snippet;
    diffOpen?: boolean; onCloseDiff?: () => void;
  } = $props();
  function stored(key: string, fallback: number, min: number, max: number) {
    try { const raw = localStorage.getItem(`git-history:${key}`); const value = raw === null ? fallback : Number(raw); return Number.isFinite(value) ? Math.max(min, Math.min(max, value)) : fallback; } catch { return fallback; }
  }
  let left = $state(stored('branches', 18, 12, 30));
  let right = $state(stored('details', 30, 22, 42));
  let bottom = $state(stored('diff', 50, 25, 70));
  let browser: HTMLDivElement | undefined = $state();
  let layout: HTMLDivElement | undefined = $state();
  function resize(key: 'branches' | 'details' | 'diff', delta: number) {
    const vertical = key === 'diff';
    const span = vertical ? layout?.clientHeight : browser?.clientWidth;
    if (!span) return 0;
    const before = key === 'branches' ? left : key === 'details' ? right : bottom;
    const value = Math.max(key === 'branches' ? 12 : key === 'details' ? 22 : 25, Math.min(key === 'branches' ? 30 : key === 'details' ? 42 : 70, before + (key === 'branches' ? delta : -delta) / span * 100));
    if (key === 'branches') left = value; else if (key === 'details') right = value; else bottom = value;
    try { localStorage.setItem(`git-history:${key}`, String(value)); } catch { /* Storage may be unavailable. */ }
    return (value - before) / 100 * span * (key === 'branches' ? 1 : -1);
  }
</script>

<div class="history-layout" bind:this={layout}>
  <div class="history-toolbar">{@render toolbar?.()}</div>
  <div class="history-browser" bind:this={browser} style:flex-basis={diffOpen ? `${100 - bottom}%` : '100%'}>
    <aside class="history-branches" style:width={`${left}%`} aria-label="History branches">{@render branches?.()}</aside>
    <div class="branch-split"><Splitter ariaLabel="Resize history branches" onResize={d => resize('branches', d)} onEqualize={() => left = 18} /></div>
    <section class="history-commits" aria-label="Commit history">{@render commits?.()}</section>
    <div class="detail-split"><Splitter ariaLabel="Resize commit details" onResize={d => resize('details', d)} onEqualize={() => right = 30} /></div>
    <aside class="history-details" style:width={`${right}%`} aria-label="Changed files and commit details">{@render details?.()}</aside>
  </div>
  {#if diffOpen}
    <Splitter direction="vertical" ariaLabel="Resize historical diff" onResize={d => resize('diff', d)} onEqualize={() => bottom = 50} />
    <section class="history-diff" style:flex-basis={`${bottom}%`} aria-label="Historical file diff">
      <div class="diff-heading"><span>Historical diff · Read-only</span><button type="button" onclick={onCloseDiff} aria-label="Close historical diff">×</button></div>
      {@render diff?.()}
    </section>
  {/if}
</div>

<style>
  .history-layout { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; container-type: inline-size; }
  .history-toolbar { display: flex; gap: 12px; align-items: center; padding: 8px 12px; border-bottom: 1px solid var(--border-color); flex-shrink: 0; }
  .history-browser { display: flex; min-height: 100px; overflow: hidden; flex-grow: 1; }
  .history-branches, .history-details { flex-shrink: 0; min-width: 0; overflow: auto; }
  .history-branches { border-right: 1px solid var(--border-color); }
  .history-details { border-left: 1px solid var(--border-color); }
  .history-commits { flex: 1; min-width: 0; overflow: auto; }
  .branch-split, .detail-split { display: flex; width: 0; }
  .history-diff { display: flex; flex-direction: column; min-height: 120px; min-width: 0; overflow: hidden; }
  .diff-heading { display: flex; flex-shrink: 0; justify-content: space-between; align-items: center; padding: 3px 10px; font-size: .75rem; line-height: 1.4; color: var(--text-secondary); border-bottom: 1px solid var(--border-color); }
  .diff-heading button { font: inherit; background: transparent; border: 0; color: inherit; cursor: pointer; font-size: 18px; line-height: 1; }
  @container (max-width: 850px) {
    .history-branches, .branch-split { display: none; }
    .history-details { width: 36% !important; }
  }
  @container (max-width: 580px) {
    .history-browser { flex-direction: column; }
    .history-details { width: 100% !important; max-height: 42%; border-left: 0; border-top: 1px solid var(--border-color); }
    .detail-split { display: none; }
  }
</style>

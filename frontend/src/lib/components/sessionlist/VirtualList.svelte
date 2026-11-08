<script lang="ts" generics="T">
  import type { Snippet } from 'svelte';
  import { onMount } from 'svelte';

  interface Props {
    items: T[];
    itemHeight?: number;
    overscan?: number;
    /** ARIA role and name for the scroll container, such as a tree. */
    role?: string;
    label?: string;
    children: Snippet<[item: T, index: number]>;
  }

  let { items, itemHeight = 84, overscan = 5, role, label, children }: Props = $props();

  /** Scrolls the least needed to show the item at index whole. */
  export function ensureVisible(index: number): void {
    if (!containerEl || index < 0 || index >= items.length) return;
    const top = index * itemHeight;
    const bottom = top + itemHeight;
    if (top < containerEl.scrollTop) {
      containerEl.scrollTop = top;
    } else if (bottom > containerEl.scrollTop + containerEl.clientHeight) {
      containerEl.scrollTop = bottom - containerEl.clientHeight;
    }
  }

  let containerEl: HTMLElement | null = $state(null);
  let scrollTop = $state(0);
  let clientHeight = $state(600);

  onMount(() => {
    if (containerEl) {
      clientHeight = containerEl.clientHeight;
    }
    // Keep clientHeight fresh when the pane is resized (window or splitter).
    if (typeof ResizeObserver !== 'undefined' && containerEl) {
      const ro = new ResizeObserver((entries) => {
        for (const entry of entries) {
          clientHeight = entry.contentRect.height;
        }
      });
      ro.observe(containerEl);
      return () => ro.disconnect();
    }
  });

  function handleScroll(e: Event) {
    const el = e.currentTarget as HTMLElement;
    scrollTop = el.scrollTop;
    clientHeight = el.clientHeight;
  }

  let totalHeight = $derived(items.length * itemHeight);
  let startIndex = $derived(Math.max(0, Math.floor(scrollTop / itemHeight) - overscan));
  let endIndex = $derived(
    Math.min(items.length, Math.ceil((scrollTop + clientHeight) / itemHeight) + overscan)
  );

  let visibleItems = $derived(
    items.slice(startIndex, endIndex).map((item, idx) => ({
      item,
      index: startIndex + idx,
      top: (startIndex + idx) * itemHeight,
    }))
  );
</script>

<div
  bind:this={containerEl}
  class="virtual-list-container"
  {role}
  aria-label={label}
  onscroll={handleScroll}
>
  <div class="virtual-list-phantom" style="height: {totalHeight}px;">
    {#each visibleItems as { item, index, top } (index)}
      <div
        class="virtual-list-item"
        style="height: {itemHeight}px; transform: translateY({top}px);"
      >
        {@render children(item, index)}
      </div>
    {/each}
  </div>
</div>

<style>
  .virtual-list-container {
    width: 100%;
    height: 100%;
    overflow-y: auto;
    position: relative;
    /* Own compositing layer for the scroller. Without it WebKitGTK drops the
       Linux scrollbar near the top of the list and flashes it on row hover.
       Verified natively in the desktop app (SS-07). */
    transform: translateZ(0);
    -webkit-overflow-scrolling: touch;
  }

  .virtual-list-phantom {
    width: 100%;
    position: relative;
    overflow: hidden;
  }

  .virtual-list-item {
    position: absolute;
    top: 0;
    left: 0;
    width: 100%;
    box-sizing: border-box;
    contain: content;
  }
</style>

<script lang="ts" module>
  export interface ContextMenuItem {
    label: string;
    onSelect: () => void;
    disabled?: boolean;
    /** Shown as a tooltip; explains why a disabled item is unavailable. */
    title?: string;
    /** Draws a divider above this item, starting a new group. */
    separator?: boolean;
  }
</script>

<script lang="ts">
  import { onMount } from 'svelte';

  interface Props {
    /** Pointer position in viewport coordinates. */
    x: number;
    y: number;
    items: ContextMenuItem[];
    /** Text above the items, such as the folder a list of places shares. */
    heading?: string;
    ariaLabel: string;
    onClose: () => void;
  }

  let { x, y, items, heading, ariaLabel, onClose }: Props = $props();

  const MARGIN = 4;

  let menu: HTMLDivElement | undefined = $state();
  let left = $state(0);
  let top = $state(0);
  let activeIndex = $state(0);
  // Where focus returns when the menu is dismissed with Escape.
  let returnFocus: HTMLElement | null = null;

  // Keep the menu inside the viewport, flipping it left or up near an edge.
  function place() {
    if (!menu) return;
    // Layout size: the opening animation scales the drawn box.
    const { offsetWidth: width, offsetHeight: height } = menu;
    const maxLeft = window.innerWidth - width - MARGIN;
    const maxTop = window.innerHeight - height - MARGIN;
    left = Math.max(MARGIN, x > maxLeft ? Math.min(x - width, maxLeft) : x);
    top = Math.max(MARGIN, y > maxTop ? Math.min(y - height, maxTop) : y);
  }

  $effect(() => {
    void x;
    void y;
    void items.length;
    place();
  });

  onMount(() => {
    returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    focusItem(0);
  });

  function buttons(): HTMLButtonElement[] {
    return menu ? Array.from(menu.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')) : [];
  }

  function focusItem(index: number) {
    const list = buttons();
    if (!list.length) return;
    activeIndex = Math.max(0, Math.min(list.length - 1, index));
    list[activeIndex].focus();
  }

  function close(refocus: boolean) {
    onClose();
    if (refocus) returnFocus?.focus();
  }

  function choose(item: ContextMenuItem) {
    if (item.disabled) return;
    close(true);
    item.onSelect();
  }

  // Stop propagation so parent keyboard navigation (e.g. the session list's
  // arrow keys) does not also react while the menu is open.
  function handleKeydown(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      focusItem(activeIndex + 1 >= items.length ? 0 : activeIndex + 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      focusItem(activeIndex - 1 < 0 ? items.length - 1 : activeIndex - 1);
    } else if (e.key === 'Home') {
      e.preventDefault();
      focusItem(0);
    } else if (e.key === 'End') {
      e.preventDefault();
      focusItem(items.length - 1);
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      const item = items[activeIndex];
      if (item) choose(item);
    } else if (e.key === 'Escape') {
      e.preventDefault();
      close(true);
    } else if (e.key === 'Tab') {
      e.preventDefault();
      close(true);
    }
  }

  function handleWindowPointerDown(e: PointerEvent) {
    if (menu && !menu.contains(e.target as Node)) close(false);
  }

  function handleWindowDismiss() {
    close(false);
  }

  // Scrolling a long menu keeps it; scrolling what is under it closes it.
  function handleScroll(e: Event) {
    if (!menu?.contains(e.target as Node)) close(false);
  }
</script>

<svelte:window
  onpointerdown={handleWindowPointerDown}
  onresize={handleWindowDismiss}
  onblur={handleWindowDismiss}
/>
<svelte:document onscrollcapture={handleScroll} />

<div
  bind:this={menu}
  class="context-menu"
  role="menu"
  tabindex="-1"
  aria-label={ariaLabel}
  style:left="{left}px"
  style:top="{top}px"
  onkeydown={handleKeydown}
  oncontextmenu={(e) => e.preventDefault()}
>
  {#if heading}<div class="context-menu-heading" title={heading}>{heading}</div>{/if}
  {#each items as item, i (item.label)}
    {#if item.separator && i > 0}<div class="context-menu-separator" role="separator"></div>{/if}
    <button
      type="button"
      class="context-menu-item"
      role="menuitem"
      tabindex={i === activeIndex ? 0 : -1}
      aria-disabled={item.disabled ? 'true' : undefined}
      class:disabled={item.disabled}
      title={item.title}
      onpointerenter={() => focusItem(i)}
      onclick={() => choose(item)}
    >
      <span>{item.label}</span>
    </button>
  {/each}
</div>

<style>
  .context-menu {
    animation: -global-pop-in var(--dur-fast) var(--ease-out);
    position: fixed;
    z-index: 120;
    min-width: 160px;
    /* Explanations wrap instead of stretching the menu across the window. */
    max-width: min(420px, calc(100vw - 16px));
    /* A long list (40 definitions) scrolls inside the window. */
    max-height: calc(100vh - 8px);
    overflow-y: auto;
    overscroll-behavior: contain;
    background: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: var(--radius-sm, 6px);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.25);
    padding: 4px;
    display: flex;
    flex-direction: column;
    gap: 2px;
    outline: none;
  }

  .context-menu-heading {
    position: sticky;
    top: -4px;
    z-index: 1;
    flex-shrink: 0;
    margin: -4px -4px 2px;
    padding: 8px 14px 6px;
    background: var(--bg-primary);
    border-bottom: 1px solid var(--border-color);
    font-size: 0.6875rem;
    color: var(--text-muted);
    overflow-wrap: anywhere;
  }

  .context-menu-item {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    border: none;
    background: transparent;
    border-radius: var(--radius-sm, 4px);
    font-size: 0.75rem;
    font-weight: 500;
    color: var(--text-secondary);
    cursor: pointer;
    text-align: left;
    width: 100%;
    box-sizing: border-box;
    outline: none;
  }

  .context-menu-item:hover,
  .context-menu-item:focus-visible,
  .context-menu-item:focus {
    background: var(--bg-tertiary);
    color: var(--text-primary);
  }

  .context-menu-separator {
    height: 1px;
    margin: 2px 6px;
    background: var(--border-color);
  }

  .context-menu-item.disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
</style>

// Tab bars: sideways wheel scrolling and reordering tabs by dragging them.
// Dragging follows the pointer itself rather than HTML drag and drop, which
// the webviews implement differently and the app's file drops also use.

import { tick } from 'svelte';
import { dropIndex, gapAt } from './tabActions';

/** How far the pointer moves before a press on a tab becomes a drag. */
const THRESHOLD = 4;
/** The width at each end of the bar where a drag scrolls it. */
const EDGE = 32;
/** The fastest a drag scrolls the bar, in pixels a frame. */
const SPEED = 14;
/** How far above or below the bar a drop still counts. */
const SLACK = 32;

/**
 * Scrolls bar sideways for a vertical wheel, so no Shift is needed. A
 * horizontal scroll, such as a trackpad swipe, keeps its own behaviour.
 */
export function scrollSideways(bar: HTMLElement, e: WheelEvent): void {
  if (e.ctrlKey || Math.abs(e.deltaX) >= Math.abs(e.deltaY)) return;
  if (bar.scrollWidth <= bar.clientWidth) return;
  const unit = e.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : e.deltaMode === WheelEvent.DOM_DELTA_PAGE ? bar.clientWidth : 1;
  e.preventDefault();
  bar.scrollLeft += e.deltaY * unit;
}

/**
 * Runs move, then gives focus back to the element that had it: moving a
 * focused tab in the page drops its focus.
 */
export function keepingFocus(move: () => unknown): void {
  const focused = document.activeElement;
  move();
  void tick().then(() => {
    if (focused instanceof HTMLElement && focused.isConnected && document.activeElement !== focused) focused.focus();
  });
}

/** Scrolls bar the least that shows all of tab, one of its children. */
export function revealTab(bar: HTMLElement, tab: HTMLElement): void {
  const left = tab.offsetLeft;
  const right = left + tab.offsetWidth;
  if (left < bar.scrollLeft) bar.scrollLeft = left;
  else if (right > bar.scrollLeft + bar.clientWidth) bar.scrollLeft = right - bar.clientWidth;
}

/**
 * Reorders the tabs of a bar by dragging. Each tab carries its ID in
 * data-tab-id, calls press on pointerdown and is moved sideways by offset
 * while a drag runs; the bar is positioned so its tabs' offsets are in its
 * scrolled content. The dragged tab follows the pointer and the tabs it
 * passes make room for it.
 */
export class TabDrag {
  /** The dragged tab's ID, once the pointer has moved far enough. */
  id = $state<string | null>(null);

  #bar: () => HTMLElement | undefined;
  #move: (id: string, to: number) => void;
  #press: { id: string; x: number; y: number; scroll: number; pointerId: number; el: HTMLElement } | null = null;
  #pointer = { x: 0, y: 0 };
  #to: number | null = null;
  #frame = 0;
  /** The dragged tab's index, the gap it would drop into, its width and how far it has moved. */
  #shift = $state<{ from: number; gap: number; width: number; dx: number } | null>(null);

  constructor(bar: () => HTMLElement | undefined, move: (id: string, to: number) => void) {
    this.#bar = bar;
    this.#move = move;
  }

  /** How far the tab id, at index in the bar, is moved sideways. */
  offset(id: string, index: number): number {
    const shift = this.#shift;
    if (this.id === null || !shift) return 0;
    if (id === this.id) return shift.dx;
    if (index > shift.from && index < shift.gap) return -shift.width;
    if (index >= shift.gap && index < shift.from) return shift.width;
    return 0;
  }

  /** Watches a press on tab id; it becomes a drag once the pointer moves. */
  press = (e: PointerEvent, id: string): void => {
    // Ctrl+click is macOS's right click.
    if (e.button !== 0 || e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return;
    if ((e.target as Element | null)?.closest?.('.tab-close')) return;
    // A press whose release went unseen gives way.
    if (this.#press) this.#end();
    const scroll = this.#bar()?.scrollLeft ?? 0;
    this.#press = { id, x: e.clientX, y: e.clientY, scroll, pointerId: e.pointerId, el: e.currentTarget as HTMLElement };
    this.#pointer = { x: e.clientX, y: e.clientY };
    window.addEventListener('pointermove', this.#onMove, true);
    window.addEventListener('pointerup', this.#onUp, true);
    window.addEventListener('pointercancel', this.#end, true);
    window.addEventListener('keydown', this.#onKey, true);
    window.addEventListener('blur', this.#end);
  };

  #onMove = (e: PointerEvent): void => {
    const press = this.#press;
    if (!press || e.pointerId !== press.pointerId) return;
    this.#pointer = { x: e.clientX, y: e.clientY };
    if (this.id === null) {
      if (Math.hypot(e.clientX - press.x, e.clientY - press.y) < THRESHOLD) return;
      this.id = press.id;
      try {
        press.el.setPointerCapture(press.pointerId);
      } catch {
        // The press ended already.
      }
      document.documentElement.classList.add('tab-dragging');
      this.#frame = requestAnimationFrame(this.#tick);
    }
    this.#update();
  };

  // Scrolls the bar while the pointer is near either end of it.
  #tick = (): void => {
    const bar = this.#bar();
    if (!bar || this.id === null) return;
    const rect = bar.getBoundingClientRect();
    const { x, y } = this.#pointer;
    if (y >= rect.top - SLACK && y <= rect.bottom + SLACK) {
      const edge = Math.min(EDGE, rect.width / 4);
      if (x < rect.left + edge) bar.scrollLeft -= SPEED * Math.min(1, (rect.left + edge - x) / edge);
      else if (x > rect.right - edge) bar.scrollLeft += SPEED * Math.min(1, (x - rect.right + edge) / edge);
    }
    this.#update();
    this.#frame = requestAnimationFrame(this.#tick);
  };

  #update(): void {
    const bar = this.#bar();
    const press = this.#press;
    const tabs = bar ? [...bar.querySelectorAll<HTMLElement>('[data-tab-id]')] : [];
    const from = tabs.findIndex((t) => t.dataset.tabId === this.id);
    // The dragged tab closed meanwhile.
    if (!bar || !press || from < 0) return this.#end();
    const rect = bar.getBoundingClientRect();
    const { x, y } = this.#pointer;
    const dragged = tabs[from];
    const width = dragged.offsetWidth;
    // The tab stays inside the bar's content.
    const max = bar.scrollWidth - dragged.offsetLeft - width;
    const dx = Math.max(-dragged.offsetLeft, Math.min(max, x - press.x + bar.scrollLeft - press.scroll));
    // Layout places, which the offsets move only on screen. The tab drops
    // where its middle is.
    const origin = rect.left + bar.clientLeft - bar.scrollLeft;
    const places = tabs.map((t) => ({ left: origin + t.offsetLeft, width: t.offsetWidth }));
    const outside = y < rect.top - SLACK || y > rect.bottom + SLACK;
    const gap = outside ? from : gapAt(places[from].left + width / 2 + dx, places);
    this.#to = dropIndex(from, gap);
    this.#shift = { from, gap, width, dx };
  }

  #onUp = (e: PointerEvent): void => {
    if (!this.#press || e.pointerId !== this.#press.pointerId) return;
    const id = this.id;
    const to = this.#to;
    const shown = id === null ? null : this.#shown();
    if (id !== null) {
      // The click this release makes is the drag's end, not a selection.
      const swallow = (click: Event) => {
        click.preventDefault();
        click.stopPropagation();
      };
      window.addEventListener('click', swallow, { capture: true, once: true });
      setTimeout(() => window.removeEventListener('click', swallow, true), 0);
    }
    this.#end();
    if (id !== null && to !== null) this.#move(id, to);
    if (shown) this.#settle(shown);
  };

  // Escape puts the tab back, without closing what the bar is in.
  #onKey = (e: KeyboardEvent): void => {
    if (this.id === null || !this.#press || e.key !== 'Escape') return;
    e.preventDefault();
    e.stopPropagation();
    const shown = this.#shown();
    this.#end();
    this.#settle(shown);
  };

  // Where each tab is on screen.
  #shown(): Map<string, number> {
    const tabs = this.#bar()?.querySelectorAll<HTMLElement>('[data-tab-id]') ?? [];
    return new Map([...tabs].map((t) => [t.dataset.tabId ?? '', t.getBoundingClientRect().left]));
  }

  // Glides the tabs from where they were shown when the drag ended into
  // their places.
  #settle(shown: Map<string, number>): void {
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    void tick().then(() => {
      for (const tab of this.#bar()?.querySelectorAll<HTMLElement>('[data-tab-id]') ?? []) {
        const dx = (shown.get(tab.dataset.tabId ?? '') ?? NaN) - tab.getBoundingClientRect().left;
        if (Math.abs(dx) >= 1) tab.animate([{ transform: `translateX(${dx}px)` }, { transform: 'none' }], { duration: 150, easing: 'ease-out' });
      }
    });
  }

  #end = (): void => {
    const press = this.#press;
    if (press?.el.hasPointerCapture?.(press.pointerId)) press.el.releasePointerCapture(press.pointerId);
    window.removeEventListener('pointermove', this.#onMove, true);
    window.removeEventListener('pointerup', this.#onUp, true);
    window.removeEventListener('pointercancel', this.#end, true);
    window.removeEventListener('keydown', this.#onKey, true);
    window.removeEventListener('blur', this.#end);
    cancelAnimationFrame(this.#frame);
    document.documentElement.classList.remove('tab-dragging');
    this.#press = null;
    this.#to = null;
    this.id = null;
    this.#shift = null;
  };
}

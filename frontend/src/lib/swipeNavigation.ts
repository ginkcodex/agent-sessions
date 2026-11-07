// A two-finger touchpad swipe goes back or forward. The native side spots it
// and sends nav:swipe (cmd/agent-sessions/mousenav_darwin.m,
// mousenav_linux.c, mousenav_windows.go). The page's wheel events cannot
// tell: they carry no phase, and on Windows the renderer merges them and
// leaves gaps while the page is busy, so a swipe guessed from them was
// missed or went several steps (SN-09).
//
// The same gesture also reaches the page as wheel events, and may be
// scrolling something sideways: a long code line, a wide transcript block,
// the tab strip, a long text field. Then it is a scroll, not a swipe, as in
// Safari, and the swipe is dropped.

/** Wheel events closer together than this (ms) are one gesture; momentum keeps it going. */
export const SWIPE_GESTURE_GAP = 200;

/** The parts of a wheel event the guard reads. */
export interface SwipeWheel {
  deltaX: number;
  ctrlKey: boolean;
  timeStamp: number;
  target: EventTarget | null;
}

interface Scroller {
  tagName?: string;
  scrollLeft: number;
  scrollWidth: number;
  clientWidth: number;
  parentElement: Scroller | null;
}

const SCROLLING = new Set(['auto', 'scroll', 'overlay']);

// Browsers give text fields overflow: clip, yet they scroll through a long value.
const TEXT_FIELDS = new Set(['INPUT', 'TEXTAREA']);

/**
 * Whether target, or an element it is in, can still scroll sideways toward
 * sign (negative: left, positive: right). Only elements whose CSS lets them
 * scroll sideways count, text fields, and the document's scrolling element.
 */
export function scrollsSideways(target: EventTarget | null, sign: number, overflowX: (el: Element) => string = el => getComputedStyle(el).overflowX, page: Element | null = typeof document === 'undefined' ? null : document.scrollingElement): boolean {
  if (sign === 0) return false;
  for (let el = target as Scroller | null; el && typeof el.scrollWidth === 'number'; el = el.parentElement) {
    const room = el.scrollWidth - el.clientWidth;
    if (room <= 1) continue;
    if (el !== (page as unknown) && !TEXT_FIELDS.has(el.tagName ?? '') && !SCROLLING.has(overflowX(el as unknown as Element))) continue;
    if (sign < 0 ? el.scrollLeft > 0 : el.scrollLeft < room - 1) return true;
  }
  return false;
}

/** Tracks the wheel gesture in progress, and whether it scrolled something sideways. */
export class SwipeGuard {
  private last = -Infinity;
  private scrolled = false;

  private readonly scrolls: (target: EventTarget | null, sign: number) => boolean;

  constructor(scrolls: (target: EventTarget | null, sign: number) => boolean = (target, sign) => scrollsSideways(target, sign)) {
    this.scrolls = scrolls;
  }

  // A gesture ends only after a pause: momentum after the fingers lift
  // keeps it going.
  wheel(event: SwipeWheel): void {
    if (event.ctrlKey) return; // a pinch, not a scroll
    if (event.timeStamp - this.last > SWIPE_GESTURE_GAP) this.scrolled = false;
    this.last = event.timeStamp;
    if (!this.scrolled && event.deltaX !== 0 && this.scrolls(event.target, Math.sign(event.deltaX))) this.scrolled = true;
  }

  /** Whether a swipe at time now (ms, as event time stamps) should go back or forward. */
  allows(now: number): boolean {
    return !(this.scrolled && now - this.last <= SWIPE_GESTURE_GAP);
  }
}

let shared: SwipeGuard | null | undefined;

/** The window's guard, listening from first use; null where there is no window. */
export function windowSwipeGuard(): SwipeGuard | null {
  if (shared !== undefined) return shared;
  if (typeof window === 'undefined' || typeof window.addEventListener !== 'function') return (shared = null);
  const guard = new SwipeGuard();
  window.addEventListener('wheel', (event: WheelEvent) => {
    // Inside a shadow root the event's target is the host; the scroller may be inside.
    const target = event.composedPath?.()[0] ?? event.target;
    guard.wheel({ deltaX: event.deltaX, ctrlKey: event.ctrlKey, timeStamp: event.timeStamp, target });
  }, { capture: true, passive: true });
  return (shared = guard);
}

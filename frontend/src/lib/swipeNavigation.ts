// A two-finger touchpad swipe goes back or forward. On macOS and Linux the
// native side spots it (nav:swipe from cmd/agent-sessions/mousenav_darwin.m
// and mousenav_linux.c). On Windows the app never sees WebView2's scroll
// messages, so the guard spots it here from the wheel events.
//
// The same gesture also reaches the page as wheel events, and may be
// scrolling something sideways: a long code line, a wide transcript block,
// the tab strip, a long text field. Then it is a scroll, not a swipe, as in
// Safari, and the swipe is dropped.

/** Wheel events closer together than this (ms) are one gesture; momentum keeps it going. */
export const SWIPE_GESTURE_GAP = 200;
/** A new stroke needs the speed to fall to this share of its peak, then rise this many events in a row. */
export const STROKE_DECAY = 0.5;
export const STROKE_RISE_EVENTS = 2;
/** How far (px) a gesture must go sideways to swipe, as SWIPE_DISTANCE in mousenav_darwin.m. */
export const SWIPE_DISTANCE = 80;
/** A touchpad sends many small deltas; a gesture swipes only after this many events. */
export const SWIPE_MIN_EVENTS = 3;
/** A delta this large (px) is a mouse wheel's step, not a touchpad: that gesture never swipes. */
export const WHEEL_STEP = 100;

/** The parts of a wheel event the guard reads. */
export interface SwipeWheel {
  deltaX: number;
  deltaY?: number;
  /** WheelEvent.deltaMode: 0 pixels, 1 lines, 2 pages. */
  deltaMode?: number;
  shiftKey?: boolean;
  ctrlKey: boolean;
  timeStamp: number;
  target: EventTarget | null;
}

export type SwipeDirection = 'back' | 'forward';

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

/**
 * Tracks the wheel gesture in progress, and whether it scrolled something
 * sideways. With detect on (Windows), it also spots swipes in the gesture
 * and tells onSwipe's listeners.
 */
export class SwipeGuard {
  private last = -Infinity;
  private scrolled = false;
  // The stroke's speed (|deltaX| + |deltaY| per event): its peak, the lowest
  // since that peak, and how many events in a row rose from that low.
  private peak = 0;
  private low = 0;
  private rises = 0;
  // The detector's stroke: how far it went, in how many events, whether a
  // mouse wheel took part, and whether it already swiped.
  private x = 0;
  private y = 0;
  private events = 0;
  private wheeled = false;
  private swiped = false;
  private readonly listeners = new Set<(direction: SwipeDirection) => void>();

  private readonly scrolls: (target: EventTarget | null, sign: number) => boolean;
  private readonly detect: boolean;

  constructor(scrolls: (target: EventTarget | null, sign: number) => boolean = (target, sign) => scrollsSideways(target, sign), detect = false) {
    this.scrolls = scrolls;
    this.detect = detect;
  }

  // Wheel events come as one stream while the fingers move, then as
  // momentum after they lift; a quick next swipe joins the same stream. So a
  // stroke ends after a pause, or when the fingers come back: the speed fell
  // to half its peak or less, then rose again. Momentum only slows down.
  wheel(event: SwipeWheel): void {
    if (event.ctrlKey) return; // a pinch, not a scroll
    const speed = Math.abs(event.deltaX) + Math.abs(event.deltaY ?? 0);
    if (event.timeStamp - this.last > SWIPE_GESTURE_GAP || this.risesAgain(speed)) this.startStroke(speed);
    this.last = event.timeStamp;
    if (!this.scrolled && event.deltaX !== 0 && this.scrolls(event.target, Math.sign(event.deltaX))) this.scrolled = true;
    if (this.detect) this.track(event);
  }

  private startStroke(speed: number): void {
    this.scrolled = false;
    this.peak = this.low = speed;
    this.rises = 0;
    this.x = this.y = this.events = 0;
    this.wheeled = this.swiped = false;
  }

  private risesAgain(speed: number): boolean {
    if (speed === 0) return false;
    // While it rises, the low it rose from stays the reference.
    if (this.low <= this.peak * STROKE_DECAY && speed >= this.low * 1.5 + 3) return ++this.rises >= STROKE_RISE_EVENTS;
    this.rises = 0;
    if (speed > this.peak) this.peak = this.low = speed;
    else if (speed < this.low) this.low = speed;
    return false;
  }

  // A swipe follows the scroll, as in Chrome and Edge: toward the left is back.
  private track(event: SwipeWheel): void {
    if (this.swiped || this.wheeled) return;
    const dx = event.deltaX;
    const dy = event.deltaY ?? 0;
    // Shift+wheel, line or page deltas and whole steps come from a mouse.
    if (event.shiftKey || (event.deltaMode ?? 0) !== 0 || Math.abs(dx) >= WHEEL_STEP || Math.abs(dy) >= WHEEL_STEP) {
      this.wheeled = true;
      return;
    }
    this.x += dx;
    this.y += dy;
    this.events++;
    if (this.events < SWIPE_MIN_EVENTS || Math.abs(this.x) < SWIPE_DISTANCE || Math.abs(this.x) <= 2 * Math.abs(this.y)) return;
    this.swiped = true;
    if (this.scrolled) return;
    const direction: SwipeDirection = this.x < 0 ? 'back' : 'forward';
    for (const fn of [...this.listeners]) fn(direction);
  }

  /** Calls fn for each swipe the guard spots itself (detect on); returns the unsubscribe. */
  onSwipe(fn: (direction: SwipeDirection) => void): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
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
  // Only Windows has no native swipe (see the top of this file).
  const guard = new SwipeGuard(undefined, typeof navigator !== 'undefined' && /^Win/.test(navigator.platform));
  window.addEventListener('wheel', (event: WheelEvent) => {
    // Inside a shadow root the event's target is the host; the scroller may be inside.
    const target = event.composedPath?.()[0] ?? event.target;
    guard.wheel({
      deltaX: event.deltaX, deltaY: event.deltaY, deltaMode: event.deltaMode,
      shiftKey: event.shiftKey, ctrlKey: event.ctrlKey, timeStamp: event.timeStamp, target
    });
  }, { capture: true, passive: true });
  return (shared = guard);
}

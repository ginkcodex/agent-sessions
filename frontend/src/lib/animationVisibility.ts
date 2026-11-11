// Work indicators move in a few discrete steps per second, and only while on
// screen. A CSS opacity or transform animation would be run by Core Animation
// outside the page, so macOS recomposites the whole window at the display
// rate (up to 120 Hz on ProMotion). An active agent terminal coincided with
// about 70% WindowServer GPU time (IR-05); these animations are suspects,
// not isolated causes. Discrete steps avoid continuous interpolation.
// Status text never depends on it, so
// an indicator without IntersectionObserver simply stays still.

type Tick = (tick: number) => void;

/** One timer per period, shared by every subscriber and stopped when unused. */
export function createTicker(periodMs: number) {
  const listeners = new Set<Tick>();
  let timer: ReturnType<typeof setInterval> | undefined;
  let tick = 0;
  return {
    subscribe(listener: Tick): () => void {
      listeners.add(listener);
      listener(tick);
      timer ??= setInterval(() => {
        tick++;
        for (const each of [...listeners]) each(tick);
      }, periodMs);
      return () => {
        listeners.delete(listener);
        if (listeners.size || timer === undefined) return;
        clearInterval(timer);
        timer = undefined;
      };
    },
    get listeners() {
      return listeners.size;
    },
  };
}

let reducedMotionQuery: MediaQueryList | undefined;
function reducedMotion(): boolean {
  if (typeof matchMedia !== 'function') return false;
  reducedMotionQuery ??= matchMedia('(prefers-reduced-motion: reduce)');
  return reducedMotionQuery.matches;
}

/** Eight 45° steps per turn: one turn a second, or two with reduced motion. */
export const SPIN_STEPS = 8;
export const spinTicker = createTicker(1000 / SPIN_STEPS);
/** Working dots dim and brighten every 0.8 s, as the old 1.6 s pulse did. */
export const blinkTicker = createTicker(800);

type Styled = Element & ElementCSSInlineStyle;

function spin(node: Element): () => void {
  const el = node as Styled;
  let angle = -1;
  const stop = spinTicker.subscribe((tick) => {
    // Reduced motion keeps the turn, at half speed, since an icon-only
    // spinner can be the only sign that work is going on.
    const step = (reducedMotion() ? Math.floor(tick / 2) : tick) % SPIN_STEPS;
    const next = step * (360 / SPIN_STEPS);
    if (next === angle) return;
    angle = next;
    el.style.transform = `rotate(${next}deg)`;
  });
  return () => {
    stop();
    el.style.removeProperty('transform');
  };
}

function blink(node: Element): () => void {
  const stop = blinkTicker.subscribe((tick) => {
    // Reduced motion: a steady dot, as the CSS pulse was switched off before.
    const dim = tick % 2 === 1 && !reducedMotion();
    if (dim !== node.hasAttribute('data-dim')) node.toggleAttribute('data-dim', dim);
  });
  return () => {
    stop();
    node.removeAttribute('data-dim');
  };
}

function whileVisible(node: Element, enabled: boolean, start: (node: Element) => () => void) {
  let observer: IntersectionObserver | undefined;
  let generation = 0;
  let stop: (() => void) | undefined;

  function halt() {
    stop?.();
    stop = undefined;
    node.removeAttribute('data-animation-visible');
  }

  function update(active: boolean) {
    const current = ++generation;
    observer?.disconnect();
    observer = undefined;
    halt();
    if (!active || typeof IntersectionObserver === 'undefined') return;
    observer = new IntersectionObserver((entries) => {
      // A callback already queued before disconnect must not reactivate a
      // formerly animated indicator after its props changed or it unmounted.
      if (current !== generation) return;
      for (const entry of entries) {
        if (entry.target !== node) continue;
        if (entry.isIntersecting && !stop) {
          node.setAttribute('data-animation-visible', '');
          stop = start(node);
        } else if (!entry.isIntersecting && stop) {
          halt();
        }
      }
    });
    observer.observe(node);
  }

  update(enabled);
  return {
    update,
    destroy() {
      ++generation;
      observer?.disconnect();
      observer = undefined;
      halt();
    },
  };
}

/** Svelte action: turn a spinner icon in discrete steps while it is on screen. */
export function animateWhenVisible(node: Element, enabled: boolean) {
  return whileVisible(node, enabled, spin);
}

/** Svelte action: blink a working dot in discrete steps while it is on screen. */
export function blinkWhenVisible(node: Element, enabled: boolean) {
  return whileVisible(node, enabled, blink);
}

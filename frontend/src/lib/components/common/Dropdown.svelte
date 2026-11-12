<script lang="ts">
  import Icon from './Icon.svelte';
  // Themed replacement for <select>. WebKitGTK draws native select popups
  // with the GTK theme, which ignores the app's colors and fonts.
  interface Option {
    value: string;
    label: string;
    /** Shown on hover, e.g. the whole of a label cut short. */
    title?: string;
    /** Can't be chosen; shown muted. */
    disabled?: boolean;
    /** Options sharing a group follow one another under its heading. */
    group?: string;
  }

  interface Props {
    options: Option[];
    value: string;
    onChange: (value: string) => void;
    ariaLabel: string;
    title?: string;
    align?: 'left' | 'right';
    /** Which side of the trigger the menu opens on; 'top' at the window's bottom edge. */
    placement?: 'bottom' | 'top';
    disabled?: boolean;
    /** Shown while no option is chosen. */
    placeholder?: string;
    /** Runs on the trigger before its own handling; preventDefault skips that. */
    onkeydown?: (e: KeyboardEvent) => void;
    oncontextmenu?: (e: MouseEvent) => void;
  }

  let { options, value, onChange, ariaLabel, title, align = 'left', placement = 'bottom', disabled = false, placeholder = '', onkeydown, oncontextmenu }: Props = $props();

  let open = $state(false);
  let activeIndex = $state(0);
  let root: HTMLDivElement | undefined = $state();
  let trigger: HTMLButtonElement | undefined = $state();
  let list: HTMLUListElement | undefined = $state();

  const id = `dropdown-${Math.random().toString(36).slice(2, 9)}`;
  let selected = $derived(options.find((o) => o.value === value));

  // A disabled dropdown closes.
  $effect(() => {
    if (disabled && open) hide(false);
  });

  // Long lists scroll; the active option stays in view.
  $effect(() => {
    if (open) list?.querySelector(`#${id}-${activeIndex}`)?.scrollIntoView({ block: 'nearest' });
  });

  // The next enabled option from `from` in `step` direction, or `from`.
  function enabledFrom(from: number, step: 1 | -1): number {
    for (let i = from; i >= 0 && i < options.length; i += step) if (!options[i].disabled) return i;
    return -1;
  }

  function move(from: number, step: 1 | -1) {
    const next = enabledFrom(from, step);
    if (next !== -1) activeIndex = next;
  }

  function show() {
    if (disabled || enabledFrom(0, 1) === -1) return;
    const current = options.findIndex((o) => o.value === value && !o.disabled);
    activeIndex = current === -1 ? enabledFrom(0, 1) : current;
    typed = '';
    open = true;
    queueMicrotask(() => list?.focus());
  }

  function hide(refocus = true) {
    open = false;
    if (refocus) trigger?.focus();
  }

  function choose(index: number) {
    const opt = options[index];
    if (!opt || opt.disabled) return;
    hide();
    if (opt && opt.value !== value) onChange(opt.value);
  }

  // Typing jumps to the next option whose label starts with what was typed;
  // pressing one letter again steps through the options starting with it.
  let typed = '';
  let typedAt = 0;
  function typeAhead(key: string): boolean {
    const now = Date.now();
    if (now - typedAt > 700) typed = '';
    // Space chooses, unless it is part of a word being typed.
    if (key === ' ' && !typed) return false;
    typed += key.toLowerCase();
    typedAt = now;
    const repeat = [...typed].every((c) => c === typed[0]);
    const prefix = repeat ? typed[0] : typed;
    const start = repeat ? activeIndex + 1 : activeIndex;
    for (let n = 0; n < options.length; n++) {
      const i = (start + n) % options.length;
      if (!options[i].disabled && options[i].label.toLowerCase().startsWith(prefix)) {
        activeIndex = i;
        break;
      }
    }
    return true;
  }

  function handleTriggerKeydown(e: KeyboardEvent) {
    onkeydown?.(e);
    if (e.defaultPrevented) return;
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      e.stopPropagation();
      show();
    }
  }

  // Stop propagation so parent keyboard navigation (e.g. the session list's
  // arrow keys) does not also react while the menu is open.
  function handleListKeydown(e: KeyboardEvent) {
    e.stopPropagation();
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      move(activeIndex + 1, 1);
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      move(activeIndex - 1, -1);
    } else if (e.key === 'Home') {
      e.preventDefault();
      move(0, 1);
    } else if (e.key === 'End') {
      e.preventDefault();
      move(options.length - 1, -1);
    } else if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey && typeAhead(e.key)) {
      e.preventDefault();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      choose(activeIndex);
    } else if (e.key === 'Escape' || e.key === 'Tab') {
      if (e.key === 'Escape') e.preventDefault();
      hide(e.key === 'Escape');
    }
  }

  function handleWindowPointerDown(e: PointerEvent) {
    if (open && root && !root.contains(e.target as Node)) hide(false);
  }
</script>

<svelte:window onpointerdown={handleWindowPointerDown} />

<div class="dropdown" bind:this={root}>
  <button
    bind:this={trigger}
    type="button"
    class="trigger"
    class:open
    aria-haspopup="listbox"
    aria-expanded={open}
    aria-label={ariaLabel}
    title={title ?? selected?.title}
    {disabled}
    onclick={() => (open ? hide() : show())}
    onkeydown={handleTriggerKeydown}
    {oncontextmenu}
  >
    <span class="trigger-label" class:placeholder={!selected}>{selected?.label ?? placeholder}</span>
    <Icon name="chevron-down" size={12} strokeWidth={2.25} class="caret" />
  </button>

  {#if open}
    <ul
      bind:this={list}
      class="menu"
      class:right={align === 'right'}
      class:up={placement === 'top'}
      role="listbox"
      tabindex="-1"
      aria-label={ariaLabel}
      aria-activedescendant="{id}-{activeIndex}"
      onkeydown={handleListKeydown}
    >
      {#each options as opt, i (opt.value)}
        {#if opt.group && opt.group !== options[i - 1]?.group}
          <li class="group" role="presentation">{opt.group}</li>
        {/if}
        <!-- Keyboard selection is handled on the listbox (aria-activedescendant). -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li
          id="{id}-{i}"
          role="option"
          aria-selected={opt.value === value}
          aria-disabled={opt.disabled || undefined}
          class:active={i === activeIndex}
          class:selected={opt.value === value}
          class:disabled={opt.disabled}
          title={opt.title}
          onpointerenter={() => {
            if (!opt.disabled) activeIndex = i;
          }}
          onclick={() => choose(i)}
        >
          <span class="check" aria-hidden="true">{opt.value === value ? '✓' : ''}</span>
          <span class="option-label">{opt.label}</span>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .dropdown {
    position: relative;
    display: inline-flex;
    min-width: 0;
    max-width: 100%;
  }

  .trigger {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    max-width: 100%;
    height: var(--control-height, 24px);
    padding: 0 8px;
    border-radius: var(--radius-sm);
    font-size: 0.72rem;
    font-weight: 500;
    white-space: nowrap;
    background-color: var(--bg-secondary);
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .trigger-label,
  .option-label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .trigger-label.placeholder {
    color: var(--text-muted);
  }

  .trigger:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .trigger:hover:not(:disabled),
  .trigger.open,
  .trigger:focus-visible {
    color: var(--text-primary);
    background-color: var(--hover-bg);
    outline: none;
  }

  .trigger :global(.caret) {
    flex-shrink: 0;
    margin-right: -2px;
    opacity: 0.6;
    transition: transform 0.15s ease;
  }

  .trigger.open :global(.caret) {
    transform: rotate(180deg);
  }

  .menu {
    animation: -global-pop-in var(--dur-fast) var(--ease-out);
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 50;
    min-width: 100%;
    max-width: min(480px, calc(100vw - 32px));
    max-height: min(320px, 60vh);
    overflow-y: auto;
    margin: 0;
    padding: 4px;
    list-style: none;
    background-color: var(--bg-primary);
    border: 1px solid var(--border-color);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-popover);
    outline: none;
  }

  .menu.right {
    left: auto;
    right: 0;
  }

  /* Above the trigger, for a control sitting on the window's bottom edge. */
  .menu.up {
    top: auto;
    bottom: calc(100% + 4px);
  }

  .menu li {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 5px 10px 5px 6px;
    border-radius: var(--radius-sm);
    font-size: 0.75rem;
    white-space: nowrap;
    color: var(--text-primary);
    cursor: pointer;
  }

  .menu li.group {
    padding: 8px 10px 3px 22px;
    font-size: 0.68rem;
    font-weight: 600;
    color: var(--text-muted);
    cursor: default;
  }

  .menu li.group:first-child {
    padding-top: 3px;
  }

  .menu li.disabled {
    color: var(--text-muted);
    cursor: default;
  }

  .menu li.active {
    background-color: var(--hover-bg);
  }

  .menu li.selected {
    color: var(--accent-color);
    font-weight: 600;
  }

  .check {
    flex-shrink: 0;
    width: 12px;
    font-size: 0.7rem;
    text-align: center;
  }
</style>

<script lang="ts">
  // Where an agent tab stands: working blinks in the accent colour, needing
  // the user is amber, finished is green. Other states show nothing.
  import type { AgentAttention } from '../../attention';
  import { attentionLabel } from '../../alerts';
  import { blinkWhenVisible } from '../../animationVisibility';

  interface Props {
    state: AgentAttention | null | undefined;
    size?: number;
    /** Names the state for screen readers too, where nothing else says it. */
    announce?: boolean;
  }

  let { state, size = 7, announce = false }: Props = $props();

  let label = $derived(attentionLabel(state));
</script>

{#if label}
  <span
    class="attention-dot {state}"
    use:blinkWhenVisible={state === 'working'}
    style="--dot-size: {size}px"
    title={label}
    role={announce ? 'img' : undefined}
    aria-label={announce ? label : undefined}
    aria-hidden={announce ? undefined : 'true'}
  ></span>
{/if}

<style>
  .attention-dot {
    flex-shrink: 0;
    width: var(--dot-size);
    height: var(--dot-size);
    border-radius: 50%;
  }

  /* Working blinks in discrete steps (blinkWhenVisible), not with a CSS
     pulse: an endless opacity animation keeps macOS recompositing the whole
     window at the display rate. No transition, for the same reason. */
  .working {
    background: var(--accent-color);
  }

  .working:global([data-dim]) {
    opacity: 0.45;
  }

  .needs {
    background: var(--status-needs);
    box-shadow: 0 0 0 2px color-mix(in srgb, var(--status-needs) 30%, transparent);
  }

  .done {
    background: var(--status-live);
  }
</style>

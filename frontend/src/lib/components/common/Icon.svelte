<script lang="ts">
  // One line-icon set for the whole app, in place of emoji: emoji draw in
  // each OS's own colors and styles. Icons take currentColor and sit on the
  // text baseline like a glyph.
  import { icons, type IconName } from '../../icons';

  interface Props {
    name: IconName;
    size?: number;
    strokeWidth?: number;
    /** Announced name; without it the icon is decorative. */
    label?: string;
    class?: string;
  }

  let { name, size = 14, strokeWidth = 2, label, class: className = '' }: Props = $props();
</script>

<svg
  class="icon {className}"
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width={strokeWidth}
  stroke-linecap="round"
  stroke-linejoin="round"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
>{@html icons[name]}</svg>

<style>
  .icon {
    display: inline-block;
    flex-shrink: 0;
    vertical-align: -0.125em;
  }
</style>

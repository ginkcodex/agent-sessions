<script lang="ts">
  import { FONT_SIZES } from '../../appearance';
  import { preferences } from '../../stores/preferences.svelte';
  import MonoFontSetting from './MonoFontSetting.svelte';
  import SizeStepper from './SizeStepper.svelte';
</script>

<MonoFontSetting
  heading="Built-in terminal font"
  ariaLabel="Built-in terminal font"
  value={preferences.terminalFont}
  onChange={(font) => preferences.setTerminalFont(font)}
  hint="Choose a monospace font installed on this computer, including Nerd Fonts. No additional fonts are bundled or downloaded. Applies immediately to all built-in terminals."
  sampleLabel="Terminal font preview"
  sampleLines={['user@host ~/project ❯ git status', '0123456789 ─│┌┐└┘ \uE0A0 \uE0B0 \uF115']}
  sampleSize="{preferences.terminalFontSize ?? FONT_SIZES.terminal.default}px"
/>
<div class="size-setting">
  <h4>Built-in terminal text size</h4>
  <SizeStepper kind="terminal" label="Terminal text size" value={preferences.terminalFontSize} onChange={(size) => preferences.setFontSize('terminal', size)} />
</div>

<style>
  .size-setting { margin-top: 16px; }
  h4 { margin: 0 0 8px; font-size: 0.85rem; }
</style>

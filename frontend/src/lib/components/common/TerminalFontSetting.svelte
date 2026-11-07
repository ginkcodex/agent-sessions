<script lang="ts">
  import { preferences } from '../../stores/preferences.svelte';
  import MonoFontSetting from './MonoFontSetting.svelte';
</script>

<MonoFontSetting
  heading="Built-in terminal font"
  ariaLabel="Built-in terminal font"
  value={preferences.terminalFont}
  onChange={(font) => preferences.setTerminalFont(font)}
  hint="Choose a monospace font installed on this computer, including Nerd Fonts. No additional fonts are bundled or downloaded. Applies immediately to all built-in terminals."
  sampleLabel="Terminal font preview"
  sampleLines={['user@host ~/project ❯ git status', '0123456789 ─│┌┐└┘ \uE0A0 \uE0B0 \uF115']}
/>

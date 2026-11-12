<script lang="ts">
  // Settings → Appearance: the editor font and the body text sizes of the
  // editor, the project docs preview and session content.
  import { sessionFontVar, sessionMonoFontVar } from '../../appearance';
  import { preferences } from '../../stores/preferences.svelte';
  import MonoFontSetting from './MonoFontSetting.svelte';
  import SizeStepper from './SizeStepper.svelte';
  import TypeSample from './TypeSample.svelte';
</script>

<section class="section">
  <h3>Appearance</h3>
  <p class="hint">
    Sizes are for body text; headings, code and labels keep their ratio to it. These settings are for this computer
    and apply at once to open files and sessions.
  </p>
</section>

<section class="section">
  <h3>Editor</h3>
  <p class="hint">The editor and viewer in Go to File, including diffs and file history.</p>
  <h4>Font size</h4>
  <SizeStepper kind="editor" label="Editor font size" value={preferences.editorFontSize} onChange={(size) => preferences.setFontSize('editor', size)} />
  <MonoFontSetting
    heading="Font"
    ariaLabel="Editor font"
    value={preferences.editorFont}
    onChange={(font) => preferences.setEditorFont(font)}
    hint="Choose a monospace font installed on this computer. No additional fonts are bundled or downloaded."
    sampleLabel="Editor font preview"
    sampleLines={['func main() { fmt.Println("0O 1lI") }', '// → ≠ ≤ ≥ {} [] () ;']}
    sampleSize="var(--editor-font-size, 0.78rem)"
  />
</section>

<section class="section">
  <h3>Project docs preview</h3>
  <p class="hint">Markdown files shown as a preview in Go to File.</p>
  <h4>Text size</h4>
  <SizeStepper kind="docs" label="Project docs text size" value={preferences.docsFontSize} onChange={(size) => preferences.setFontSize('docs', size)} />
  <TypeSample body="var(--docs-font-size, 14.5px)" label="Project docs text preview" />
</section>

<section class="section">
  <h3>Session content</h3>
  <p class="hint">Messages, tool calls and the other parts of a session.</p>
  <h4>Text size</h4>
  <SizeStepper kind="session" label="Session text size" value={preferences.sessionFontSize} onChange={(size) => preferences.setFontSize('session', size)} />
  <label class="setting">
    <input
      type="checkbox"
      checked={preferences.sessionUsesEditorFont}
      onchange={(e) => preferences.setSessionUsesEditorFont((e.currentTarget as HTMLInputElement).checked)}
    />
    Use the editor font for monospace text in session content
  </label>
  <p class="hint">
    Every part of a session that uses a monospace font (code blocks, inline code, tool calls and output, patches and
    system messages) uses the editor font chosen above, so files and sessions share one font. Without an editor font
    chosen, the app default is used.
  </p>
  <TypeSample
    body={sessionFontVar(preferences.sessionFontSize) ?? '14.5px'}
    monoFont={sessionMonoFontVar(preferences.sessionUsesEditorFont, preferences.editorFont)}
    label="Session text preview"
  />
</section>

<style>
  .section + .section { margin-top: 20px; padding-top: 18px; border-top: 1px solid var(--border-color); }
  h3 { margin: 0 0 8px; font-size: 14px; font-weight: 600; }
  h4 { margin: 14px 0 8px; font-size: 0.85rem; }
  .setting { display: flex; align-items: center; gap: 8px; margin: 14px 0 6px; font-size: 0.85rem; font-weight: 600; }
  .setting input { accent-color: var(--accent-color); }
  .hint { margin: 0 0 10px; font-size: 0.75rem; line-height: 1.5; color: var(--text-muted); }
  .section > :global(.type-sample) { margin-top: 12px; }
</style>

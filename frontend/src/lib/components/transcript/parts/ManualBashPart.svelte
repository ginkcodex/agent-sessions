<script lang="ts">
  import type { ManualBash } from '../../../manualBash';
  import { bashPreview } from '../../../manualBash';
  import { copyToClipboard } from '../../../portable';
  import Icon from '../../common/Icon.svelte';

  let { record, defaultExpanded = false }: { record: ManualBash; defaultExpanded?: boolean } = $props();
  let expanded = $state(false);
  let copied = $state('');
  let copyError = $state('');
  let streams = $derived(record.kind === 'input'
    ? [{ label: 'Command', text: record.command }]
    : [{ label: 'stdout', text: record.stdout }, { label: 'stderr', text: record.stderr }].filter(stream => stream.text));
  let showFull = $derived(defaultExpanded || expanded);
  let clipped = $derived(streams.some(stream => bashPreview(stream.text).clipped));

  async function copy(text: string, label: string) {
    copyError = '';
    try {
      if (!(await copyToClipboard(text))) throw new Error('Clipboard unavailable');
      copied = label;
    } catch {
      copyError = 'Could not copy. Select the text and copy it manually.';
    }
  }
</script>

<section class="bash-card" aria-label={record.kind === 'input' ? 'Manually run Bash command' : 'Bash output'}>
  <header>
    <span class="heading"><Icon name="square-terminal" size={14} /><strong>Bash</strong><span class="subtitle">{record.kind === 'input' ? 'Run by you' : 'Output'}</span></span>
    {#if record.kind === 'input'}
      <button type="button" aria-label="Copy command" onclick={() => copy(record.command, 'Command')}>{copied === 'Command' ? 'Copied' : 'Copy'}</button>
    {/if}
  </header>
  {#each streams as stream (stream.label)}
    <div class="stream">
      {#if record.kind === 'output'}
        <div class="stream-header"><span>{stream.label}</span><button type="button" aria-label={`Copy ${stream.label}`} onclick={() => copy(stream.text, stream.label)}>{copied === stream.label ? 'Copied' : 'Copy'}</button></div>
      {/if}
      <div class="code-row">
        {#if record.kind === 'input'}<span class="prompt" aria-hidden="true">$</span>{/if}
        <!-- Scrollable text must be keyboard-focusable for arrow-key scrolling. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <pre tabindex="0" role="region" aria-label={stream.label}><code>{showFull ? stream.text : bashPreview(stream.text).text}</code></pre>
      </div>
    </div>
  {/each}
  {#if streams.length === 0}<p class="empty">No output</p>{/if}
  {#if clipped && !defaultExpanded}
    <button type="button" class="disclosure" aria-expanded={expanded} onclick={() => expanded = !expanded}>{expanded ? 'Show less' : 'Show full text'}</button>
  {/if}
  {#if copyError}<p class="copy-error" role="alert">{copyError}</p>{/if}
</section>

<style>
  .bash-card { width: 100%; overflow: hidden; border: 1px solid var(--border-color); border-radius: var(--radius-md); background: var(--bg-secondary); }
  header, .stream-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  header { min-height: 36px; padding: 6px 12px; border-bottom: 1px solid var(--border-color); }
  .heading { display: flex; align-items: center; gap: 8px; color: var(--text-secondary); font-size: calc(var(--session-font-size, 14.5px) * var(--type-small)); }
  strong { font-weight: 600; }
  .subtitle { color: var(--text-muted); font-weight: 400; }
  button { background: transparent; color: var(--text-muted); border: 0; border-radius: var(--radius-sm); padding: 3px 6px; font-size: calc(var(--session-font-size, 14.5px) * var(--type-label)); cursor: pointer; }
  button:hover { color: var(--text-primary); background: var(--hover-bg); }
  button:focus-visible, pre:focus-visible { outline: 2px solid var(--accent-color); outline-offset: -2px; }
  .stream + .stream { border-top: 1px solid var(--border-color); }
  .stream-header { padding: 6px 12px 0; color: var(--text-muted); font-family: var(--font-mono); font-size: calc(var(--session-font-size, 14.5px) * var(--type-label)); }
  .code-row { display: flex; align-items: flex-start; gap: 10px; padding: 10px 12px; }
  .prompt, pre { font-family: var(--font-mono); font-size: calc(var(--session-font-size, 14.5px) * var(--type-code)); line-height: 1.55; }
  .prompt { color: var(--text-muted); user-select: none; }
  pre { flex: 1; min-width: 0; max-height: 350px; overflow: auto; white-space: pre; color: var(--text-primary); margin: 0; }
  code { font: inherit; }
  .empty, .copy-error { padding: 10px 12px; color: var(--text-muted); font-size: calc(var(--session-font-size, 14.5px) * var(--type-small)); }
  .copy-error { color: var(--danger); }
  .disclosure { display: block; width: 100%; padding: 8px 12px; text-align: left; border-top: 1px solid var(--border-color); }
</style>

<script lang="ts">
  // A few kinds of text at one body size, sized with the type scale tokens
  // the docs preview and session content use, so a size change in Settings
  // shows them scaling together.
  interface Props {
    /** The body size, a CSS length. */
    body: string;
    label: string;
  }

  let { body, label }: Props = $props();
</script>

<div class="type-sample" style:--sample-body={body} role="img" aria-label={label}>
  <div class="heading">Release notes</div>
  <p>Body text with <code>inline code</code> and a second sentence.</p>
  <pre><code>const size = body * ratio;
return size;</code></pre>
  <div class="label">Label · 12:04</div>
</div>

<style>
  .type-sample {
    padding: 12px 14px;
    border-radius: var(--radius-md);
    background: var(--bg-secondary);
    color: var(--text-primary);
    font-size: calc(var(--sample-body) * var(--type-body));
    line-height: 1.6;
    overflow-x: auto;
  }

  .heading {
    font-size: calc(var(--sample-body) * var(--type-h2));
    font-weight: 600;
    line-height: 1.3;
    margin-bottom: 4px;
  }

  p {
    margin: 0 0 8px;
  }

  p code {
    font-family: var(--font-mono);
    font-size: calc(var(--type-inline-code) * 1em);
    padding: 1px 4px;
    border-radius: 4px;
    background: var(--bg-tertiary);
  }

  pre {
    margin: 0 0 8px;
    padding: 8px 10px;
    border-radius: var(--radius-sm);
    background: var(--bg-primary);
    font-family: var(--font-mono);
    font-size: calc(var(--sample-body) * var(--type-code));
    line-height: 1.45;
    white-space: pre;
  }

  pre code {
    font: inherit;
  }

  .label {
    font-size: calc(var(--sample-body) * var(--type-label));
    color: var(--text-muted);
  }
</style>

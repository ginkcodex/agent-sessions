<script lang="ts">
  interface Props {
    text: string;
  }

  let { text }: Props = $props();
</script>

<div class="notice-pill">
  <span class="notice-icon">ℹ️</span>
  <span class="notice-text">{text}</span>
</div>

<style>
  .notice-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    margin: 4px 0;
    border-radius: 4px;
    background-color: var(--bg-tertiary);
    border: 1px solid var(--border-color);
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-label));
    color: var(--text-muted);
  }

  .notice-icon {
    font-size: calc(var(--session-font-size, 14.5px) * var(--type-small));
    opacity: 0.7;
  }

  .notice-text {
    font-family: var(--font-mono);
  }
</style>

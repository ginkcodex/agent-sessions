<script lang="ts">
  import { APP_DESCRIPTION, APP_NAME, BUY_ME_A_COFFEE_URL } from '../../about';
  import Icon from './Icon.svelte';

  interface Props {
    version: string;
    /** The app icon as this platform shows it. */
    icon: string;
    /** Opens a link in the default browser. */
    openURL: (url: string) => void;
  }

  let { version, icon, openURL }: Props = $props();
</script>

<main class="about">
  <img class="app-icon" src={icon} alt="" width="96" height="96" draggable="false" />
  <h1>{APP_NAME}</h1>
  {#if version}
    <p class="version">{version}</p>
  {/if}
  <p class="description">{APP_DESCRIPTION}</p>
  <button type="button" class="coffee" title={BUY_ME_A_COFFEE_URL} onclick={() => openURL(BUY_ME_A_COFFEE_URL)}>
    <Icon name="coffee" size={20} strokeWidth={2.25} />
    Buy me a coffee
  </button>
</main>

<style>
  .about {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 0;
    min-height: 100vh;
    box-sizing: border-box;
    padding: 20px 24px 28px;
    text-align: center;
    user-select: none;
    -webkit-user-select: none;
  }

  .app-icon {
    width: 96px;
    height: 96px;
    margin-bottom: 14px;
  }

  h1 {
    margin: 0 0 8px;
    font-size: 20px;
    font-weight: 700;
    color: var(--text-primary);
  }

  .version,
  .description {
    margin: 0 0 8px;
    font-size: 13px;
    color: var(--text-muted);
  }

  .version {
    font-variant-numeric: tabular-nums;
  }

  .description {
    margin-bottom: 20px;
  }

  /* Buy Me a Coffee's own yellow, with dark text in both themes. */
  .coffee {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    padding: 10px 26px;
    border: none;
    border-radius: 999px;
    background: #ffdd00;
    color: #0d0c22;
    font: inherit;
    font-size: 15px;
    font-weight: 600;
    cursor: pointer;
    transition: background-color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out);
  }

  .coffee:hover {
    background: #ffe433;
  }

  .coffee:active {
    transform: scale(0.98);
  }

  .coffee:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 3px;
  }
</style>

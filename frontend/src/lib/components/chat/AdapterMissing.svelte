<script lang="ts">
  // An agent's ACP adapter was not found: why, and how to get it on the host
  // the chat would run on. Where the host can install it for the app, Install
  // shows what would be downloaded and installs only once the user approves;
  // the command to copy and run stays either way.
  import type { ChatAgent } from '../../types';
  import { copyToClipboard } from '../../portable';
  import { ADAPTER_TOOLS } from '../../managedTools';
  import { toast } from '../../stores/toast.svelte';
  import { connectionStore } from '../../stores/connection.svelte';
  import { adapterInstall as installer } from '../../stores/adapterInstall.svelte';
  import ManagedInstallReview from '../common/ManagedInstallReview.svelte';
  import ManagedInstallProgress from '../common/ManagedInstallProgress.svelte';

  interface Props {
    agent: ChatAgent;
    /** Where the adapter is missing: "this computer" or the host's name. */
    host: string;
  }

  let { agent, host }: Props = $props();

  let copied = $state(false);

  let tool = $derived(ADAPTER_TOOLS.find((t) => t === agent.installTool));
  // Managed installs come with code checks' setup; an older server sends no
  // installTool. An adapter set in config.toml runs before an app copy.
  let installable = $derived(!!tool && !agent.override && connectionStore.capabilities.codeDiagnostics === true);
  let flow = $derived(tool && installer.tool === tool ? installer : undefined);
  let proposal = $derived(flow?.state?.state === 'proposed' ? flow.state.proposal : undefined);
  let installing = $derived(flow?.state?.state === 'installing');

  const work =
    "Runs npm ci against a lockfile shipped with this app, with install scripts disabled and optional packages (the agent's own native builds) left out; every package is checked against its recorded hash. It runs with the host's Node.js 22 or later, else the app's own.";

  async function copy(): Promise<void> {
    if (agent.install && (await copyToClipboard(agent.install))) {
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } else {
      toast.show({ title: 'Could not copy the command', body: 'Select it and copy it by hand.', tone: 'error' });
    }
  }
</script>

<div class="adapter-missing" data-adapter-missing={agent.agent}>
  <p class="what">
    <strong>{agent.name}</strong> chats need its ACP adapter on {host}.
    {#if agent.problem}<span class="problem">{agent.problem}</span>{/if}
  </p>
  {#if installable && tool}
    {#if proposal}
      <ManagedInstallReview
        {proposal}
        where={host}
        {work}
        scope={`Nothing is installed globally or into a project; chats run it with your own ${agent.name} CLI.`}
        after="Chats use it once it is installed. An adapter on PATH or set in config.toml still comes first."
        installDisabled={installer.pending}
        dismissDisabled={installer.pending}
        onInstall={() => void installer.install()}
        onDismiss={() => void installer.dismiss()}
      />
    {:else if installing}
      <ManagedInstallProgress
        text={`Downloading ${tool} into this app's folder on ${host}… Progress is coarse; no percentage is available. Disconnecting cancels it. ${agent.name} can chat once it finishes.`}
        cancelDisabled={installer.pending}
        onCancel={() => void installer.cancel()}
      />
    {:else}
      <div class="install">
        <button type="button" title="Shows what would be downloaded; nothing is installed until you confirm" disabled={installer.busy} onclick={() => void installer.propose(tool!)}>
          {flow?.pending ? 'Checking…' : 'Install…'}
        </button>
        <span class="how">Installs a pinned copy for this app only, after you review it.</span>
      </div>
    {/if}
    {#if flow?.error}<p class="error" role="alert">{flow.error}</p>{/if}
  {/if}
  {#if agent.install}
    <p class="how">{installable ? 'Or install it yourself: run this' : 'To install it, run this'} on {host}:</p>
    <div class="command">
      <code>{agent.install}</code>
      <button type="button" onclick={() => void copy()}>{copied ? '✓ Copied' : 'Copy'}</button>
    </div>
  {/if}
  {#if agent.override}
    <p class="how">The path comes from <code>[chat.adapters]</code> in config.toml: fix or remove it there.</p>
  {/if}
</div>

<style>
  .adapter-missing {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 12px;
    line-height: 1.5;
  }

  p {
    margin: 0;
  }

  .problem {
    display: block;
    color: var(--text-secondary);
  }

  .how {
    color: var(--text-secondary);
  }

  .install {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .error {
    color: var(--error-color, #cf222e);
    overflow-wrap: anywhere;
  }

  button:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .command {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .command code {
    flex: 1;
    min-width: 0;
    padding: 4px 8px;
    border-radius: var(--radius-sm, 4px);
    background: var(--bg-secondary);
    font-family: var(--font-mono);
    font-size: 11.5px;
    overflow-wrap: anywhere;
    user-select: all;
  }

  button {
    padding: 4px 10px;
    border-radius: var(--radius-sm, 4px);
    background: var(--btn-bg);
    color: var(--text-primary);
    font-size: 12px;
    cursor: pointer;
    flex-shrink: 0;
  }

  button:hover {
    background: var(--btn-bg-hover);
  }
</style>

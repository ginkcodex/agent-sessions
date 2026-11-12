<script lang="ts">
  // The review of an app-managed install before anything is downloaded: what,
  // from where, checked how, and where it goes, on which host. Code checks
  // and the chat setup show the same card; nothing installs until Install.
  import type { ManagedToolProposal } from '../../managedTools';

  interface Props {
    proposal: ManagedToolProposal;
    /** Where it installs: "this computer", "the remote host" or a host's name. */
    where: string;
    /** How the install fetches and checks the tool. */
    work: string;
    /** What the install leaves alone. */
    scope: string;
    /** What happens once it is installed. */
    after: string;
    installDisabled?: boolean;
    dismissDisabled?: boolean;
    onInstall: () => void;
    onDismiss: () => void;
  }

  let { proposal, where, work, scope, after, installDisabled = false, dismissDisabled = false, onInstall, onDismiss }: Props = $props();

  /** Brings a card that appears after a click into view in a scrolled panel. */
  function reveal(node: HTMLElement) { node.scrollIntoView?.({ block: 'nearest' }); }
</script>

<div class="card" role="group" aria-label="Installation review" use:reveal>
  <p><strong>Install {proposal.tool} {proposal.version} on {where}?</strong></p>
  <p class="wrapped">Source: {proposal.source}</p>
  <p class="wrapped">Integrity: {proposal.integrity}</p>
  {#if proposal.note}<p class="wrapped">{proposal.note}</p>{/if}
  <p>{work} The download needs network access. {scope}</p>
  <p class="wrapped">App-private destination: {proposal.destination}</p>
  <p>This offer expires: {proposal.expires}. {after}</p>
  <p class="mono">Target host identity: {proposal.hostId}</p>
  <div class="actions">
    <button type="button" disabled={installDisabled} onclick={onInstall}>Install</button>
    <button type="button" disabled={dismissDisabled} onclick={onDismiss}>Not now</button>
  </div>
</div>

<style>
  .card { border: 1px solid var(--border-color); border-radius: 6px; padding: 6px 8px; margin: 6px 0; background: var(--bg-secondary); font-size: 12px; line-height: 1.45; }
  p { margin: 4px 0; }
  .wrapped { overflow-wrap: anywhere; white-space: pre-wrap; }
  .mono { overflow-wrap: anywhere; font-family: var(--font-mono); font-size: 11px; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 6px 0; }
  button { padding: 3px 8px; border: 1px solid var(--border-color); border-radius: 4px; cursor: pointer; font-size: 12px; }
  button:disabled { opacity: 0.5; cursor: default; }
</style>

<script lang="ts">
  // Code check settings: approval, runs and optional setup. Mounted only inside the
  // disclosed bottom details; the compact status button owns the summary.
  import { codeDiagnostics as checks } from '../../../stores/codeDiagnostics.svelte';
  import { manage } from '../../../stores/manage.svelte';
  import { liveLanguages, proposalRows, providerMode, providerStatus, remoteHost, syncFailureReason, toolName } from '../../../codeCheckLabels';
  import { MANAGED_TOOL_FOR_PROVIDER, type ManagedToolName } from '../../../managedTools';
  import type { CodeDiagnosticsProvider } from '../../../codeDiagnostics';
  import { CODE_LANGUAGES, languageList, type CodeLanguageId } from '../../../codeLanguages';
  let { blocked = false }: { blocked?: boolean } = $props();
  let dismissed = $state(false);
  // A new attachment gets its own approval choice. Not now never calls the backend.
  $effect(() => { void checks.project; dismissed = false; });
  const labels = {
    detached: 'Code checks disabled', attaching: 'Discovering code check configurations…',
    unsupported: 'Code checks unsupported on this connection', resyncing: 'Reconciling code checks; retained results are stale',
    disconnected: 'Code checks disconnected; retained results are stale', error: 'Code checks unavailable; status uncertain', ready: 'Code checks',
  };
  function perform(work: () => Promise<unknown>) { void work().catch(() => {}); } // Store owns bounded, textual errors.
  let remote = $derived(remoteHost(checks.project?.hostId));
  let watching = $derived(checks.providers.some(p => p.availability === 'available' && p.coverage.externalChanges === 'watching'));
  let failedDocuments = $derived((checks.liveDocuments ?? []).filter(d => d.failure));
  // Live languages offered for approval, and those running once approved.
  const rustProviders = ['rust-analyzer', 'cargo-check'];
  // Non-Rust folders register muted Rust rows; only Rust projects get Rust setup guidance.
  let rustProject = $derived(checks.providers.some(p => rustProviders.includes(p.providerId) && (!['unsupported', 'disabled'].includes(p.availability) || (p.coverage.roots?.length ?? 0) > 0)));
  // Text names only the languages that are on in Settings.
  let off = $derived(checks.languagesOff ?? []);
  let languagesOn = $derived(CODE_LANGUAGES.filter(l => !off.includes(l.id)));
  let languagesOff = $derived(CODE_LANGUAGES.filter(l => off.includes(l.id)).map(l => l.name));
  let ignored = $derived((checks.languagesIgnored ?? []).map(l => l.name));
  const navigationServers: Record<CodeLanguageId, string> = {
    go: 'gopls for Go', typescript: 'vtsls for JavaScript/TypeScript (Node.js)', svelte: 'the Svelte language server for .svelte files', python: 'Pyright for Python', rust: "the app's pinned rust-analyzer for Rust",
  };
  const liveServers: Partial<Record<CodeLanguageId, string>> = {
    go: 'gopls for Go', typescript: 'vtsls for TypeScript/JavaScript', svelte: 'the Svelte language server for .svelte files (macOS on Apple silicon only)', python: 'Ruff plus Pyright for Python (lint/syntax and type errors, respectively)',
  };
  const manualInstall: Partial<Record<CodeLanguageId, { text: string; mono?: boolean }[]>> = {
    go: [{ text: 'gopls with your Go toolchain' }],
    typescript: [{ text: 'npm install -g @vtsls/language-server@0.3.0', mono: true }],
    python: [{ text: 'uv tool install ruff==0.15.20', mono: true }, { text: 'npm install -g pyright', mono: true }],
  };
  let navigationText = $derived(languageList(languagesOn.map(l => navigationServers[l.id])));
  let liveText = $derived(languageList(languagesOn.flatMap(l => liveServers[l.id] ?? [])));
  let manualSteps = $derived(languagesOn.flatMap(l => manualInstall[l.id] ?? []));
  let proposedLive = $derived(liveLanguages(checks.providers, ['available', 'approvalRequired']));
  let activeLive = $derived(liveLanguages(checks.providers));
  let where = $derived(remote ? 'the remote host' : 'this computer');
  // A status that is unavailable without a tool means this host offers no managed installs at all.
  let canOffer = $derived(!!checks.managed && !(checks.managed.state === 'unavailable' && !checks.managed.tool));
  /** A tool this host cannot install, such as gopls without Go; its row says why instead of offering it again. */
  let refused = $derived(checks.managed?.state === 'unavailable' && checks.managed.tool && checks.managed.message ? checks.managed.tool : undefined);
  /** The app-managed install a missing language server can get here, if any. */
  function offerFor(provider: CodeDiagnosticsProvider): ManagedToolName | undefined {
    const tool = MANAGED_TOOL_FOR_PROVIDER[provider.providerId];
    if (!tool || !canOffer || tool === refused || (provider.availability !== 'missingTool' && !provider.missingTool)) return undefined;
    // An app copy that exists (verified or not) is not offered again; "missingTool" means none.
    const install = checks.managedInstalls?.[tool];
    return !install || install === 'missingTool' ? tool : undefined;
  }
  /** The tool whose review card is open; its Install button is replaced by the card. */
  let reviewing = $derived(checks.managed?.state === 'proposed' && !checks.managedDismissed && !checks.autoPending ? checks.managed.proposal?.tool : undefined);
  // With nothing approvable, the approval card leads with the installs that would change that.
  let proposedRows = $derived(proposalRows(checks.snapshot?.proposal?.tools ?? []));
  let preparation = $derived(checks.snapshot?.preparation);
  let preparationSteps = $derived(checks.snapshot?.proposal?.preparation ?? []);
  let installFirst = $derived(checks.snapshot?.proposal?.fingerprint ? [] : checks.providers.flatMap(p => offerFor(p) ?? []).filter(t => t !== reviewing));
  /** Why a server is missing and how to get it, when no Install button says so; svelte-check has no managed install. */
  function setupNote(provider: CodeDiagnosticsProvider): string {
    const tool = MANAGED_TOOL_FOR_PROVIDER[provider.providerId];
    if (!tool) return provider.providerId === 'svelte-check' ? 'Setup not supported here: install it on the host yourself, then rediscover checkers.' : '';
    if (provider.availability !== 'missingTool' && !provider.missingTool) return '';
    if (tool === refused) return checks.managed!.message!;
    if (checks.managed && !canOffer) return 'No app-managed install is offered on this host: install it yourself, then rediscover checkers.';
    if (checks.managedInstalls[tool] === 'unavailable') return `This app's copy of ${tool} on ${where} failed verification and is not used. Delete its folder under the app's managed-tools directory, then rediscover checkers.`;
    return '';
  }
  /** Language servers this project uses, with their app-managed install where one exists. */
  let setupRows = $derived(checks.providers.flatMap(p => {
    const tool = MANAGED_TOOL_FOR_PROVIDER[p.providerId];
    if (!tool || !['available', 'missingTool'].includes(p.availability)) return [];
    const install = checks.managedInstalls[tool];
    const text = p.availability === 'available' ? (install === 'available' ? 'Ready · app copy installed' : 'Ready')
      : install === 'available' ? 'App copy installed · rediscover checkers'
      : install === 'unavailable' ? 'App copy failed verification and is not used'
      : 'Not found';
    return [{ providerId: p.providerId, text }];
  }));
  /** One count pill per checker status, in first-seen order, for the collapsed checker list. */
  let statusCounts = $derived([...checks.providers.reduce((counts, p) => {
    const { text, tone } = providerStatus(p);
    const count = counts.get(text);
    counts.set(text, { text, tone, count: (count?.count ?? 0) + 1 });
    return counts;
  }, new Map<string, { text: string; tone: string; count: number }>()).values()]);
  // The list opens by itself only when a checker offers an install, cannot get one, or has failed.
  let checkersNeedAttention = $derived(checks.providers.some(p => (checks.status === 'ready' && offerFor(p) && offerFor(p) !== reviewing) || (refused && MANAGED_TOOL_FOR_PROVIDER[p.providerId] === refused) || p.failure || p.phase === 'failed'));
  /** Brings a card that appears after a click into view in the scrolled panel. */
  function reveal(node: HTMLElement) { node.scrollIntoView({ block: 'nearest' }); }
  const installWork: Record<ManagedToolName, string> = {
    gopls: "Downloads gopls source and its dependencies through the Go module proxy, verified by the Go checksum database, and builds it with this host's Go. Go itself is never downloaded or upgraded.",
    ruff: 'Downloads one release archive and checks its SHA-256 before unpacking anything.',
    vtsls: 'Runs npm ci against a lockfile shipped with this app, with install scripts disabled; every package is checked against its recorded hash.',
    pyright: 'Runs npm ci against a lockfile shipped with this app, with install scripts disabled; uses the host Node.js or downloads an app-managed copy.',
    'rust-analyzer': 'Downloads one release archive and checks its SHA-256 before unpacking anything. Installing it does not approve running it or prepare the Rust toolchain.',
    'svelte-language-server': 'Runs npm ci against a lockfile shipped with this app, with install scripts disabled; uses the host Node.js or downloads an app-managed copy. Once the project is approved, the server loads its svelte.config and vite.config files.',
  };
</script>

{#snippet rediscoverButton()}
  <button type="button" title="Looks for language servers and checkers again; approves and starts nothing" disabled={blocked || !!checks.pending} onclick={() => perform(() => checks.rediscover())}>Rediscover checkers</button>
{/snippet}

{#snippet installButton(tool: ManagedToolName)}
  <button type="button" title="Shows what would be downloaded; nothing is installed until you confirm" disabled={blocked || checks.managedPending || checks.managed?.state === 'installing'} onclick={() => perform(() => checks.managedAction('propose', tool))}>Install {tool}…</button>
{/snippet}

<section class="code-checks" aria-label="Code check settings" aria-busy={!!checks.pending || checks.status === 'attaching'}>
  {#if checks.status !== 'ready' || checks.pending}
    <p role="status" class="muted">{labels[checks.status]}{checks.pending ? ` · ${checks.pending} request pending` : ''}</p>
  {/if}
  {#if checks.error}
    <p class="wrapped">{checks.error.message}</p>
    {#if checks.snapshot && checks.status === 'error'}
      <button type="button" disabled={blocked || !!checks.pending} onclick={() => checks.refresh()}>Refresh check status</button>
      {#if checks.revoking}
        <p>Revocation is uncertain. Retry revoking before reconnecting; remembered permission may remain.</p>
        <button type="button" disabled={blocked || !!checks.pending} onclick={() => perform(() => checks.revoke())}>Retry revocation</button>
      {/if}
    {/if}
  {/if}
  {#if checks.status === 'ready' && ignored.length}
    <p role="status" class="wrapped">This host's Agent Sessions server is older and still checks {languageList(ignored)}, though {ignored.length === 1 ? 'it is' : 'they are'} off in Settings. Update the server on this host to leave {ignored.length === 1 ? 'it' : 'them'} out.</p>
  {/if}
  {#if checks.languagesChanged}
    <div class="actions"><span class="muted">Language settings changed since these checkers were found.</span>{@render rediscoverButton()}</div>
  {/if}
  {#if checks.status === 'ready' && checks.snapshot}
    {#if preparation}
      <div class="card" role="status" aria-label="Workspace preparation">
        {#if preparation.state === 'running'}
          <p><strong>Preparing workspace…</strong></p>
          <p class="wrapped">{preparation.step ?? ''}{preparation.index != null && preparation.total ? ` (step ${preparation.index}/${preparation.total})` : ''}</p>
        {:else if preparation.state === 'failed'}
          <p><strong>Workspace preparation failed</strong></p>
        {:else}
          <p><strong>Workspace preparation canceled</strong></p>
        {/if}
        {#if preparation.message}<p class="wrapped">{preparation.message}</p>{/if}
        <p class="muted">No checker runs while preparation is {preparation.state === 'running' ? 'in progress' : 'not active'}; no project tool is started.</p>
        <button type="button" disabled={blocked || !!checks.pending} onclick={() => perform(() => checks.refresh())}>Refresh check status</button>
        {#if preparation.state !== 'running'}{@render rediscoverButton()}{/if}
        <button type="button" disabled={blocked || !!checks.pending || checks.revoking} onclick={() => perform(() => checks.revoke())}>Revoke approval</button>
      </div>
    {:else if checks.snapshot.approval.state !== 'approved'}
      {#if dismissed}
        <p>Code checks disabled for now. No tools have been started.</p>
        <button type="button" onclick={() => (dismissed = false)}>Enable code checks for this project</button>
      {:else}
        <div class="card">
          <p><strong>Enable code checks for this project?</strong></p>
          <!-- One line of risk stays visible; the full explanation and tool list are a click away. -->
          {#if checks.snapshot.proposal && !checks.snapshot.proposal.tools.length}
            <p>Only built-in checks are available on this host; they run no program.</p>
          {:else}
            <p>Runs {proposedRows.length === 1 ? 'one tool' : `${proposedRows.length} tools`} on this host with your account's permissions. This is not a sandbox; only approve projects you trust.</p>
          {/if}
          {#if checks.liveEnabled && proposedLive && navigationText}<p class="muted">Approval also enables go to definition, references and hover with available language servers: {navigationText}.{off.includes('python') ? '' : ' Ruff alone does not provide navigation.'} This works in opened files, including unsaved edits; running Check Project is not needed.</p>{/if}
          {#if rustProject && checks.providers.some(p => rustProviders.includes(p.providerId) && ['available', 'approvalRequired'].includes(p.availability))}<p>Rust approval permits build scripts and procedural macros to execute with your account's permissions and write build/cache output. Cargo uses offline + locked mode; that prevents Cargo downloads, not network access by project code.</p>{/if}
          {#if preparationSteps.length}<p>Approval first prepares the listed Rust toolchain and dependency cache. These steps may download tools or packages and write the reviewed output paths. Checks and live analysis stay inactive until preparation succeeds and discovery validates the result.</p>{/if}
          {#if checks.snapshot.approval.state === 'revoked'}<p>Permission revoked. Pending and running checks are being canceled; previous findings have been cleared.</p>{/if}
          {#if checks.approvalBlocker}<p role="status" class="muted">{checks.approvalBlocker}</p>{/if}
          {#if installFirst.length}<p>Install a language server on {where} first, or install one yourself and rediscover checkers. Installing asks for confirmation and approves nothing.</p>{/if}
          <div class="actions">
            {#each installFirst as tool (tool)}{@render installButton(tool)}{/each}
            <button type="button" disabled={blocked || !!checks.approvalBlocker || !checks.snapshot.proposal?.fingerprint} onclick={() => perform(() => checks.approve('session'))}>Enable for this session</button>
            <button type="button" disabled={blocked || !!checks.approvalBlocker || !checks.snapshot.proposal?.fingerprint} onclick={() => perform(() => checks.approve('persistent'))}>Always allow this project</button>
            {#if !checks.snapshot.proposal?.fingerprint}{@render rediscoverButton()}{/if}
            <button type="button" disabled={!!checks.pending} onclick={() => (dismissed = true)}>Not now</button>
          </div>
          <details>
            <summary>What will run and what approval means</summary>
            {#if checks.snapshot.proposal && !checks.snapshot.proposal.tools.length}
              <p>The app reads saved project files and watches them for changes. No external checker was found, so type and vet errors are not reported.</p>
            {:else}
              <p>Project configuration and plugins can execute code. Tools may read files and dependencies, access the network, and write caches or other files. This is not a sandbox or a read-only guarantee.</p>
            {/if}
            <p class="muted">{checks.liveEnabled && proposedLive ? `Check Project runs on saved files. After approval, opened ${proposedLive} files are also sent unsaved to the listed language servers for live diagnostics; drafts are never saved.` : 'Only saved files are checked; drafts are never saved or checked automatically.'} Session approval lasts until this connection or app session ends. Always allow remembers this host and project; later tool or configuration changes do not ask again until you revoke it. Approval does not start a check.</p>
            {#if preparationSteps.length}
              <p><strong>Preparation steps (before any checks)</strong></p>
              <ol>
                {#each preparationSteps as step}<li class="wrapped">{step.name}<div class="mono">{step.command}</div></li>{/each}
              </ol>
              {#if checks.snapshot.proposal?.outputs?.length}<p>Reviewed preparation outputs:</p><ul>{#each checks.snapshot.proposal.outputs as output}<li class="mono wrapped">{output}</li>{/each}</ul>{/if}
            {/if}
            <ul class="rows">
              {#each checks.snapshot.proposal?.analyzers ?? [] as analyzer}
                <li><span class="name">{toolName(analyzer.providerId)}</span><span class="pill">built-in</span><span class="detail">{analyzer.description}</span></li>
              {/each}
              {#each proposedRows as row (row.executable)}
                <li><span class="name" title={row.name}>{row.name}</span>{#if row.buildContexts}<span class="pill">{row.uses} build contexts (includes default use)</span>{:else if row.uses > 1}<span class="pill">{row.uses} workspaces</span>{/if}<span class="detail mono">{row.executable} · Configuration: {row.configPaths.join(', ') || 'none discovered'}{#if row.buildContexts} · Inferred tags: {row.buildContexts.join('; ')}{/if}</span></li>
              {/each}
            </ul>
          </details>
        </div>
      {/if}
    {:else}
      <div class="actions">
        <span class="muted">{checks.revoking ? 'Revocation pending; checks disabled' : `Approved for ${checks.snapshot.approval.mode === 'persistent' ? 'this project persistently' : 'this session'}`}.</span>
        {#if checks.approved && checks.providers.some(p => p.availability === 'available' && p.coverage.kind === 'savedProject')}
          <button type="button" disabled={blocked || !checks.canRun} onclick={() => perform(() => checks.run())}>Check Project</button>
        {/if}
        {#if checks.approved && checks.running}
          <button type="button" disabled={blocked || !!checks.pending} onclick={() => perform(() => checks.cancel())}>Cancel check</button>
        {/if}
        <button type="button" title="Re-reads open files from disk so fixes made elsewhere are checked; saves nothing" disabled={blocked || !!checks.pending} onclick={() => perform(() => checks.refresh())}>Refresh check status</button>
        <button type="button" disabled={blocked || checks.pending === 'revoke' || checks.revoking} onclick={() => perform(() => checks.revoke())}>Revoke approval</button>
      </div>
      <p class="muted">{watching ? 'Saved Go files are checked automatically when they change on disk.' : ''}{checks.liveEnabled && activeLive ? ` Opened ${activeLive} files are checked live as you type.` : ''} Check Project runs the on-demand checkers on saved files.</p>
    {/if}
  {/if}
  {#if checks.project && checks.status === 'ready'}
    {#if checks.managedNotice}<p role="status" class="wrapped">{checks.managedNotice}</p>{/if}
    {#if checks.managed?.message && !refused}<p role="status" class="wrapped">{checks.managed.message}</p>{/if}
    {#if reviewing && checks.managed?.proposal}
      {@const proposal = checks.managed.proposal}
      <div class="card" role="group" aria-label="Installation review" use:reveal>
        <p><strong>Install {proposal.tool} {proposal.version} on {where}?</strong></p>
        <p class="wrapped">Source: {proposal.source}</p>
        <p class="wrapped">Integrity: {proposal.integrity}</p>
        {#if proposal.note}<p class="wrapped">{proposal.note}</p>{/if}
        <p>{installWork[proposal.tool]} The download needs network access. Nothing is installed globally or into the project, and no project configuration is used.</p>
        <p class="wrapped">App-private destination: {proposal.destination}</p>
        <p>This offer expires: {proposal.expires}. Installation does not approve running it; code checks are rediscovered afterwards and ask for approval separately.</p>
        <p class="mono">Target host identity: {proposal.hostId}</p>
        <div class="actions">
          <button type="button" disabled={blocked || checks.managedPending} onclick={() => perform(() => checks.managedAction('install'))}>Install</button>
          <button type="button" disabled={checks.managedPending} onclick={() => perform(() => checks.managedAction('dismiss'))}>Not now</button>
        </div>
      </div>
    {:else if checks.managed?.state === 'installing' || checks.autoPending}
      <div class="card" role="group" aria-label="Installation progress">
        <p role="status">Downloading {checks.managed?.tool ?? checks.autoPending ?? 'the tool'} into this app's folder on {where}… Progress is coarse; no percentage is available. Disconnecting cancels it. Code checks are rediscovered when it finishes; running it still needs your approval.</p>
        {#if checks.autoInstall}<p class="muted">Missing language servers are downloaded automatically. Turn this off under Settings › Code checks.</p>{/if}
        <button type="button" disabled={blocked || checks.managedPending || checks.managed?.state !== 'installing'} onclick={() => perform(() => checks.managedAction('cancel'))}>Cancel download</button>
      </div>
    {:else if checks.managed?.state === 'installed' && checks.managed.tool}
      <p role="status">{checks.managed.tool} is installed for this app on {where}. Code checks were rediscovered; approve them to use it. Setup started no server.</p>
    {:else if checks.managedDismissed}
      <p>Setup dismissed. No installation requested.</p>
    {/if}
  {/if}
  {#if checks.snapshot}
    {#if checks.providers.length === 0}<p class="muted">No supported checker discovered.</p>{/if}
    {#if checks.truncated}<p class="muted">Results were shortened or omitted to stay within limits; no complete-coverage claim.</p>{/if}
    {#if failedDocuments.length}
      <div class="card warn" role="group" aria-label="Live sync problems">
        <p><strong>Live sync stopped for {failedDocuments.length === 1 ? '1 file' : `${failedDocuments.length} files`}.</strong> Their findings are stale. Browsing and drafts still work.</p>
        <ul class="rows">
          {#each failedDocuments as document (document.documentId)}
            <li>
              <span class="name mono" title={document.path}>{document.path}</span>
              <span class="detail" role="status">Live document synchronization failed ({document.failure}): {syncFailureReason(document.failure ?? '')}</span>
              {#if document.canReopen}
                <button type="button" disabled={blocked || !!checks.pending} aria-label={`Reopen live synchronization for ${document.path}`} onclick={() => checks.reopenDocument(document.path, document.documentId)}>Reopen</button>
              {/if}
            </li>
          {/each}
        </ul>
        {#if failedDocuments.some(d => d.canReopen)}
          <p class="muted">Reopen sends the displayed text again with a fresh document identity under the current approval, at most twice. Nothing is saved. It is not a provider restart; same-URI results may remain uncertain until a new isolated session.</p>
        {/if}
      </div>
    {/if}
    {#if checks.providers.length}
      <details class="checkers" open={checkersNeedAttention}>
        <summary><span class="heading">Checkers</span>{#each statusCounts as s (s.text)}<span class="pill tone-{s.tone}">{s.count} {s.text.toLowerCase()}</span>{/each}{#if checks.partial}<span class="muted">partial coverage</span>{/if}{#if checks.truncated}<span class="muted">truncated</span>{/if}</summary>
        <ul class="rows">
          {#each checks.providers as provider (provider.providerId)}
            {@const status = providerStatus(provider)}
            {@const note = setupNote(provider)}
            {@const offer = checks.status === 'ready' ? offerFor(provider) : undefined}
            <li>
              <span class="name">{toolName(provider.providerId)}</span>
              <span class="pill tone-{status.tone}">{status.text}</span>
              <span class="detail">{providerMode(provider)}{provider.freshness !== 'current' && provider.availability === 'available' ? ` · ${provider.freshness}` : ''}</span>
              {#if offer && offer !== reviewing}{@render installButton(offer)}{/if}
              {#if checks.approved && provider.phase === 'failed' && provider.availability === 'available' && provider.coverage.kind === 'savedProject'}
                <button type="button" disabled={blocked || !checks.canRetry(provider.providerId)} onclick={() => perform(() => checks.retry(provider.providerId))}>Retry {provider.providerId}</button>
              {/if}
              {#if provider.failure}<p class="sub wrapped">{provider.failure.message} (retained findings are not a successful check)</p>{/if}
              {#if provider.phase === 'canceled'}<p class="sub">Check canceled; retained findings may be stale.</p>{/if}
              {#if checks.approved && provider.phase === 'failed' && provider.availability === 'available' && provider.coverage.kind !== 'savedProject'}
                <p class="sub">No provider restart action is supported here. A failed session can be recreated by a subsequent document open; reopening in a surviving session may remain uncertain until a new isolated session. Refresh check status only reads status.</p>
              {/if}
              {#if provider.availability !== 'available' && note}<p class="sub">{note}</p>{/if}
              {#if provider.availability === 'disabled'}<p class="sub">Turn this language on under Settings › Code checks.</p>
              {:else if provider.notice}<p class="sub wrapped">{provider.notice}</p>{/if}
            </li>
          {/each}
        </ul>
      </details>
      <details>
        <summary>Coverage details</summary>
        <ul class="tech">
          {#each checks.providers as provider (provider.providerId)}
            <li class="mono">{provider.providerId}: {provider.availability} · {provider.phase} · {provider.freshness} · {provider.coverage.kind === 'savedProject' ? 'saved project files' : 'opened documents'} · roots: {provider.coverage.roots?.join(', ') || '(project root)'}{provider.coverage.partial ? ' · partial' : ''}{provider.truncated ? ' · truncated' : ''}
              {#if provider.coverage.externalChanges === 'watching'} · watching saved project files on disk; updates after external changes (open drafts are not analyzed){/if}
              {#if provider.coverage.externalChanges === 'unwatched'} · external changes unwatched; unopened-file coverage stale — reopen, refresh or rerun to reconcile saved context (not continuous sync){/if}
            </li>
          {/each}
        </ul>
      </details>
    {/if}
  {/if}
  {#if checks.project && checks.status === 'ready'}
    <details aria-label="Language-server setup">
      <summary>Tools setup · language servers</summary>
      <p class="muted">Like Zed, each language server is looked up in the project first (node_modules/.bin, .venv), then on your login shell's PATH, then in this app's own install folder on {where}. {!checks.liveEnabled ? 'Live diagnostics are not available on this connection.' : liveText ? `Live diagnostics (experimental) use them for opened files only, after project approval: ${liveText}.` : ''}{languagesOn.length ? ` For ${languageList(languagesOn.map(l => l.name))}, a missing supported server that this project's files need is ${checks.autoInstall ? "downloaded into this app's folder automatically, at most once per app session" : "offered for download into this app's folder, for you to review"}.` : ''} Installing is separate from trusting this project: it never approves or starts a server. Revoking project approval also abandons a pending installation offer and cancels an owned installation.</p>
      {#if rustProject}<p class="muted">Rust live diagnostics and navigation require this app's pinned managed rust-analyzer on {where}; an unrelated installed server is not used for live analysis. Cargo workspaces must stay inside the opened folder. Review the listed preparation steps before approval: they may install an explicitly selected toolchain through existing rustup and fetch dependencies with <span class="mono">--locked</span>, never bootstrap rustup. Review missing Cargo.lock files separately. Check Project runs <span class="mono">cargo check --workspace --offline --locked --message-format=json</span> on saved files with default features and build-selected targets. Normal checks never silently download dependencies or change locks; live analysis covers opened .rs documents, including unsaved edits. Build scripts and procedural macros are not sandboxed.</p>{/if}
      {#if languagesOff.length}<p class="muted">{languageList(languagesOff)} {languagesOff.length === 1 ? 'is' : 'are'} off in Settings and {languagesOff.length === 1 ? 'gets' : 'get'} no code checks, navigation or downloads.</p>{/if}
      <p class="muted">The opened folder does not need Git. Nested Python and JavaScript/TypeScript projects use their own configuration and environments; coverage roots are listed under Coverage details.</p>
      <p class="muted">Languages and automatic downloads are chosen in <button type="button" class="link" onclick={() => manage.openSettings('checks')}>Settings › Code checks</button>.</p>
      {#if setupRows.length}
        <ul class="rows">
          {#each setupRows as row (row.providerId)}
            <li><span class="name">{toolName(row.providerId)}</span><span class="detail">{row.text}</span></li>
          {/each}
        </ul>
      {/if}
      {#if !checks.managed && !checks.managedNotice}<p class="muted">Setup status not loaded yet.</p>{/if}
      <div class="actions">
        <button type="button" disabled={blocked || checks.managedPending} onclick={() => perform(() => checks.refreshManaged())}>Refresh setup status</button>
        {@render rediscoverButton()}
      </div>
      {#if manualSteps.length}<p class="muted">To install a server yourself instead, add it to the project or put it on your PATH: {#each manualSteps as step, i}{i ? (i === manualSteps.length - 1 ? ' or ' : ', ') : ''}{#if step.mono}<span class="mono">{step.text}</span>{:else}{step.text}{/if}{/each}. Then rediscover checkers.</p>{/if}
    </details>
  {/if}
  {#if checks.project}
    <details>
      <summary>Technical details</summary>
      <p class="mono">Host: {checks.project.hostId} · Root: {checks.project.root}</p>
      {#if checks.snapshot?.proposal?.fingerprint}<p class="mono">Proposal identity: {checks.snapshot.proposal.fingerprint}</p>{/if}
      {#if proposedRows.some(row => row.buildContexts)}<p class="muted">Go files may be analyzed in separate automatically inferred host contexts, at most four lazily started gopls processes. Unsaved dependency changes are not synchronized across those contexts. Package, cgo or dependency failures can still prevent navigation. Opening this disclosure starts nothing; a new context inventory changes the project fingerprint and uses the ordinary approval buttons.</p>{/if}
    </details>
  {/if}
</section>

<style>
  .code-checks { padding: 6px 10px; font-size: 12px; line-height: 1.45; }
  p { margin: 4px 0; }
  .checkers > summary { display: flex; flex-wrap: wrap; align-items: center; gap: 4px 8px; }
  .checkers > summary::before { content: '▶'; font-size: 0.75em; }
  .checkers[open] > summary::before { content: '▼'; }
  .checkers > summary::-webkit-details-marker { display: none; }
  .heading { font-weight: 600; }
  .muted { color: var(--text-secondary); }
  .mono { overflow-wrap: anywhere; font-family: var(--font-mono); font-size: 11px; }
  .wrapped { overflow-wrap: anywhere; white-space: pre-wrap; }
  .card { border: 1px solid var(--border-color); border-radius: 6px; padding: 6px 8px; margin: 6px 0; background: var(--bg-secondary); }
  .card.warn { border-color: rgba(210, 153, 34, 0.5); }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin: 6px 0; }
  .rows { list-style: none; margin: 4px 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
  .rows li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 8px; padding: 3px 6px; border-radius: 4px; }
  .rows li:hover { background: var(--hover-bg); }
  .name { font-weight: 600; min-width: 12ch; max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .detail { color: var(--text-secondary); min-width: 0; overflow-wrap: anywhere; }
  .rows li > button { margin-left: auto; }
  .sub { flex-basis: 100%; margin: 0; padding-left: 2px; font-size: 11px; color: var(--text-secondary); }
  .pill { padding: 0 6px; border-radius: 8px; font-size: 11px; line-height: 16px; white-space: nowrap; border: 1px solid var(--border-color); color: var(--text-secondary); }
  .tone-ok { color: #3fb950; border-color: rgba(63, 185, 80, 0.5); }
  .tone-busy { color: var(--accent-color); border-color: var(--accent-color); }
  .tone-bad { color: #f85149; border-color: rgba(248, 81, 73, 0.5); }
  .tone-warn { color: #d29922; border-color: rgba(210, 153, 34, 0.5); }
  :global(:root:not([data-theme='dark'])) .tone-ok { color: #1a7f37; }
  :global(:root:not([data-theme='dark'])) .tone-bad { color: #cf222e; }
  :global(:root:not([data-theme='dark'])) .tone-warn { color: #9a6700; }
  .tech { margin: 4px 0; padding-left: 16px; }
  .toggle { display: flex; align-items: center; gap: 6px; margin: 6px 0; }
  button { padding: 3px 8px; border: 1px solid var(--border-color); border-radius: 4px; cursor: pointer; font-size: 12px; }
  button:disabled { opacity: 0.5; cursor: default; }
  button.link { padding: 0; border: none; background: none; color: var(--accent-color); font-size: inherit; text-decoration: underline; }
  details { margin: 6px 0; }
  summary { cursor: pointer; color: var(--text-secondary); }
</style>

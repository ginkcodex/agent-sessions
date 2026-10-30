<script lang="ts">
  import { gitHistory } from '../../../stores/gitHistory.svelte';
  let { searching = false }: { searching?: boolean } = $props();
  function date(time: number) { const value = new Date(time * 1000); return Number.isNaN(value.getTime()) ? '' : value.toLocaleDateString(); }
  function iso(time: number) { const value = new Date(time * 1000); return Number.isNaN(value.getTime()) ? undefined : value.toISOString(); }
  function navigate(e: KeyboardEvent) {
    if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key) || e.altKey || e.ctrlKey || e.metaKey) return;
    const row = e.currentTarget as HTMLButtonElement;
    const rows = [...(row.closest('ul')?.querySelectorAll<HTMLButtonElement>('[data-commit]') ?? [])];
    const index = rows.indexOf(row);
    const next = e.key === 'Home' ? 0 : e.key === 'End' ? rows.length - 1 : Math.max(0, Math.min(rows.length - 1, index + (e.key === 'ArrowDown' ? 1 : -1)));
    if (rows[next]) { e.preventDefault(); rows[next].focus(); rows[next].click(); }
  }
</script>

<div class="commits" aria-busy={gitHistory.loading}>
  {#if gitHistory.blockedReason}<p role="status">{gitHistory.blockedReason}</p>
  {:else}
    {#if gitHistory.discoveryError}<p role="alert">{gitHistory.discoveryError}</p>{/if}
    {#if gitHistory.refsError}<p role="alert">{gitHistory.refsError}</p>{/if}
    {#if gitHistory.pageError}<p role="alert">{gitHistory.pageError}</p>{/if}
    {#if gitHistory.repositoriesLoading}<p role="status">Discovering repositories…</p>
    {:else if gitHistory.requiresRepositorySelection}
      <p role="status">This workspace contains {gitHistory.repositories.length} repositories. Choose one to browse its history.</p>
      <ul class="repositories" aria-label="Repositories">
        {#each gitHistory.repositories as repo (repo.path)}<li><button type="button" title={repo.path} onclick={() => void gitHistory.selectRepository(repo.path)}>{repo.path}</button></li>{/each}
      </ul>
    {:else if !gitHistory.repositories.length && !gitHistory.discoveryError}<p role="status">No Git repositories found in this workspace.</p>
    {:else if gitHistory.refsLoading}<p role="status">Loading branches…</p>
    {:else if !gitHistory.commits.length && !gitHistory.pageError && !gitHistory.refsError && !gitHistory.discoveryError}
      <p role="status">{gitHistory.pagesLoading || searching ? 'Loading commits…' : gitHistory.unborn ? 'This repository has no commits yet (unborn HEAD).' : gitHistory.query ? 'No commits match this search.' : 'No commits in this history.'}</p>
    {/if}
    {#if gitHistory.commits.length}
      <div class="columns" aria-hidden="true"><span>Subject</span><span>Author</span><span>Date</span><span>OID</span></div>
      <ul aria-label="Commits">
        {#each gitHistory.commits as commit (commit.oid)}
          <li><button type="button" data-commit={commit.oid} class:selected={gitHistory.selectedCommit === commit.oid} aria-current={gitHistory.selectedCommit === commit.oid ? 'true' : undefined} tabindex={gitHistory.selectedCommit === commit.oid || (!gitHistory.selectedCommit && commit === gitHistory.commits[0]) ? 0 : -1} title={`${commit.subject}\n${commit.author.name} <${commit.author.email}>\n${commit.oid}`} onclick={() => void gitHistory.selectCommit(commit.oid)} onkeydown={navigate}>
            <span class="subject">{commit.subject || '(No subject)'}</span><span class="author">{commit.author.name}</span><time datetime={iso(commit.author.time)}>{date(commit.author.time)}</time><code>{commit.oid.slice(0, 8)}</code>
          </button></li>
        {/each}
      </ul>
    {/if}
    {#if gitHistory.pagesLoading && gitHistory.commits.length}<p role="status">Loading more commits…</p>{/if}
    {#if gitHistory.shallow}<p role="status">Shallow repository: earlier history may be missing.</p>{/if}
    {#if gitHistory.limited}<p role="status">History search reached a traversal limit; results may be incomplete.</p>{/if}
    {#if gitHistory.retainedLimit}<p role="status">Display limit reached. Narrow your search to see other commits.</p>{/if}
    {#if gitHistory.repositoriesTruncated}<p role="status">Repository discovery was truncated; some repositories may not be listed.</p>{/if}
    {#if gitHistory.refsTruncated}<p role="status">Branch listing was truncated; some refs may not be listed.</p>{/if}
    {#if gitHistory.cursor && !gitHistory.retainedLimit}<button class="more" type="button" disabled={!gitHistory.canLoadMore} onclick={() => void gitHistory.loadMore()}>Load more</button>{/if}
  {/if}
</div>

<style>
  .commits { font-size: .8rem; line-height: 1.4; } p { padding: 8px 12px; margin: 0; color: var(--text-secondary); overflow-wrap: anywhere; } [role='alert'] { color: var(--error-color, #d55); }
  .columns, li button { display: grid; grid-template-columns: minmax(70px, 1fr) minmax(50px, 22%) 76px 66px; gap: 8px; align-items: center; padding: 7px 10px; }
  .columns { position: sticky; top: 0; background: var(--bg-secondary); color: var(--text-secondary); font-size: .75rem; }
  ul { list-style: none; margin: 0; padding: 0; } li button { width: 100%; border: 0; border-radius: 0; text-align: left; background: transparent; color: var(--text-primary); cursor: pointer; font: inherit; }
  .repositories li button { display: block; overflow-wrap: anywhere; font-family: var(--font-mono); }
  li button:hover { background: var(--hover-bg); } li button.selected { background: var(--active-bg); } li button:focus-visible { outline: 2px solid var(--accent-color); outline-offset: -2px; }
  .subject, .author { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } time, code { font-size: .75rem; } code { font-family: var(--font-mono); }
  .more { font: inherit; margin: 10px; padding: 6px 12px; color: var(--text-primary); background: var(--bg-secondary); border: 1px solid var(--border-color); border-radius: 4px; cursor: pointer; } .more:disabled { opacity: .6; cursor: default; }
  @container (max-width: 580px) { .columns, li button { grid-template-columns: minmax(60px, 1fr) minmax(45px, 20%) 68px 58px; gap: 5px; padding: 7px 6px; } }
</style>

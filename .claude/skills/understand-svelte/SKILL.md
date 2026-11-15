---
name: understand-svelte
description: Update an existing Understand-Anything graph incrementally, with compiler-backed Svelte scripts, props and SvelteKit route context, without a count-triggered full rebuild.
argument-hint: "[project-directory]"
---

# Understand Svelte

Use this project-local workflow instead of the stock `/understand` for incremental updates involving Svelte. This skill **does not modify** the installed Understand-Anything plugin or change global hooks. Requires Node >=24 (synchronous module load hooks), built Understand-Anything **2.9.8**, and the target project's installed Svelte 5 compiler. No silent installation. `UA_PLUGIN_ROOT` may select a different installation of the same version.

## Entry point

Resolve this skill's absolute directory from the directory containing this SKILL.md. Below, `RUN` means `<skill-directory>/run.mjs`, `ROOT` is the target project directory (default current git root), and `UA` is `ROOT/.understand-anything` if it exists, otherwise `ROOT/.ua`.

All plugin script commands must go through `node "$RUN" <command> "$ROOT" ...`. This installs the same deterministic Svelte adapter for **every Node child process**, including the Python merge script's validator and the finalizer's independent save-time validation. Calling bundled scripts directly loses Svelte support.

The installed plugin is not edited: a subprocess-local preload integrates the adapter and version-checked source substitutions. Exact-match integration failures stop the run. Never weaken or forge symbol evidence to work around a failure.

## Update workflow

1. Read `UA/meta.json`, check graph and fingerprints exist. If there is no valid baseline, report that a one-time initial `/understand` build is needed. Do not launch that expensive build without asking.
2. Require a clean working tree for **analyzed paths**. The upstream validator checks committed source against the worktree. Do not suppress that check or commit without being asked. A clean detached worktree can be used for a graph at HEAD; say explicitly that uncommitted changes are not included. The wrapper itself never redirects ephemeral worktree outputs into the main checkout.
3. Run `node "$RUN" prepare "$ROOT" <meta.gitCommitHash>`. Respect the helper's inventory and structural-diff list. Do not construct an LLM analysis list by hand. The local classifier uses incremental architecture refresh, not full rebuild, for large structural diffs. Missing/corrupt/incompatible baselines still require the upstream full-build path.
4. Read `UA/intermediate/incremental-plan.json`:
   - `SKIP`: run wrapped `finalize`; report no LLM analysis.
   - `PARTIAL_UPDATE` / `ARCHITECTURE_UPDATE`: continue below.
   - `FULL_UPDATE`: stop and explain the baseline problem. Do not relabel this plan.
5. For nonempty `filesToReanalyze`, run `node "$RUN" batches "$ROOT" --changed-files="$UA/intermediate/changed-files.json"`. Dispatch only these batches, at most 5 concurrently. Report batch index, count, filenames. Fuse small dispatches only if they still write one `batch-<originalIndex>.json` (or numbered parts) per original batch.
6. Use a general-purpose file-analysis agent with the installed plugin's `agents/file-analyzer.md` as the baseline protocol, **overridden explicitly by this skill** for extraction commands and Svelte metadata:
   - Run `node "$RUN" extract "$ROOT" <input.json> <output.json>`, NOT the unwrapped bundled extractor. Pass all `batchFiles` fields and pre-resolved `batchImportData` unchanged.
   - Read `incremental-symbol-baseline.json`: **`files` is an array**, use `.find(f => f.filePath === path)`. Preserve still-present prior symbols and IDs; update their semantics and ranges from current source. Do not copy old semantics as a substitute for reading source.
   - Use `<skill-directory>/inspect.mjs "$ROOT" <relative-path>...` to obtain compiler-derived script scope, original ranges, `$props()` public names, aliases/rest and legacy `export let`, plus SvelteKit file roles. Inspect actual source for semantics. Module and instance scripts are distinct; same-name symbols are not interchangeable. A `scope: module` declaration needs explicit module ownership, never fake class containment just to satisfy validation.
   - SvelteKit roles: `+page.svelte`, `+layout.svelte`; universal `+page.ts/js`, `+layout.ts/js`; server `+page.server.ts/js`, `+layout.server.ts/js`; HTTP `+server.ts/js`. Read actual exports for `load`, actions and HTTP handler meaning. Strip route groups from URLs, retain parameter syntax. Filename roles are metadata, not automatic imports/calls.
   - Imports come from compiler-delimited scripts and existing TS resolution. Exact `.svelte` paths and explicit tsconfig aliases work. Never execute svelte.config.js. Do not invent local files for `$app`, `$env`, `./$types` or unresolved aliases. `$lib` requires an explicit tsconfig alias; inherited/generated config aliases are not resolved by this integration.
   - Output concise summaries/tags and supported graph nodes/edges. Keep props in file summaries/tags, not as callable nodes. Do not invent component call edges; source must support them.
7. Verify output exists for every batch, then run `node "$RUN" merge "$ROOT"` and read `incremental-symbol-report.json`. Nonzero exit blocks publication. A single targeted retry via wrapped `retry` is allowed by the upstream protocol; reanalyze affected files completely. Svelte deletions, unsupported declarations, scope changes and ambiguity remain conservative blockers—report them; never fill the graph with removed symbols or override unknown evidence. Plain retained/moved script functions now validate deterministically.
8. For `rerunArchitecture` / `rerunTour`, follow the installed Understand-Anything architecture/tour/lesson steps on the **merged full graph**. This refreshes architecture context, not every file's LLM analysis. Preserve layer names where possible. After writing `UA/intermediate/tour.json`, run `node "$RUN" normalize-tour "$ROOT"` and use `UA/intermediate/tour.normalized.json` for lesson dispatch and downstream tour references. A nonzero exit blocks publication; do not normalize by hand or call the plugin script directly. Stage lessons only for a tour actually rebuilt; no stale sidecar publication. For ordinary partial updates keep existing narrative/lessons, pruning orphan lessons after save.
9. Run `node "$RUN" finalize "$ROOT"`. The finalizer independently rechecks actual graph symbols before saving graph, fingerprints and meta. Only after success publish staged lessons if attempted, or remove stale lessons on a failed lesson rebuild per upstream rules. Run wrapped `prune`. Never publish partial evidence past a failing gate.
10. Report changed files analyzed, resulting nodes/edges, baseline commit, warnings, output path, and what was not checked. Do not auto-launch a dashboard or run a full graph reviewer unless requested.

## Safety and limits

- Svelte fingerprints deliberately stay content-only: template, style, prop and script changes all count as structural. This spends a little on the changed component rather than risking missed updates; it never triggers an LLM whole-project rebuild solely because many files changed.
- Strict evidence is produced by the existing TS AST declaration/coverage adapter over compiler-delimited source, not regex or LLM guesses. Module functions have a separate source owner. Local scope IDs remain disjoint. A conservative Svelte coverage gap prevents automatically approving removals where template/cross-script bindings may matter. Retained source identities still validate.
- Svelte call graph extraction is deliberately empty; merging module/instance call graphs could falsely bind same-name calls. Source-confirmed edges can be emitted by the analyzer.
- Scope-changing/missing symbols may require improved parser coverage or manual investigation. A full rebuild remains a user choice, never an automatic cost surprise.
- Plugin version upgrades require re-verification. This is an opt-in local skill; stock `/understand` and its commit hooks remain unchanged.

## Self-check

Run `node --test <skill-directory>/*.test.mjs` before using a modified adapter. Tests include compiler ranges, props, SvelteKit conventions, strict identity preservation, omitted symbols, scope changes, parse errors, local planner policy and end-to-end save gates.

# Understand Svelte incremental updates

## Goal

Provide a project-local skill for affordable Understand-Anything updates, including Svelte script blocks, Svelte 5 props and SvelteKit route conventions. Reuse an existing graph instead of rebuilding the entire project after ordinary component edits.

## Design

- Store the skill and tested helper scripts under `.claude/skills/understand-svelte/`.
- Reuse the installed Svelte compiler and Understand-Anything TypeScript analysis. Parse Svelte, do not identify script boundaries with regex. Preserve original file paths and source locations, distinguish module and instance scripts, and fail closed on unsupported syntax.
- Recognize `$props()` destructuring and SvelteKit `+page`, `+layout`, `+server` and associated load/server modules as semantic context, without treating filename conventions as proof of calls.
- Integrate deterministic source evidence into both merge-time and save-time symbol validation. Do not accept fabricated reports, suppress unknown symbols or copy old semantic nodes to satisfy validation.
- Allow incremental processing beyond the stock 30-file cutoff. Preserve missing-baseline and incompatible-format safeguards. Use local, version-checked integration rather than editing plugin cache files.
- Reuse the current pending batch outputs when they remain valid; graph, fingerprints and metadata only advance after validation succeeds.
- Expose the installed deterministic `normalize-tour.mjs` through the same version-checked wrapper for architecture updates; test path resolution, missing-input failure and no publication during normalization.

## Decisions

No new dependency, automatic hook, global settings change, whole-plugin fork or app runtime change. A local slash skill is sufficient. Existing dependencies must be available; report missing dependencies instead of downloading them silently.

## Risks

Plugin integration points can change; reject incompatible versions. Svelte module/instance scopes and TS declarations must not become ambiguous after extraction. Template or CSS changes must not be mistaken for cosmetic changes if they affect graph semantics. SvelteKit is fixture-tested here: this app uses Svelte with Vite, not SvelteKit.

## Release criteria

Tests cover TS/JS scripts, module scripts, props aliases/rest, source ranges, unchanged/deleted/shadowed symbols, invalid input, imports and SvelteKit routes. A changed Svelte file validates using deterministic parser evidence at merge and save. Existing pending updates can publish without a whole-project LLM rebuild, or the task records the exact remaining blocker. All task/index progress reflects executed checks.

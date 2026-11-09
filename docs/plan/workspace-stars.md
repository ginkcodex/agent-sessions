# Workspace stars

## Goal

Let users mark important workspaces and quickly show only those workspaces from a star button beside the sidebar directory filter.

## Design

- Directory rows have an always-visible star toggle at the far right, after the session count, so showing the hover actions menu cannot shift its position. Filled/accent stars indicate saved workspaces; toggling never selects or expands a row.
- A star filter beside the directory search toggles the visible navigation tree only. It composes with the existing backend filters and does not close the current session or change navigation/collapse state.
- Save stars as client-side preferences in localStorage, identified by data-host alias and workspace path, not display label or group key. Local, WSL and SSH identities are separate. Windows-style paths are case-insensitive and separator-normalized.
- Filter directory-first and agent-first trees without mutating the catalog. Keep matching directory subtrees and necessary agent ancestors, recalculating ancestor session references/counts. Flat mode matches session cwd and retains matching families/context ancestors.
- The new-workspace/no-sessions panel in the second column includes a star toggle beside its workspace information. It uses the same persisted host/path preference, reflects existing stars and respects blocked links. Starring reveals the workspace in the starred-only sidebar without clearing that filter or navigating away.
- When no stars match, explain the empty result and offer a button to show all workspaces. The filter itself is transient and starts off on application launch.

## Decisions

- This is a local UI preference, not a host-side workspace registration change; discovered and explicitly opened workspaces can both be starred, including missing directories.
- Do not reorder the default workspace list or modify session history/files.
- Stars survive temporary discovery/filter absence and unregistering a workspace. Only explicitly unstarring removes the preference.
- Storage failures leave the current in-memory interaction usable; malformed saved values are ignored.

## Risks

- Paths and host aliases are identities: alias or directory renames do not migrate stars automatically.
- Filtering must retain group structure, matching descendants and consistent ancestor counts without leaking unrelated workspace rows.
- Nested buttons must stop click/keyboard propagation so a star action cannot navigate.

## Release criteria

- Automated coverage for persistence, host isolation, malformed/unavailable storage, path identity, all grouping modes, empty results, row controls and filter accessibility.
- Frontend tests, Svelte/TypeScript check and production build pass. Record actual checks and any native/manual gaps in WS-01.

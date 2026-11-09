# Workspace terminal selection

## Goal

Clicking a directory (or an agent/topic row under it) in the workspace tree opens the first visible session. The terminal panel should then show that session's tab, exactly as a direct click on the session row does, instead of the first agent tab launched in the directory.

## Current behavior

- `SessionList.handleSelectSession` calls `terminals.selectDirectory(session.cwd, host, session)`, so the session's own tab (rank 0) wins.
- `GroupTree.handleSelect` calls `terminals.selectDirectory(cwd, host)` with the row's directory and **no session**, before the sessions load. Afterwards it selects `sessionToSelect(node, visible)` and never tells the terminal panel.
- So rank 0 can never apply. The first agent tab launched in the directory wins, even when it runs another session. A tab resumed with a different or empty launch directory is missed entirely. A matching agent tab that is already active stays, even when it runs a different session.
- The transcript only corrects the terminal when the user later clicks or focuses it (`TranscriptView.activateWorkspaceTerminal`).

## Design

- After the group's sessions load and the session to open is chosen, the tree computes the terminal target with a pure helper in `tree.ts`. The target is the chosen session's meta and its `cwd`. The helper falls back to the row's directory when the session's directory is unknown. When no session is visible, the target is the row's directory alone.
- The tree calls `terminals.selectDirectory(target.dir, host, target.session)` once, only if the click is still current and the host is unchanged. That is the same call a session-row click makes.
- Remove the early meta-less call, so the panel does not first switch to a wrong tab and then correct itself.
- `TerminalsStore.selectDirectory` ranking is unchanged. Subagents still resolve to their parent's tab through `parentId`.

## Decisions

- Keep the ranking in the store. The bug is in what the tree passes it, not in how tabs are ranked.
- The terminal switch waits for the session list. It is normally immediate. With a slow list, the old tab stays until the session is known, which is better than showing a wrong one first.
- A chosen session with `cwdMissing` still passes its identity, with the row's directory, so its own tab can win. A direct session-row click skips the terminal for such sessions. The tree has a directory to use, so it does not need to.

## Risks

- Superseded clicks or host switches must not activate a tab for the old click. The existing group-key check and a host check guard this.
- `sessionToSelect` returns the first row of the flat sorted list. With a non-default sort that is not the newest session. The terminal follows whichever session is opened, which is the requirement.

## Release criteria

- Helper unit tests cover: the session's own directory and identity, the fallback to the row's directory, an empty list, and no directory at all.
- A store-level regression test shows that the tree target selects the opened session's tab over another agent tab of the same directory, and a subagent's parent tab.
- The source wiring test asserts the tree's new call and that the meta-less early call is gone.
- Terminal and tree tests, the frontend check and the full frontend suite pass. Live desktop clicking is reported separately.

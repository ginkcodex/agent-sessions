# TFL-02 — Check tool file links in the running app

- **Status:** ⬜ Not started
- **Estimate:** 1 h
- **Depends on:** TFL-01
- **Plan:** [tool-file-links.md](../../plan/tool-file-links.md)

## Scope

Click a file tool's path in the app and record what happens. Code changes
only if something is wrong.

## Acceptance criteria

- Mock app (`vite` dev server, a browser): in a mock chat, the live `Read`
  card's `README.md` path opens the file in the file viewer.
- Desktop app, a real Claude Code session, Activity and All:
  - an Edit or Write path inside the session's folder opens that file;
  - a Read with `offset` opens at that line;
  - a path outside any known folder shows the "Could not open message file"
    toast;
  - a long path ends in an ellipsis and the status badge stays in place;
  - Tab reaches the path and Enter opens it.
- One OpenCode session's edit card, if one is at hand.

## Tests

Manual. Record what ran, and on which host, in Evidence.

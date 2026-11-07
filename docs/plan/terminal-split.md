# Split terminals

Planned 2026-10-25. Tasks: [terminal-split](../tasks/terminal-split/README.md).

Request: show two or more shells at once in the built-in terminal, as VS
Code's terminal does.

Model: a tab is a group of one to four panes; each pane is one terminal (one
PTY), as every tab is today. The backend is unchanged. `terminals.tabs` stays
the flat list of panes in start order, so session links, attention, alerts,
`tabFor`, `show(id)` and closing keep working on pane ids; `active` is the
focused pane. Groups (`terminals.groups`) own the tab-strip and pane order and
are not persisted (terminals do not survive a restart).

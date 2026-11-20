# Compact port forwarding rows

## Goal

Fit many forwarding rules in the Port Forwarding dialog. Each rule used three
lines (status, route, a row of text buttons), so only about five fit.

## Design

- One line per rule: status, `127.0.0.1:<local> → <host>:<port>`, an "Auto"
  badge when the rule auto-starts, then the actions as icon buttons on the
  right.
- Actions: open in browser, copy address, start/stop (Retry/Cancel by state),
  edit, remove. Each icon button has an `aria-label` naming the rule and a
  tooltip with the action. Enable/disable rules are unchanged.
- The "This computer" / "Remote host" captions become tooltips on the
  addresses; the intro text already says which side is which.
- A rule's error shows under its row.
- Icons come from the shared Lucide set (`icons.ts`): external-link, copy,
  play, pencil, trash-2 added; square and x reused.

## Decisions

- Icons over text so the row fits on one line in the 760 px dialog. Narrow
  windows wrap the action group to a second line instead of overflowing.
- No confirm on Remove, same as before.

## Risks

- Icon-only buttons are less discoverable; tooltips and accessible names
  cover it.

## Release criteria

- Every rule is one line at the default dialog width.
- All actions still work and are reachable by keyboard with a name.

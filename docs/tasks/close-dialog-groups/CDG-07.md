# CDG-07 — The host-switch tabs question reads like Close window

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** CDG-02, CDG-03
- **Plan:** [close-dialog-groups.md](../../plan/close-dialog-groups.md)

## Scope

Asked in use: disconnecting from a host with terminal tabs showed
"Terminal tabs from previous host" with a plain paragraph ("4 terminal
tab(s) belong to cos…"), while closing the same window lists the tabs in
a Terminals card. The disconnect question should show the same content.

- `closeConfirm.ts`: `hostTabsWarning(tabs, host, chats, action)` returns a
  `CloseWarning`: the "Close window?" counts sentence with the host ("on
  cos", "on this computer"), ended tabs counted apart, the open chats
  sentence, and a closing clause on the keep-or-end choice. Cards:
  Terminals (running tabs first, ended ones badged "ended"; six named,
  the rest summed up) and Chats when chats are open. The running-count
  sentence is shared with `closeWindowWarning`.
- `connection.svelte.ts`: the question uses it, titled "Disconnect from
  <host>?" or "Connect to <host>?"; the choices are unchanged.
  `switchAction` names the switch for both this and "End open chats?".

## Likely files

- `frontend/src/lib/closeConfirm.ts`
- `frontend/src/lib/stores/connection.svelte.ts`
- `frontend/test/close-confirm.test.mjs`, `frontend/test/connection.test.mjs`

## Acceptance criteria and tests

- Disconnecting from a host with running tabs shows the counts sentence
  and a Terminals card naming each tab with its host, as "Close window?".
- Ended tabs and open chats are told apart; the keep and close choices
  and what they do are unchanged.

## Evidence

- `frontend/test/close-confirm.test.mjs`: two new tests, covering the
  four-tab example from the report (sentence and card rows), this
  computer and WSL hosts, ended tabs (sentence, badge, order), the chats
  clause and card, and the six-row cap.
- `frontend/test/connection.test.mjs`: the keep/close test now checks the
  title, the sentence, that only the previous host's tab is in the card,
  and the labels for every connect and disconnect pair. The combined test
  checks the whole message and both cards.
- `npm test`: 1411 tests, 1410 pass, 0 failures, 1 skip that was already
  there. `npm run check`: 0 errors, 0 warnings.
- Not checked: the desktop app (covered by CDG-04's QA, still not started).

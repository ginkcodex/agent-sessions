# Conversation PDF printing

## Goal

Add **Print conversation to PDF…** to the session-content header's three-dot
menu. Print the entire selected session at Chat level, including messages not
currently loaded in the transcript viewer. Preserve single-message printing.

## Design

- Reuse the existing single-message PDF renderer and native save flow.
- Fetch the full conversation independently of the viewer's paged/rendered
  window. Do not change the reader's scroll position, filters or loaded window.
- Apply the same Chat-level inclusion rules as the transcript: user/assistant
  conversation content, without reasoning, tool calls or tool output.
- Preserve chronological order, message boundaries, roles, Markdown, code and
  existing print styles. Identify the session in the document title.
- Disable duplicate print requests while preparing/saving. Report failures
  through the existing error UI; never silently print an incomplete fetch.
- Capture the selected session when the action starts, so switching sessions
  cannot mix their messages or metadata.

## Decisions

- The action always prints Chat level, regardless of the viewer's current
  detail level or search filter.
- Printing is read-only with respect to session stores, for local and remote
  sessions alike. No new PDF dependency or remote protocol is intended.
- Keep native save cancellation consistent with single-message printing.
- Load forward in size-capped pages from index zero. Require matching revision
  and total count across pages; if the session changes during preparation,
  report that they should try again rather than produce a mixed document.
  Older hosts without revisions still require a stable total count.
- Use role headings in the existing editable parts job, so one conversion
  numbers image attachments across the entire conversation. Preserve displayed
  translations (without initiating any) and manual Bash formatting.
- Cancel preparation silently on session/host changes or component teardown.

## Risks

- Virtualization and pagination can otherwise omit most of a conversation;
  tests must cover fetching beyond the initial window.
- Provider messages can mix visible text with tools/reasoning; filtering must
  use the shared Chat-level rules rather than roles alone.
- Very large conversations cost memory and PDF rendering time, as a full
  document is necessarily assembled. Preparation must show a busy state.
- Native PDF output differs by OS; automated rendering/flow tests do not
  replace a native visual/save-dialog check.

## Release criteria

- Header menu exposes the action with appropriate empty/busy guards.
- All Chat-level messages are printed in order, independent of loaded pages,
  current detail/search settings and subsequent session selection changes.
- Single-message PDF printing remains compatible; Markdown is sanitized and
  controls/tool details do not leak into the document.
- Automated tests cover filtering, full loading, rendering, failures and menu
  wiring. Run frontend tests, type checks, build and the repository test target.
- Record native checks separately and do not claim checks that were not run.

Tasks: [conversation-pdf](../tasks/conversation-pdf/README.md).

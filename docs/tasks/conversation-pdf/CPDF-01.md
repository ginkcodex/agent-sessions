# CPDF-01 — Print the entire Chat-level conversation

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** —
- **Plan:** [conversation-pdf.md](../../plan/conversation-pdf.md)

## Scope

Reuse the message PDF export flow for the full selected session. Fetch messages
outside the loaded viewer window, filter to shared Chat-level content, render
chronologically with roles, and add the action to the session header menu.
Handle empty sessions, in-flight requests, cancellation and errors without
changing transcript state or mixing sessions.

## Likely files

- `frontend/src/lib/conversationPrint.ts`
- `frontend/src/lib/components/transcript/{SessionHeader,TranscriptView}.svelte`
- `frontend/test/conversation-print{,-menu}.test.mjs`

## Acceptance criteria

- **Print conversation to PDF…** is available in the session-content three-dot
  menu and prints all Chat-level content, not just loaded/filtered messages.
- User and assistant text retain chronological order, Markdown and code;
  reasoning, tool calls/results and other non-Chat content are omitted.
- Existing single-message PDF behavior is preserved.
- The original session is captured before asynchronous work; repeated clicks
  are guarded and errors are visible. Failed fetching does not save partial PDF.
- Empty Chat content is handled clearly, and native cancellation is not an error.

## Tests

Add focused regression tests for Chat filtering, pagination/full fetch,
conversation document rendering, failure/busy behavior and menu wiring.
Run frontend tests, Svelte/TypeScript checks, production build, `make test`
and lint. Document native UI/save-dialog checks separately.

## Evidence

- Pulled `origin/develop` to `b95f03e` before feature code changes; preserved
  both upstream feature-index additions and this feature's planning docs.
- Added a header menu callback and preparation guard. Full conversation fetches
  start at zero, follow actual capped-page lengths, reject changed snapshots or
  incomplete fetches, and do not modify the viewer window/search/detail mode.
- Shared Chat filtering retains text/files/error notices and omits tools,
  thinking and meta/system records. Role headings share one attachment-number
  namespace. Manual Bash formatting and displayed translations are preserved.
- Reused the existing editable print preview, Markdown sanitization, image
  loading, paper styling and native print/PDF dialog; no new backend protocol.
- New regression tests: 13/13 passed. Includes 123-message pagination, multiple
  image pages, hidden-only pages, legacy no-revision responses, changing
  revisions/counts, gaps/empty pages, empty Chat content, error reporting,
  duplicate-click prevention, busy/empty/locked menu states and cancellation
  on session/host changes or teardown. View-flow tests compile its actual
  preparation code in a minimal Svelte host; header tests click the real menu.
- First full frontend run found a source-assertion collision in an existing
  pagination test (it counted the new delegating `api.getMessages` call).
  Changed delegation to a bound method; the pagination assertions now pass
  without altering the test.
- Full frontend suite: 1,192 passed, 1 skipped, 0 failed.
- `make test` on updated develop (`go test -race ./...`): passed.
- `npm run check`: 0 errors, 0 warnings.
- `npm run build`: passed; existing Vite large-chunk warning remains.
- `make lint`: failed with 135 findings in unchanged Go files, including
  unchecked `fmt.Fprintf` results in `cmd/agent-sessions-cli/export.go`.
  This task changes no Go files.
- Native desktop/save-dialog and actual multi-page PDF visual testing were
  not performed. Automated tests do not establish native pagination quality.

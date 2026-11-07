# Transcript pages capped by size

Planned 2026-11-07. Tasks: [transcript-page-cap](../tasks/transcript-page-cap/README.md).

Report: on the remote host `plgl`, project `/home/haitran/Workspaces/be`,
opening a Codex session sometimes shows the transcript spinner noticeably
longer than for a Claude Code or OpenCode session.

## Cause

The server is not the slow part. `TestMeasureTranscriptLoad`
(`internal/provider/all/measure_load_test.go`) on `plgl`, `be`, read only:

| Provider | Slowest first load | Median first load | Newest 50 messages as JSON |
|---|---|---|---|
| Claude | 284 ms (24 MB file) | ~10 ms | 0.05–0.2 MB |
| Codex | 199–237 ms (19 MB file, 85 messages) | ~90 ms | 0.44–1.29 MB |
| OpenCode | — | — | up to 1.56 MB in some sessions |

The difference is what the viewer's first request returns. It asks for the
newest 50 messages (`PAGE_SIZE`, `TranscriptView.svelte`). A Codex message is
a whole turn with all its tool calls, so 50 Codex messages are most of a
session, and their tool output (0.96 MB of the 1.29 MB page) all travels in
the first reply. Claude splits turns into many small messages, so its 50 are
small. Over SSH, without compression, 1.3 MB from `plgl` takes about
150–240 ms beyond the connection's own latency, then the frontend parses and
renders all 50 messages before the spinner goes.

## Goal

The first page of any transcript is small enough to arrive and render
quickly, whatever the provider: at most 50 messages *and* at most about
256 KB, always at least one message. Earlier and later messages still load
as the reader scrolls, as today.

Measured with a 256 KB budget (JSON of each message) on the same sessions:

| Provider | Largest single message | First page with the cap |
|---|---|---|
| Codex | 0.25 MB | 4–32 messages, ≤ 0.25 MB (was up to 1.29 MB) |
| Claude | — | unchanged: 50 messages, ≤ 0.13 MB |
| OpenCode | up to 0.29 MB | 9–50 messages, ≤ 0.25 MB |

## Design

### Engine (`internal/engine`)

`pageOf` takes an optional cap:

```go
// PageCap limits a page by size as well as by count.
type PageCap struct {
	// MaxBytes is the page's size budget (model.MessageBytes); 0 = no cap.
	MaxBytes int64 `json:"maxBytes,omitempty"`
	// Anchor is the transcript index of the message the cap keeps. The page
	// grows from it, within [offset, offset+limit), while the next message
	// fits. -1 means the last message of the page.
	Anchor int `json:"anchor,omitempty"`
}
```

The uncapped page is chosen first, exactly as today (offset -1 is the newest
`limit`). With a cap, the anchor is clamped into the page, then the page grows
outward from it, one message before and one after in turn (only before for
anchor -1, only after when the anchor is the page's first message), while the
total stays within `MaxBytes`. The anchor message is always kept, even alone
over budget. The page reports its real `Offset`, `Limit` (message count) and
`HasMore`, plus a new `Capped bool` (`json:"capped,omitempty"`) when the cap
dropped messages.

`model.MessageBytes` is the size: already used by the transcript cache,
cheap, and close to the JSON size for text-heavy messages (JSON escaping
adds a little; the budget is a target, not a limit to the byte).

`Service.GetMessagesCapped(ctx, ref, offset, limit, cap)`, `Engine` and the
`Backend` interface gain it; `GetMessages` stays and means "no cap".

### RPC and app

- `GetMessagesParams` gains `Cap *engine.PageCap` (`json:"cap,omitempty"`).
  The server's `getMessages` case passes it on. An older server ignores the
  unknown field and returns the full page, which the client already handles,
  so no capability is needed. An older client sends no cap.
- `rpc.Client.GetMessagesCapped` sends the field; `offlineBackend` keeps its
  error.
- `App.GetMessagesCapped(ref, offset, limit, cap)` is a new Wails binding.
- The delta request (`getMessagesSince`) is not capped: a refresh must keep
  the window the reader has.

### Viewer (`TranscriptView.svelte`, `api.ts`)

`api.getMessages(ref, offset, limit, cap?)` uses `GetMessagesCapped` when a
cap is given and the binding exists, else `GetMessages`. The mock API
implements the cap with the same rules.

| Request | Cap anchor | After the reply |
|---|---|---|
| First load and back to bottom (`-1, 50`) | -1 (newest) | window as today |
| Earlier (`start, end-start`) | `end-1` (next to what is shown) | `pageOffset = page.offset`, not `start`; prepend |
| More (`end, limit`) | `end` | append, `hasMore = page.hasMore` |
| Search jump (`jumpOffset(i), 50`) | `i` (the match) | window as today |

Budget `PAGE_BYTES = 256 * 1024` beside `PAGE_SIZE`.

A live refresh today asks for `max(window length, 50)` messages. After a
capped first page that would fetch the dropped messages back on the first
refresh. When the window came from a capped page it asks for the window's
own length instead (the window slides forward with new messages, and the
delta still reuses what the client holds). `fillViewport` keeps loading
earlier pages while the view is too short, so a page of a few small messages
still fills the screen.

## Decisions

- **Cap by size on the server.** The server knows the messages' sizes before
  sending them; the client does not. A count alone cannot fit providers whose
  messages differ in size by two orders.
- **Opt in per request.** Only the viewer's paging asks for the cap; the
  CLI, exports and refreshes keep full pages.
- **Anchor instead of a direction.** One field covers the newest page, both
  scroll directions and search jumps, which keep the match itself.
- **256 KB.** Measured: it keeps every Claude first page whole and cuts the
  largest Codex and OpenCode pages to a fifth. A smaller budget saves little
  more, since single messages already reach 0.25 MB.
- **No SSH or RPC compression now.** SSH compression is per connection; on
  macOS and Linux the app's multiplexed connection to a host also carries
  terminals, file transfers and the server deploy, most of it already
  compressed. Compressing large RPC replies in the app is the targeted
  alternative; it waits for measurements after the cap.

## Risks

- **More round trips** when scrolling up through large messages: each
  earlier page is smaller. Each is also faster, and the reader sees one
  screen at a time.
- **`MessageBytes` under the JSON size** for output with many escaped
  characters: a page can be somewhat over 256 KB. Acceptable; the test
  records the real JSON size.
- **Older servers** return full pages until their bundle is redeployed;
  nothing breaks.
- **Window offset bugs** if a caller still assumes the requested offset;
  `loadEarlierMessages` is the one place that does today.

## Release criteria

- Go tests for the cap: newest, earlier, later and jump anchors; one message
  over budget; no cap; `Capped`, `Offset`, `HasMore`; RPC round trip with and
  without the field.
- Frontend tests: each request passes its anchor; earlier pages use
  `page.offset`; a capped window's refresh does not refetch dropped messages;
  mock cap matches.
- `TestMeasureTranscriptLoad` on `plgl`, `be`, after: Codex first pages
  ≤ ~0.3 MB.
- In the desktop app on `plgl`, `be`: a large Codex session opens without the
  long spinner, scrolling up loads the rest, a search jump lands on its match,
  a live session keeps following.

# Go to File — lightweight file editing

Status: FE-01–FE-12 implemented on branch `editor` (commits `11ca1cd`,
`82e00ca`, `6e5581a` and the release-review fixes). Editing is exposed in the
UI. Automated regression coverage is in place and the release review (FE-14) is
done, with its findings fixed. Manual desktop and Linux/Windows/SSH/WSL QA
(FE-13) has not been performed and blocks release.

## Goal and scope

Add an explicit Edit mode to Go to File for quick project-file changes, not a
full IDE. Keep the existing source viewer, Markdown/image previews, and Git diff
as the default read-only experience.

First version includes:

- Existing UTF-8 text files, including empty files, Markdown, and SVG source.
- CodeMirror 6: syntax highlighting, line numbers, indentation, undo/redo,
  find, and word wrap. Lazy-load the editor and language support.
- Explicit Save and Discard actions (both leave edit mode); Cmd+S / Ctrl+S.
- Dirty-tab indicators, retained drafts, and guarded destructive transitions.
- Revision-checked saves and an explicit conflict-resolution flow.
- Local, WSL, and SSH access; older remote servers remain read-only.

Revised by user decision: there is no separate Done action. Save and Discard
both leave edit mode; Cmd/Ctrl+S saves and keeps editing.

Out of scope: new files, rename/delete, Save As, autosave, disk-backed draft
recovery, merge editor, filesystem watching, LSP/autocomplete, formatting, and
diagnostics. Binary/image/oversized files remain read-only.

Follow-up scope: [code-diagnostics.md](code-diagnostics.md) proposes project
checks and live language-server diagnostics as a separate staged feature. The
exclusions above describe this editing release, not that follow-up.

Drafts stay in memory, not local storage or another persistent cache: project
files may contain secrets. Normal close/quit must be guarded, but a crash or
forced termination can lose drafts. Do not log file bodies.

## Current integration points

- `frontend/src/lib/components/common/GoToFileDialog.svelte`: dialog lifecycle,
  Escape handling, lock, presentation mode, and file picker.
- `frontend/src/lib/components/common/go-to-file/FileViewer.svelte`: current
  viewer, previews, Git diff, find, and toolbar.
- `frontend/src/lib/components/common/go-to-file/FileTabs.svelte`: tab close,
  middle-click, and bulk-close context actions.
- `frontend/src/lib/stores/files.svelte.ts`: tabs, asynchronous reads, refresh,
  and per-directory saved state. Currently refresh and reopen reread tabs;
  connection changes call `files.reset()`. These must become draft-aware.
- `frontend/src/lib/api.ts` and `frontend/src/lib/types.ts`: frontend contracts.
- `internal/files/files.go`: bounded UTF-8 reads and root-contained resolution;
  no write operation exists. Current text-read limit is 5 MiB.
- `internal/engine/{backend,service,engine}.go`: backend interface and routing;
  the service restricts reads to known session directories.
- `internal/rpc/` and `internal/app/`: remote transport, desktop bindings,
  capabilities, offline behavior, and connection lifecycle.

## Behavioral contract

| Action | Behavior |
|---|---|
| Edit | Load the editor and promote a temporary preview tab to a kept tab. |
| Switch tabs | Retain draft, undo history, cursor/selection, and scroll position. |
| Save (button) | Submit an explicit snapshot, then return to viewer. A failed or conflicted save, or typing during the save, keeps the editor open. |
| Cmd/Ctrl+S | Submit an explicit snapshot and stay in edit mode. |
| Type during save | Allow typing; edits after the submitted snapshot remain dirty. |
| Discard (button) | Confirm when dirty, drop the draft and return to viewer; reread from disk after a conflict or uncertain save. |
| Reload (error bar) | Confirm when dirty, reload from disk, and reset editor history; stay in edit mode. |
| Close a tab | Dirty files require Save / Discard / Cancel. |
| Bulk-close tabs | Coordinate one flow; no overlapping prompts or unawaited saves. |
| Refresh | Refresh the listing and clean tabs; never replace dirty drafts. |
| Hide the dialog | Preserve drafts in memory; reopen must not reread over them. |
| Change directory | Preserve drafts in the existing per-directory cache. |
| Switch host / clear state | Guard any operation that would discard drafts, including cached directories. |
| Disconnect | Keep drafts; block saves until the correct host is available. |
| Quit / close window | Guard normal exits with unsaved drafts. |

Editor shortcuts take priority while editing. Cmd/Ctrl+F uses CodeMirror search;
Alt+Z uses the existing wrap preference. Escape may close editor search but must
not discard a draft. Preserve the dialog-lock behavior. Presentation mode and
rendered previews remain read-only; leaving edit mode requires the same dirty
check as Discard.

After save, update the saved baseline, size/revision, Git listing status, and
saved-file diff. Do not refresh away drafts in other tabs. Git markers describe
saved disk content, not speculative unsaved changes.

## Save and conflict contract

Finalize exact exported names in FE-01 before implementation. Intended shape:

```ts
saveFile(dir, path, text, expectedRevision)
```

- A revision is an opaque hash of the original file bytes, not mtime/size alone.
- The save request requires a revision; no unchecked force-write flag.
- The backend validates the known directory, containment, existing regular-file
  status, UTF-8/text eligibility, and input/output byte limits independently of
  frontend controls. Account for JSON expansion and RPC frame limits.
- Initial edit limit: 500,000 UTF-8 bytes, below the viewer's 5 MiB read limit.
  Viewing larger text still works. Enforce the same edit limit in backend and UI.
  Later raised to the read limit, 5 MiB ([large-file-view](large-file-view.md),
  LFV-05).
- Preserve executable permissions, UTF-8 BOM, uniform LF/CRLF style, and the
  intended final newline. Do not normalize bytes merely by entering edit mode.
  Mixed line endings and unsupported newline conventions stay read-only in v1.
- Refuse writes through symlinked child paths in v1 (including directory
  components). A canonicalized project root itself may be a symlink. Audit
  Windows junction/reparse behavior and hard links before enabling writes.
- **Revised by user decision:** write in place, not by replacement. The user
  selected this after the strict replacement gate refused macOS provenance
  attributes. Open the existing inode without truncation; write the snapshot,
  truncate to its final length, then sync. This preserves inode identity and
  existing ownership, permissions, ACLs, streams and extended attributes without
  copying them. Hard links are allowed: all aliases see the write, including
  aliases outside the project. A crash, full disk or I/O failure can leave partial
  contents; do not claim atomic saves or rollback. Keep the frontend draft on
  failure. No-op saves do not write or change mtime.
- Serialize writes to a target within this process. Use race-resistant path
  handling where available; resolving a pathname once is not a complete defense
  against symlink substitution.
- Return a structured success/conflict result that survives RPC and Wails;
  do not identify conflicts by matching error-message strings.

On conflict, retain the draft and offer Reload from disk, Overwrite…, or Cancel.
Reload requires confirmation before losing the draft. Overwrite requires an
explicit confirmation and retries against the revision just observed. If that
revision changes again, show another conflict. Deleted/replaced/ineligible files
are not silently recreated or overwritten.

Revision checks protect against ordinary stale saves, but cannot exclude an
unrelated writer in the final check/replace interval. Document this limitation;
do not claim cross-process compare-and-swap or locking guarantees.

A disconnected save can have an unknown outcome. Do not automatically replay it
or mark the draft clean. On reconnect, reread the correct target to reconcile:
if its bytes equal the submitted snapshot, acknowledge that snapshot as saved;
otherwise preserve the draft and require the normal revision/conflict flow.

## Task breakdown

Tasks FE-01…FE-14, their progress and the implementation findings are in
[the file-editing task group](../tasks/file-editing/README.md).

## Acceptance scenarios

1. Edit an empty file or a source/config/Markdown/SVG text file, save locally or
   remotely, and verify exact disk bytes and updated Git status.
2. Switch tabs/directories, hide/reopen the dialog, and refresh without losing
   draft text, undo history, cursor, or selection.
3. Close one/many dirty tabs or attempt host switch/quit: Save / Discard / Cancel
   works; save errors never discard the remaining drafts.
4. Let another process modify the file: ordinary save refuses the stale
   revision; reload and confirmed revision-checked overwrite both work.
5. Type during a save: successful response acknowledges only the submitted
   snapshot, leaving subsequent edits dirty.
6. Disconnect during a remote save: preserve the draft, never write locally,
   and reconcile the result after reconnect without blindly replaying it.
7. Read-only, oversized, unsupported-encoding/newline, link, and protected files
   remain viewable where previously supported but cannot bypass backend checks.
8. Older remote servers still support viewing without advertising editing.

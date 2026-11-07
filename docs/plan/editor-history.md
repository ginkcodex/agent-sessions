# Editor back and forward

Planned 2026-11-07. Tasks: [editor-history](../tasks/editor-history/README.md).

## Goal

In Go to File, Back and Forward return through the files and places the
user went, per workspace (project directory), as the session list does for
sessions. The mouse's side buttons, the keyboard's Browser Back/Forward keys,
Alt+←/→ (⌘[ / ⌘] on macOS) and the viewer's ← / → buttons all step it.

Places recorded:

- another file shown: picked in the tree or the file search, a tab clicked
  or selected with Cmd/Ctrl+1–9, a search match or problem in another file,
  a file link from a session or terminal;
- a Markdown link followed in the preview;
- Go to Definition and Find References results (today's code-navigation
  history, which this replaces).

## Today

`FileViewer.svelte` keeps a `SourceHistory` (`sourceNavigation.ts`) for code
navigation only. It lives in the viewer and is cleared whenever `files.dir`
changes, which includes closing the dialog (`files.close()` sets `dir` to
null) and switching to Git History (the viewer remounts). Going back always
reveals a range in the source, which turns the Markdown preview off. The
mouse's side buttons are cancelled by `App.svelte` and do nothing while a
modal (Go to File is one) is open.

## Design

### Places

`SourceLocation` becomes `EditorPlace`:

```ts
interface EditorPlace {
  path: string;
  external?: boolean;          // outside the project, read-only
  range?: CodeDiagnosticRange; // the caret or selection, when there is one
  preview?: boolean;           // the Markdown or SVG preview was shown
}
```

Where the user is (`currentLocation` in the viewer), in order:

1. the preview is shown: `{ path, preview: true }`;
2. the editor's caret, or the selection in the source view: with `range`;
3. the last place navigation landed in this file;
4. otherwise the file as shown: no `range`.

`SourceHistory` keeps `EditorPlace`s, with the same rules as today (visit
drops forward, consecutive duplicates merge, 50 per side), plus `remap` for
renames.

### Per workspace, in the files store

`FilesStore` holds one `SourceHistory` per directory, beside `saved`, so it
survives closing the dialog, switching projects and the Git History layout.
At most 20 directories are kept, least recently shown dropped first.
`reset()` (host switch) drops them all, as it drops `saved`. Rename remaps
the places under the renamed path; delete drops them.

The store records a place itself when the shown working tab changes:
`openFile` and `selectTab` ask the viewer for where the user is
(`setPlaceReader`) before switching, and visit it. Not recorded:

- the same tab shown again;
- leaving from or to the Git History layout, a Git review (`gitBasis`) or a
  comparison: those are not places in the working tree;
- closing a tab (its neighbour is shown), restoring a project, a rename;
- a step through history itself, and opens that record their own place
  (`withoutHistory`).

Go to Definition records the place it was asked from, as today, then opens
without the store recording again. Jumps inside one file are recorded only
for definitions and references (as today); search matches and problems in
the same file are not, so stepping with F4 does not fill the history.

### Going back

A place with `range` opens as today: the tab (preview tab unless one is
open), then the range is revealed and selected, the preview off. A place
without `range` opens the tab and lets the viewer's per-tab scroll memory
restore the view; for a Markdown or SVG file the preview is turned on or
off as `preview` says. Paths not valid in the project are skipped, as are
deleted ones (already dropped on delete).

### Mouse buttons

`GoToFileDialog.svelte` listens for side buttons while it is the only open
modal: the DOM's buttons 3/4 on `mouseup` (WebView2, browsers) and the
native `nav:mouse` event (`subscribeMouseNavigate`, Linux and macOS). App
keeps cancelling the webview's own navigation on every side button and
still ignores them while a modal is open, so a press does one thing. With
a prompt, Settings or another dialog over Go to File, nothing happens.

## Decisions

- Tab switches count as navigation, as in VS Code's Go Back.
- History is kept per directory for the app's run, not across restarts.
- Same-file search and problem jumps are not recorded (see above).
- A step reopens a closed file in the preview tab, as code navigation does.

## Risks

- Reading the place on every switch: `currentLocation` is cheap (one
  selection lookup), but must never throw into `openFile`; the store
  guards the call.
- The scroll memory for a reopened, previously replaced preview tab is
  per path (`scrollTops`) and may land at the top for source files read
  again; acceptable, the file is right.
- Native side-button delivery is untested on real hardware on Linux and
  Windows (TECH-NOTES); the dialog uses the same paths as the session list.

## Release criteria

- Picking files, switching tabs, following a Markdown link and Go to
  Definition can each be undone with Back and redone with Forward, from the
  keyboard, the toolbar and the mouse's side buttons.
- Back to a Markdown file left in preview shows the preview.
- History of project A is still there after closing the dialog, opening
  project B and coming back to A; B's history is its own.
- `npm test` and `npm run check` pass.

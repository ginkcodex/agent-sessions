# Code completion in the editor

Status: planned 2026-11-18. Task briefs are in
[docs/tasks/code-completion](../tasks/code-completion/README.md) (CC).

## Goal

Suggest code while typing in the Go to File editor, as VS Code and Zed do:
a popup with names that fit at the cursor, accepted with Enter or Tab. The
[editor essentials](editor-essentials.md) plan left completion out of scope;
this plan adds it.

There are two levels, and the first is useful without the second:

1. **Without a language server.** Words from the open file, plus what
   CodeMirror's language packages already know (JS/TS local names and
   keywords, CSS properties, HTML tags and attributes). Works in every
   project and on every host, with no approval and no server.
2. **With the project's language server.** Members after `.`, imported and
   project symbols, signatures and documentation, from the servers that code
   checks already run (gopls, Pyright, vtsls, rust-analyzer, Svelte,
   Intelephense, JDT LS; ruby-lsp after CC-08).

Out of scope: inline "ghost text" and AI completion, signature help while
typing arguments, auto-closing brackets, rename, code actions, and running
any command a completion item carries.

## Decisions

- **The popup opens while typing**, like VS Code and Zed. To open it by
  hand (setting off, after Escape, or after a space or `(`): Cmd+I on
  macOS, Ctrl+Space on Windows and Linux. macOS keeps Ctrl+Space as a
  second key, but the system usually takes it to switch input sources.
  CodeMirror's macOS defaults, Option+\` and Option+I, are not bound: on
  the US layout they are dead keys that start an accented letter, and they
  would interfere with Vietnamese input. If the macOS web view takes Cmd+I
  as italic before the editor sees it, Option+Esc is used instead (VS Code
  binds both).
- **Enter and Tab accept; Escape closes the popup** before it closes a
  hover, the search panel or the dialog. Tab indents as before when no
  popup is open. When the selected item is exactly the name just typed (a
  variable typed out in full), Enter closes the popup and breaks the line
  instead of accepting an item that changes nothing.
- **One setting**, Settings › General › Editor: "Suggest code while typing",
  on by default. Off stops the popup from opening by itself; the shortcut
  still opens it. It covers both levels.
- **One approval covers everything.** Code checks are approved once per
  project (2026-10-28, [trust policy](code-diagnostics-trust.md)), and that
  approval already lets the same servers answer Go to Definition,
  references and hover. Completion uses it too: it works for an approved
  project whose server is running, never starts a process, and asks for
  nothing more.
- **No new errors while typing.** When the server cannot answer (not
  approved, not running, still starting, older remote host), level 1 still
  works and nothing is shown about it. Code checks already explains why a
  server is not available.
- **Typing is sent before asking.** Draft changes reach the server after a
  300 ms debounce (CD-21). A completion request first sends the pending
  change at once (CC-05) and asks for the version the server has, so
  positions match. Navigation's polling wait is too slow for this.
- **One request at a time per editor.** A new request cancels the previous
  one, locally and over SSH/WSL, through an explicit cancel binding like
  `CancelSearch`: the app cancels the request's context. Over SSH/WSL the
  client then sends `$/cancelRequest`, which cancels the server's request in
  turn, so no cancel RPC method is needed. A request times out after 5 s.
- **Filtering is local when the server allows it.** When an answer is
  complete (`isIncomplete` false), CodeMirror filters it as the user types
  more and does not ask again. An incomplete or truncated answer is asked
  again on the next keystroke.
- **Edits are applied against the version they were computed for.** An
  item's replacement range and its extra edits (`additionalTextEdits`, such
  as gopls and vtsls adding an import) are mapped through the edits typed
  since. Extra edits that touch text changed since are dropped, and the
  main edit still applies. All of it is one undoable step. vtsls sends its
  import edits only when the item is resolved, whatever the client
  advertises (found in CC-09). CC-10 returns them from a resolve at the
  document's current version. CC-12 resolves such items on accept: the
  popup closes, and the item goes in with its import once resolved, or
  alone after 500 ms, a failure, or typing meanwhile. The info panel's
  resolve is shared, so a selected item's import is usually ready.
- **Item details come later and stay on the host.** The backend keeps the
  last answer per document, through later typing, and resolves an item (`completionItem/resolve`)
  by result and index, so the server's opaque `data` never crosses the wire.
  A session keeps at most 8 MiB of these raw items over all its documents:
  older answers go first, and items past the cap fall back to their listed
  details (CC-13).
- **Commands are never run.** An item's `command` is ignored.
  `commitCharacters` are ignored too.
- **Snippets come in their own step.** CC-02 tells servers it does not
  support snippets, so they send plain text. CC-07 turns snippet support on
  and converts LSP placeholders (`$1`, `${1:name}`, `$0`) to CodeMirror
  snippets with tab stops.
- **Ruby completion and Python auto-imports are switched on in CC-08.**
  ruby-lsp ran with `completion: false` and Pyright with
  `autoImportCompletions: false` (`ruby_lsp.go`, `pyright_lsp.go`); both are
  on since CC-08. The
  settings are part of the server's fingerprint, but a project's grant
  carries over to a new fingerprint (`GrantStore.lookup`), so nobody is
  asked again. Running Ruby and Python servers restart once with the new
  settings.

## Design

### Backend (CC-02, CC-03, CC-04)

Completion follows navigation's path:
`frontend/src/lib/api.ts` → `internal/app` binding → `engine` backend
(local or `rpc.Client`) → `codediagnostics.Manager` → `SessionDocuments`.

- `initializeParams` declares `textDocument.completion`: `contextSupport`,
  `completionItem.snippetSupport` (false until CC-07, true since),
  `documentationFormat: [markdown, plaintext]`, `labelDetailsSupport`,
  `resolveSupport.properties: [documentation, detail]`, and the item kinds.
  No `insertReplaceSupport`, so servers send one range.
- `negotiateCapabilities` records `completionProvider`: its trigger
  characters and `resolveProvider`.
- New `lsp_completion.go`, beside `lsp_navigation.go`:
  `Manager.Complete(ctx, context, documentId, version, position, trigger)`
  routes to the owner that offers completion (Pyright, not Ruff), with the
  same version, position and generation checks as `Navigate`.
- Answers are normalized: `CompletionItem[]` or `CompletionList`; label,
  label details, kind, detail, sort and filter text, insert text or edit,
  insert format, extra edits, deprecated tag. Ranges are UTF-16 and are
  checked against the document. Caps: 2,000 items (more marks the answer
  incomplete), short fields to 1 KiB, documentation to 64 KiB on a UTF-8
  boundary, 2 MiB per answer. Text an item inserts (insert text, the new
  text of each edit) is not cut, since cut code would be wrong: an item with
  more than 16 KiB of it, or with more than 100 extra edits, is left out.
  So is an item whose extra edits overlap each other or the main edit, as
  part of an import can break the code (CC-13).
- Triggers: `invoked`, `character` (one of the server's trigger characters,
  checked against the session that answers) and `incomplete` (asking again
  after an incomplete answer, LSP trigger kind 3).
- Error codes are navigation's (`staleVersion`, `notRunning`,
  `resyncRequired`, `unsupported`, `timeout`, `canceled`, `busy`,
  `toolFailed`).
- Wire: app bindings `CompleteCode`, `ResolveCodeCompletion` and
  `CancelCodeCompletion(requestId)`; RPC methods `completeCode` and
  `resolveCodeCompletion` in the live method list (cancel is
  `$/cancelRequest`); a `codeCompletion` capability set only with
  `liveCodeDiagnostics`.

### Frontend (CC-01, CC-05, CC-06, CC-07)

- `@codemirror/autocomplete` becomes a direct dependency (it is already
  installed through the language packages).
- `fileEditor.ts` adds `autocompletion()` in a compartment driven by the
  setting, with the completion keymap at higher precedence than
  `indentWithTab`. The popup is themed with the app's variables, so it
  follows light and dark.
- Sources, merged in one popup: the language's own completions, words from
  the document, and the language-server source. Server items rank first;
  a word already offered by the server is not repeated. CodeMirror only
  merges equal items of the same type and apply, so the word source itself
  leaves out names the language's sources already answer with at once
  (keywords, CSS properties, which lang-css offers as `name: `). It also
  leaves out the word being typed once it is complete.
- The word source skips documents over the highlighting limit
  (`HIGHLIGHT_LIMIT`), so large files stay fast. A read-only editor shows
  no popup.
- The language-server source asks through `EditorSymbols`, as hover and
  navigation do, so the editor stays free of stores. It runs when the user
  asked (the shortcut), after a word character, or after one of the server's
  trigger characters.
- `liveDocuments.flush(path)` (CC-05) sends a pending change now and
  resolves once the server confirmed it, or after a short bound.

## Risks

- **Latency over SSH.** Each request costs a round trip. Mitigated by local
  filtering of complete answers, cancelling superseded requests and the
  flush. CC-09 measures it on a real SSH host.
- **Large answers.** vtsls can return thousands of items. The 2,000-item cap
  marks the answer incomplete, so typing more asks again with a narrower
  prefix.
- **Edit mapping bugs** could corrupt text. CC-06 tests mapping through
  later typing, overlapping extra edits and undo.
- **Keyboard conflicts.** Tab, Enter and Escape already mean something in the
  editor and the dialog. CC-01 tests each with and without a popup, in
  jsdom and with real key events in headless Chrome
  (`file-editor-chrome.test.mjs`, opt in with `AS_TEST_CHROME`). Whether
  Cmd+I reaches the editor in the macOS web view needs a native check; the
  app's menus bind no Cmd+I (wails' default menus).
- **Servers not compared yet.** Completion behaviour differs between servers;
  CC-09 checks each one for real.
- **A restart** of running Ruby and Python servers after CC-08, which may
  briefly show Code checks as starting.
- **Python imports from unopened files.** With `diagnosticMode:
  openFilesOnly`, Pyright does not load project files that are not open,
  so it cannot suggest their names with an import (found in CC-09). CC-11
  decides whether to change that.

## Release criteria

- Go tests with fake servers cover capability negotiation, routing, both
  answer forms, caps, ranges, errors, cancel, resolve and the unchanged
  approval policy.
- An approved Ruby or Python project stays approved after CC-08 (grant
  carry-over test with the new settings).
- RPC and app tests cover routing, host checks and the capability flag;
  an older remote host gets level 1 only, without errors.
- Frontend tests cover the sources, merging, the setting, the keymap, edit
  mapping, snippets and the flush; `svelte-check`, `npm test` and
  `npm run build` pass.
- CC-09 records real-server checks for each language and an SSH latency
  check. Native GUI checks on macOS, Windows and Linux are recorded only if
  they were actually run.

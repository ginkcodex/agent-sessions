# PPS-01 — Shared print page setup

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** —
- **Plan:** [print-page-setup.md](../../plan/print-page-setup.md)

## Scope

Expose paper, orientation and uniform margin controls in the shared print
editor. Apply selected setup to CSS and native Linux/macOS printing, retaining
Windows interactive printing and old binding compatibility. Default to A4,
portrait, 14 mm margins, 100% scale. Explain native dialog overrides.

## Likely files

- `frontend/src/lib/print.ts`, `printPageSetup.ts`, `api.ts`, `mock/mockApi.ts`
- `frontend/src/lib/components/common/Print{Dialog,Sheet}.svelte`
- `internal/app/{print,desktop}.go`, print/router tests
- `cmd/agent-sessions/{desktop_v3,print_*}`
- `frontend/test/print-*.test.mjs`

## Acceptance criteria

- All shared print flows offer A4/Letter, portrait/landscape and 0–40 mm margins.
- Defaults and selected values match between CSS and Linux/macOS native setup.
- Invalid values cannot reach native printing; older calls/bindings still work.
- Preview uses the selected printable width; controls do not enter PDF output.
- Native print dialogs remain available, with guidance for scale and headers.

## Tests

Unit tests for setup validation/dimensions/CSS, component tests for controls and
document dispatch, Go validation/window-routing tests. Run full frontend tests,
check/build, Go race tests and lint; compile native adapters where possible.
Record native PDF comparisons separately.

## Evidence

- Updated from `origin/develop` before implementation; already up to date.
- Added shared print-editor controls: A4/Letter, portrait/landscape and uniform
  0–40 mm margins, default A4/portrait/14 mm. Retains selection for the current
  window. Invalid/empty margins disable Print and its keyboard shortcut.
- Shared setup generates physical `@page` size/margins; preview displays selected
  printable width and the sheet uses it as its maximum width. No extra margin
  padding is added. Temporary styles/title restore on afterprint, fallback
  interaction, failure and teardown. Existing 2 px macOS clipping guard remains.
- Added validated `PrintWindowWithSetup` binding and per-window desktop callback;
  preserved argumentless PrintWindow and older-binding fallback. Real native
  errors are not retried, avoiding duplicate print dialogs.
- Linux uses a fresh WebKitPrintOperation/GtkPageSetup/GtkPrintSettings with
  selected paper/orientation/margins and scale 100%, parented to the caller.
  macOS custom printing receives matching settings and resets scale to 100%.
  Windows retains interactive WebView2 printing with shared CSS and guidance
  to match native dialog settings and disable headers/footers when offered.
- New frontend regression tests cover validation, dimensions, CSS/style cleanup,
  API fallback, real editor/dropdown/input changes, invalid keyboard dispatch,
  file/message document dispatch, setup retention, native print invocation and
  lifecycle cleanup/default/invalid-document behavior.
- Focused frontend print tests: 12/12 passed (10 new, 2 existing).
- Full frontend suite: 1,202 passed, 1 skipped, 0 failed.
- `npm run check`: 0 errors, 0 warnings.
- `npm run build`: passed; Vite still warns about large chunks.
- Re-ran focused print tests (12/12), check and build after the final preview
  grid overflow fix; all passed.
- `make test` (`go test -race ./...`): passed across the repository.
- Backend validation/default/JSON/error-propagation/router tests passed, including
  `go test -race ./internal/app -run 'Test(DefaultPrint|Print|Router)' -count=1`.
- Linux desktop compile passed: `go build -tags desktop` to a fresh temporary
  directory. Windows desktop cross-compile passed with CGO_ENABLED=0,
  GOOS=windows, GOARCH=amd64, also to a fresh temporary directory.
- `make lint`: failed with 135 existing findings in unchanged Go files; none
  target this feature's modified/new Go files.
- macOS native code could not be compiled here (no macOS SDK). No native print
  dialog/PDF visual comparison was performed on any OS; actual margin handling
  and pagination still need manual comparison. GTK asynchronous print failures
  are logged after dialog dispatch.

# Notes for Claude

## Git attribution

When creating commits, do not add `Co-Authored-By` trailers or any other
AI/agent attribution to commit messages. Likewise, do not add AI/agent
attribution to pull request descriptions. Preserve this rule for all new
changes, including amended or rewritten commits.

## Docs: plans and tasks

Every new feature request follows the same steps, before any code is written:

1. **Plan:** write `docs/plan/<feature>.md` (goal, design, decisions, risks,
   release criteria) and add it to the Features table in
   `docs/plan/README.md`. Contracts, spikes, QA records and other supporting
   notes for one plan sit beside it, flat, as `docs/plan/<feature>-<topic>.md`.
2. **Task group:** create `docs/tasks/<feature>/`, named after the plan.
   - `README.md` starts with `# <Feature> — tasks`, then
     `- **Plan:** [<feature>.md](../../plan/<feature>.md)` and
     `- **Progress:** N/M done`, then a table:
     `| Task | Status | Scope | Est | Depends on |`, one row per task, each
     linking to its file.
   - One `<ID>.md` per task. IDs use a short prefix for the group plus a number
     (`RB-01`). Each file starts with `# <ID> — <title>`, then a block of
     `- **Status:**`, `- **Estimate:**`, `- **Depends on:**` and
     `- **Plan:**` (a link to the plan), then the details: scope, likely
     files, acceptance criteria and tests. Tasks are at most 4 h; split
     larger work into more tasks before starting.
3. **Indexes:** add the group to the Features tables in `TASKS.md` and
   `docs/tasks/README.md`.

While implementing, keep the progress current in the same change as the code:

- Set the task file's Status and add what was done and how it was verified
  (an `## Evidence` section: tests run, results, what was not checked).
- Update the group README's table row and `**Progress:**` line, and the
  Progress cells for the group in `TASKS.md` and `docs/tasks/README.md`.
- Status values: ✅ Done · 🟡 Partial (say what is missing) · ⛔ Blocked
  (say on what) · ⬜ Not started · ➖ Superseded. Planned QA is not a pass;
  mark only what actually ran.
- New work found along the way becomes a new task file and row, not an
  untracked note.

Milestones M0–M8 follow the same layout (`docs/plan/M*.md`,
`docs/tasks/M*/`). Reference docs that are not plans stay in `docs/`
(`formats.md`, `perf.md`). When moving or renaming a doc, `git grep` for its
path and fix every link.

## Windows test builds

Development runs in WSL2 on Windows. When the user asks for a Windows test
version of the desktop client, put it in the next unused
`C:\Users\Gink\agent-sessions-launchN` (WSL: `/mnt/c/Users/Gink/agent-sessions-launchN`).
Check which numbers exist first and never overwrite an existing folder: an
older build in it may still be running.

Each folder holds:

- `agent-sessions.exe`: the desktop app.
- `agent-sessions-cli.exe`: the CLI.
- `remote/`: freshly built servers, with their `SHA256SUMS`. The app deploys
  these to WSL and SSH hosts from `remote\` next to the exe. Stale bundles miss
  server-side fixes, so always rebuild them.
- `conpty/`: the terminal panel's ConPTY (`make conpty`). Without it the
  terminal falls back to Windows' own, which hides the terminal's colors from
  TUIs such as OpenCode.

Build in a fresh `/tmp` folder so `build/bin` and `build/windows` stay
untouched. `make windows-app` is the same build `make package-windows` uses:
it writes the icon, manifest, version info and `.syso` to
`$WINDOWS_BUILD_DIR/windows` and the exe to `$WINDOWS_BUILD_DIR/bin`, and it
does not edit any tracked file. While it compiles it keeps a copy of the
`.syso` in `cmd/agent-sessions/`, so it refuses to run while another Windows
build from this checkout is running.

```sh
N=6   # next unused number
B=/tmp/as-win-build-$N; W=/mnt/c/Users/Gink/agent-sessions-launch$N
test ! -e "$B" && test ! -e "$W"
V=$(sed -n 's/^ *"productVersion": *"\([^"]*\)".*/\1/p' wails.json)
LDFLAGS="-s -w -X github.com/ginkcode/agent-sessions/internal/version.Version=$V"

make windows-app WINDOWS_BUILD_DIR="$B"
make remote-servers REMOTE_DIR="$B/remote"
make conpty CONPTY_DIR="$B/conpty"
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" \
  -o "$B/bin/agent-sessions-cli.exe" ./cmd/agent-sessions-cli

mkdir -p "$W/remote" "$W/conpty"
cp "$B/bin/agent-sessions.exe" "$B/bin/agent-sessions-cli.exe" "$W/"
cp "$B/remote/"*.gz "$B/remote/SHA256SUMS" "$W/remote/"
cp "$B/conpty/"* "$W/conpty/"
(cd "$W/remote" && sha256sum -c SHA256SUMS)
"$W/agent-sessions-cli.exe" version
```

Tell the user the Windows path of `agent-sessions.exe`, the version it
reports, and whether the build includes uncommitted changes.

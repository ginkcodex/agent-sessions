# Notes for Claude

## Git attribution

When creating commits, do not add `Co-Authored-By` trailers or any other
AI/agent attribution to commit messages. Likewise, do not add AI/agent
attribution to pull request descriptions. Preserve this rule for all new
changes, including amended or rewritten commits.

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

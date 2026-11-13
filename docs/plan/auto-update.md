# Automatic updates on macOS and Windows

Planned 2026-11-13. Tasks: [auto-update](../tasks/auto-update/README.md).

## Goal

On macOS and Windows the app downloads a new release in the background,
checks it, and installs it when the user agrees to restart, or when they
quit. Users no longer download and run installers by hand. Linux keeps its
package managers ([linux-repos.md](linux-repos.md)).

## Today

- `internal/update` compares the app's version with GitHub's latest release
  once a day, one window checking for all of them
  (`frontend/src/lib/update.ts`), and a toast opens the release page.
- **macOS:** a universal `.dmg`, Developer ID signed, notarized and stapled
  in CI. Users drag the app to a folder of their choice.
- **Windows:** a per-user MSI in `%LOCALAPPDATA%\Programs\Agent Sessions`, so
  no UAC prompt. It is not code-signed. `_setup.exe` removes the installed
  version and installs the new MSI as two Windows Installer transactions,
  because `wixl` cannot schedule an in-place major upgrade: if the second
  fails, the app is gone (`packaging/windows/README.md`).
- Release files are checked by `SHA256SUMS`, which comes from the same
  release as the files it covers.

## Design

### Trust

- CI signs the release's `SHA256SUMS` with **minisign**, writing
  `SHA256SUMS.minisig`. The secret key is used only by the release job, in a
  protected `release` environment.
- The app has the public key(s) built in, as a list so a new key can be added
  a release before the old one is retired.
- An update is used only if:
  1. `SHA256SUMS.minisig` verifies with a built-in key;
  2. the installer's size and SHA256 match its line in `SHA256SUMS`;
  3. its version, from the file name, is newer than the running one;
  4. on macOS, the new `.app` passes `codesign --verify --strict` and
     `spctl --assess`, and its Team ID equals the running app's.

Only GitHub and the signature are involved; the website Worker that serves
the Linux repositories ([linux-repos.md](linux-repos.md)) plays no part.

### Finding an update

`internal/update` downloads, from GitHub Releases directly:

- `https://github.com/ginkcodex/agent-sessions/releases/latest/download/SHA256SUMS`
- `https://github.com/ginkcodex/agent-sessions/releases/latest/download/SHA256SUMS.minisig`

These are release downloads, not GitHub API calls, so the API's limit of 60
unauthenticated requests an hour does not apply. After the signature
checks, the version is read from the installer's file name in `SHA256SUMS`
(`agent-sessions_<version>_macos_universal.dmg`,
`agent-sessions_<version>_windows_amd64.msi`). The installer comes from
`releases/download/v<version>/<file>`, and the toast's release page is
`releases/tag/v<version>`.

The same check replaces today's API call on every platform, Linux included,
where it only feeds the toast. The daily schedule and the
one-window-checks-for-all rule stay as they are.

`releases/latest` is the release GitHub marks as Latest. The release
workflow already sets it for stable releases only (`--latest`), so
pre-releases are never offered.

### Pulling a bad release

Mark it as a pre-release on GitHub, or mark the previous release as Latest.
`releases/latest` then points at the previous release again: apps that
have not updated stop being offered the bad one, and a ready download of it
is dropped at the next check (a ready version that differs from the latest
is removed). Apps never downgrade; a fixed release is the way forward for
those already on it. Marking it as a pre-release (not only moving Latest)
also drops it from the Linux repositories within an hour.

### Downloading

- One process downloads, holding a lock (`internal/filelock`) in
  `<user cache>/agent-sessions/updates/`.
- Files go to `updates/<version>/`, are checked as above, and only then
  marked ready. Older version folders are removed.
- A failed download or check is retried at the next daily check. After
  three failures for the same version, the app shows today's toast with the
  release page instead.

### Asking

| State | What the user sees |
|---|---|
| Downloading | Nothing |
| Ready | Toast: "Agent Sessions 1.6.0 is ready. **Restart now** / Later" |
| Later | Installs on the next quit; the toast comes back at the next launch |
| Not possible here (see below) | Today's toast, with the release page |
| Setting off | Today's toast |

Settings gets **Download updates automatically** (on by default). The app
never restarts on its own: running terminals and chats would be lost.
**Restart now** goes through the normal quit, so the usual close
confirmations still ask about running terminals.

### Checking by hand

The daily check runs about 10 s after launch when one is due, then once a
day; the due time is shared by every window and survives a restart. So a new
release can go unnoticed for up to a day. A manual check closes that gap:

- **Settings › General › Updates** has **Check for updates** (AU-14). It
  uses the same backend call as the daily check, so a release it finds is
  downloaded, checked and announced as usual.
- It counts as the day's check: on success the shared due time moves a day
  ahead, so no window repeats it. A failure leaves the schedule as it was.
- It always answers. While it runs the button says "Checking…" and is
  disabled (also while a download runs). Then the status line reads
  "Up to date: 1.6.0 is the latest version", or shows the update state as
  for the daily check, or "Couldn't check for updates: <reason>". A
  development build says it has no release to compare with.
- A ready version whose toast was put off with **Later** is not toasted
  again: the Settings line already offers **Restart now**.
- It does not clear a give-up. A version that failed three downloads, or
  failed to install, stays on its release page.
- The macOS app menu gets **Check for Updates…** below About (AU-15). It
  opens Settings › General and starts the check. Windows and Linux have no
  app menu; Settings is the place there.

### Installing

The app copies itself to `helper/` in the updates folder and starts the copy
with `--apply-update <ready folder>`. That copy waits for the app to exit, so
no file it replaces is in use. It writes a log to the updates folder, and the
next launch shows any failure.

On macOS the copy is the whole `.app` bundle (an APFS clone where possible),
not just the executable: a Developer ID executable with the hardened runtime
is bound to its bundle's `Info.plist`, and macOS kills a bare copy as it
starts (AU-16). On both systems the app quits only once the helper has
reported in over a pipe (macOS) or started (Windows). A helper that cannot
start counts as a failed install: the app restarts or quits as if no update
were ready, and the next launch points to the release page.

**macOS:**

- **When auto-update is possible:**
  - the bundle's parent folder is writable by this user;
  - the app is not running from App Translocation or a mounted disk image
    (`/Volumes/...`);
  - it is a release build.
- **Preparing**, before asking the user: mount the `.dmg` read-only and
  without showing it, check the `.app`, and copy it next to the installed
  bundle, on the same volume, as `.Agent Sessions.app.update-<version>`.
- **Applying:** rename the installed bundle aside, rename the new one into
  its place, start it with `open`, then delete the old one. If any step
  fails, put the old one back.
- The bundle keeps its path and its bundle ID (`com.wails.agent-sessions`),
  so permissions users granted stay.

**Windows:**

- **When auto-update is possible:** the running exe is the one the MSI
  installed: its path matches the product's install folder in the Windows
  Installer registry. Test builds in `agent-sessions-launchN` folders and
  portable copies never update themselves.
- **Preparing:**
  - download the new MSI and, for rollback, the installed version's MSI;
  - check both against their signed `SHA256SUMS`;
  - check that the MSI is ours (UpgradeCode, product name) with the code
    `packaging/windows/setup` already uses.
- **Applying:**
  - wait until no process runs from the install folder (the app, its CLI,
    terminals started from it);
  - remove the installed product, then install the new MSI with `/quiet`,
    using setup's verbose logs;
  - if the install fails after the removal, reinstall the old MSI;
  - start the new exe.
- Windows Installer's own cached copy of the installed MSI is not used for
  rollback: it may lack the embedded files.

### Remote servers

The update replaces the server bundles inside the app. The existing deploy
compares versions and checksums when it connects, so WSL and SSH hosts get
the new server on their next connection.

## Decisions

- **Our own updater, not Sparkle.** Sparkle covers only macOS. It would need
  cgo, an embedded framework signed inside the bundle, and its own feed.
  Windows needs its own path regardless.
- **minisign over `SHA256SUMS`** rather than a new manifest format. One
  signature covers every file of a release, and users can check it by hand
  with `minisign -Vm`.
- **Download quietly, ask before restarting**, as VS Code and Chrome do.
- **Windows keeps remove-then-install, with our own rollback MSI.** That is
  the path `_setup.exe` already tests. Moving the MSI build to WiX on a
  Windows runner, for an atomic in-place upgrade, is a later option.
- **Windows code signing (Azure Trusted Signing) is a separate task, not a
  release criterion.** minisign protects the update path. Code signing
  removes the SmartScreen warning on first install.
- **macOS first.** Its signing and notarization are already in place.
  Windows follows.
- **A manual check is the day's check**, not an extra one. Clicking it
  should never make the daily check run again, nor hurry it.
- **A manual check does not retry a given-up version.** The failure would
  most likely repeat: the release page stays the way to get that version.

## Risks

- **A broken updater cannot update itself.** The fallback toast must keep
  working whatever the updater's state, and every release is tested
  updating to the next one (AU-08, AU-11). This happened: 1.5.6 shipped a
  macOS helper that macOS kills on start (AU-16). A 1.5.6 install downloads
  the next version, and every Restart now or quit then ends with no update
  and no relaunch. Users on 1.5.6 must install the next version from its
  `.dmg` once, and its release notes must say so.
- **Windows removal succeeds, install and rollback both fail:** the app is
  gone. The log names the kept MSIs for a manual install, and the next
  manual setup recovers.
- **macOS folder not writable** (an admin installed the app, a standard user
  runs it): auto-update is off for that user, who sees today's toast.
- **Losing the minisign key** means shipping a release with a new key the
  old apps don't know. Mitigation: keep an offline backup, and list a second
  key in the app from the start.
- **A pulled release already downloaded** stays ready until the next check,
  which drops a ready version that is no longer the latest. A user who
  restarts before that check installs it.
- **GitHub's Latest flag decides what is offered.** Marking the wrong
  release as Latest by hand offers it to every app; only the release
  workflow, or a deliberate pull, should change it.

## Release criteria

- macOS: a Developer ID signed build N, installed from its `.dmg`, updates to
  N+1 after **Restart now**, and after **Later** plus a quit. The bundle path
  and granted permissions are unchanged.
- Windows: an MSI install of N updates to N+1 the same ways. A failure
  injected after the removal leaves N installed again.
- A bad signature, a wrong hash, a wrong Team ID, or an unwritable folder
  each leave the installed app untouched and show today's toast.
- With the setting off, or for test builds and portable copies, nothing is
  downloaded.
- Running terminals are never ended without the normal close confirmation.

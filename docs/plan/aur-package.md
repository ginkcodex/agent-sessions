# Arch Linux: the agent-sessions-bin AUR package

Planned 2026-11-08. Tasks: [aur-package](../tasks/aur-package/README.md).

## Goal

Arch Linux users install and update the app with an AUR helper:

```sh
paru -S agent-sessions-bin    # or: yay -S agent-sessions-bin
```

Every stable release updates the AUR package without manual steps.

## Today

Releases ship a `.deb` and an `.rpm` per architecture (`make package-linux`,
`packaging/nfpm.yaml`). Arch users have nothing to install them with.

## Design

`agent-sessions-bin` repackages the release's `.deb`. It holds the same files
apt installs: `/usr/bin/agent-sessions`, the `.desktop` entry, the icon, and
the remote servers in `/usr/lib/agent-sessions/remote/`, where
`internal/remote/locate.go` looks for them. The binary is built on
Ubuntu 24.04 (glibc 2.39) and links GTK 4 and WebKitGTK 6.0. Arch is rolling,
so its glibc is newer, and its `gtk4` and `webkitgtk-6.0` packages provide
the same sonames.

| Piece | Role |
|---|---|
| `packaging/arch/PKGBUILD.in` | The PKGBUILD with `@PKGVER@`, `@PKGREL@` and checksum placeholders |
| `packaging/arch/render-pkgbuild.sh` | Fills the template for one release from its `SHA256SUMS` and copies `LICENSE` beside it |
| `packaging/arch/verify-aur-package.sh` | In an `archlinux:base-devel` container: writes `.SRCINFO`, builds with `makepkg` (downloads the published `.deb` and checks its checksum), checks the aarch64 source checksum, installs with pacman, checks libraries, files and the host server's version, removes it |
| `packaging/arch/publish-aur.sh` | Commits `PKGBUILD`, `.SRCINFO` and `LICENSE` to an AUR checkout; pushes to `master` only with `--push` |
| `make aur-package VERSION=x.y.z` | Downloads the published `SHA256SUMS`, renders and verifies into `build/aur/` |
| `.github/workflows/aur.yml` | Reusable workflow: render, verify, publish. Called by `release.yml` after a stable release; also runnable by hand for a released version, with a `pkgrel` |

The AUR checkout used locally lives in `aur/` (git-ignored).

## Decisions

- **Binary package first.** It ships exactly the binaries CI tested.
  A from-source `agent-sessions` package would need Go, Node and npm downloads
  at build time, and a toolchain that keeps up with Arch. It can come later,
  and `provides`/`conflicts` already leave room for it.
- **Stable releases only.** A pacman `pkgver` cannot hold a `-`, and an AUR
  package should follow the GitHub "Latest" release, so the AUR job skips
  versions with a suffix.
- **Checksums come from the release's `SHA256SUMS`**, the file
  `release-checksums.sh` verified, and `makepkg` checks them against the
  downloaded asset before the package is pushed.
- **`LICENSE` lives in the AUR repository**, so the package does not download
  it. The license (PolyForm Noncommercial 1.0.0) is non-free: the AUR accepts
  that, but the official repositories would not.
- **AUR host keys are pinned** in `aur.yml`. They are the fingerprints
  aur.archlinux.org publishes (checked 2026-11-08), not keys a `ssh-keyscan`
  would accept on first use.
- **A failed AUR job does not undo the GitHub release.** The release is
  already published by then. Fix the problem and run the AUR workflow by hand.

## Risks

- The `archlinux` Docker image is x86_64 only. The aarch64 package is checked
  for its source checksum, not built and installed (Arch Linux ARM is a
  separate distribution).
- A future Arch `webkitgtk-6.0` soname bump breaks the binary until the next
  release. Ubuntu-built binaries cannot follow Arch's rebuilds.
- The `AUR_SSH_PRIVATE_KEY` secret can push to every AUR package of that
  account. Use a dedicated key, added to the AUR account only.

## Release criteria

- `make aur-package` for the latest release (1.3.2) builds, installs and
  removes the package in a clean Arch container.
- `agent-sessions-bin` is on the AUR, and `paru -S agent-sessions-bin`
  installs it.
- A stable release pushes the new version to the AUR from CI, and a
  pre-release does not.

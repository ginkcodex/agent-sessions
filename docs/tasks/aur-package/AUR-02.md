# AUR-02 — Build, install and remove it in an Arch container

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** AUR-01
- **Plan:** [aur-package.md](../../plan/aur-package.md)

## Scope

`packaging/arch/verify-aur-package.sh PKGDIR [IMAGE]`, in
`archlinux:base-devel`, as an AUR helper would: writes `.SRCINFO`, builds with
`makepkg` as a non-root user (downloading the published x86_64 `.deb` and
checking its checksum), checks the aarch64 source's checksum, runs namcap
(advisory), installs with pacman, then checks the libraries, files, desktop
entry and the host server's `version`, and removes the package.

`make aur-package [VERSION=…] [PKGREL=…]` downloads the release's
`SHA256SUMS`, renders and verifies into `build/aur/`.

## Likely files

`packaging/arch/verify-aur-package.sh`, `Makefile`.

## Acceptance criteria and tests

- `make aur-package` passes for the latest release in a clean container.
- Every library the binary links resolves from the declared dependencies, and
  namcap reports no missing direct dependency.
- Removing the package leaves none of its paths behind.

## Evidence

- `make aur-package VERSION=1.3.1` and `make aur-package` (1.3.2) in
  `archlinux:base-devel` (pulled 2026-11-08), exit 0 both. makepkg downloaded
  the x86_64 `.deb` and passed its checksum, built the package, and passed
  the aarch64 `.deb`'s checksum. pacman installed the package with its
  dependencies, all 157 linked libraries resolved, the desktop entry passed
  `desktop-file-validate`, 4 servers passed `gzip -t`, the linux-amd64 server
  reported `1.3.2`, and `pacman -Rns` left none of the paths behind.
- namcap, first run: `libsoup3` and `libx11` "detected and implicitly
  satisfied", now declared (AUR-01). Remaining warnings: `source_x86_64`
  mentions x86_64 (the standard per-architecture source idiom), and the
  binary lacks full RELRO and PIE (the release build's flags; out of scope
  for repackaging).
- Not checked: the aarch64 package is not built or installed (the image is
  x86_64 only), and the app is not started (no display).

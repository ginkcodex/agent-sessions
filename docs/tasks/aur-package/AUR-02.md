# AUR-02 — Build, install and remove it in an Arch container

- **Status:** ⬜ Not started
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

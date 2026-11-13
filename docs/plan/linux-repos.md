# Linux package repositories (apt and dnf)

Planned 2026-11-13. Tasks: [linux-repos](../tasks/linux-repos/README.md).

## Goal

Debian, Ubuntu and Fedora users add the Agent Sessions repository once, and
`apt upgrade` or `dnf upgrade` brings every new stable release from then on:

```sh
# Debian 13+, Ubuntu 24.04+
curl -fsSL https://agent-sessions.ginklab.cv/apt/agent-sessions.gpg \
  | sudo tee /usr/share/keyrings/agent-sessions.gpg >/dev/null
echo "deb [signed-by=/usr/share/keyrings/agent-sessions.gpg] https://agent-sessions.ginklab.cv/apt stable main" \
  | sudo tee /etc/apt/sources.list.d/agent-sessions.list
sudo apt update && sudo apt install agent-sessions

# Fedora 40+
sudo dnf config-manager addrepo --from-repofile=https://agent-sessions.ginklab.cv/rpm/agent-sessions.repo
sudo dnf install agent-sessions
```

Every stable version stays listed and downloadable, so users skipping
several releases, or upgrading from an old package list, never hit a 404.

## Today

- Releases ship a `.deb` and an `.rpm` per architecture
  (`make package-linux`, `packaging/nfpm.yaml`), about 45 MB each. Arch
  users get them through the AUR ([aur-package.md](aur-package.md)).
  Debian and Fedora users download and install each release by hand, and
  the app's update toast sends them to the release page.
- An earlier apt repository on GitHub Pages kept only recent packages. Users
  whose package list was older than the newest files got 404s, because the
  files their list named had been deleted.
- The website (`web/`, its own repository `ginkcodex/agent-sessions-web`) is
  a static SvelteKit build on Cloudflare Workers Static Assets at
  `agent-sessions.ginklab.cv`, with no Worker code, bindings or secrets.

## Design

### What is where

| Piece | Lives in | Notes |
|---|---|---|
| `.deb` and `.rpm` files | GitHub Releases | Never deleted or replaced |
| Index entries per package (`repo-meta.json`) | Release asset, made in CI | Covered by `SHA256SUMS` |
| Release and asset records | Postgres, through Hyperdrive | Every release ever ingested |
| Generated, signed index files | Postgres, as numbered generations | Rebuilt when releases change |
| Serving | Website Worker, `/apt/*` and `/rpm/*` | Package downloads are 302s to GitHub |

The Worker serves only the Linux repositories. The macOS and Windows
updater reads GitHub Releases directly ([auto-update.md](auto-update.md)).

### In CI (this repository)

- **RPMs are signed at build time** by nfpm with the RPM key (secret in this
  repository). The signed file is the release asset, so `SHA256SUMS` covers
  it and `rpm -K` checks it in `packaging/linux/verify-packages.sh`.
- **`repo-meta.json`** is written by `packaging/linux/repo-meta.sh` from the
  built packages, in the jobs that gather the release set (`artifacts`,
  `release`), since each `linux` job builds one architecture:
  - for each `.deb`, the `Packages` stanza `apt-ftparchive packages` writes
    for that one file, with `Filename:` set to `pool/<version>/<file name>`;
  - for each `.rpm`, the `<package>` elements `createrepo_c` writes for that
    one file in `primary.xml`, `filelists.xml` and `other.xml`, with
    `location href` set to `pool/<version>/<file name>`. createrepo_c
    records the file's mtime as `<time file>`, and a download sets that
    mtime, so the script sets it to the package's build time: the same
    package always gives the same entry. Ubuntu's createrepo_c (0.17, on the
    runner) and Fedora's (1.2) write the same elements.
- `release-checksums.sh` expects `repo-meta.json` in the release set, and
  checks that it lists the release's packages with their sizes and hashes,
  and that each stanza and `<package>` element names its file's location,
  size and hash.
- `packaging/linux/assemble-rpm-repo.sh` puts the elements together the way
  LR-12 will, for the tests: `make test-repo-meta` has dnf read the result
  in a Fedora container.
- After publishing, a `linux-repos` job calls the Worker's ingest endpoint.

### Worker setup

The website's Worker gains code. `wrangler.jsonc` sets
`assets.run_worker_first` for `/api/*`, `/apt/*` and `/rpm/*`; every other
path stays a static asset as today. The Worker connects to the maintainer's
Postgres through a Hyperdrive binding (`HYPERDRIVE`), with the postgres.js
client. Production uses `agent_sessions_prod`; previews bind their own
config to `agent_sessions_demo`. Both are reached through Cloudflare
Tunnels.

The repository lives in the database, not in the deployed files, so a
website deploy, from any branch or by hand, cannot remove or roll it back.

| Name | Where | Use |
|---|---|---|
| `HYPERDRIVE` | Worker binding | Postgres |
| `INGEST_TOKEN` | Worker secret; `LINUX_REPOS_TOKEN` in this repository | The release workflow's ingest call |
| `GITHUB_TOKEN` | Worker secret | Fine-grained, read-only access to public release data (5,000 requests an hour) |
| `REPO_SIGNING_KEY` | Worker secret | Signs the indexes (see Keys) |

### Data

Migrations are plain SQL files in `web/migrations/`, applied by
`npm run db:migrate` with a `DATABASE_URL`. The Worker never changes the
schema.

| Table | Holds |
|---|---|
| `releases` | version (primary key), tag, published time, prerelease flag, the release's `SHA256SUMS` text, ingested time |
| `assets` | version, file name, os, arch, kind (`deb`, `rpm`, `meta`, `other`), size, sha256, download URL |
| `release_documents` | version, name, body: `repo-meta.json`, kept verbatim |
| `repo_generations`, `repo_files`, `repo_state` | Generated index files by generation, and the current generation |

### Adding a release

1. The release workflow calls `POST /api/ingest {"tag": "v1.6.0"}` with
   `Authorization: Bearer $INGEST_TOKEN`.
2. The Worker trusts only the tag name from that call. It reads the release
   from the GitHub API with its own `GITHUB_TOKEN`, downloads `SHA256SUMS`,
   and checks `repo-meta.json` against it.
3. It writes the release, its assets and documents in one transaction, then
   rebuilds the repositories.
4. Ingesting a release again is a no-op when its `SHA256SUMS` and prerelease
   flag are unchanged. When either changed (a rerun's
   `gh release upload --clobber`, or a release marked as a pre-release), the
   rows are replaced and the repositories rebuilt.

An hourly cron trigger lists the ten newest releases and ingests any that
are missing or changed, so a failed call, or a release pulled on GitHub,
is picked up within an hour. `npm run backfill` ingests every existing
release once.

**Pulling a bad release** is done on GitHub: mark it as a pre-release. The
cron sees the flag and drops it from the next generation, and the app's
updater stops offering it (`releases/latest` no longer points at it).

### Building the repositories

Each rebuild makes a new generation from every stable release with a
`repo-meta.json`:

- **apt** (suite `stable`, component `main`, architectures `amd64` and
  `arm64`): `dists/stable/main/binary-<arch>/Packages` and `Packages.gz`,
  `dists/stable/Release` with `Acquire-By-Hash: yes`, `InRelease` and
  `Release.gpg`, and the `by-hash/SHA256/<hash>` copies.
- **dnf** (one repository for `x86_64` and `aarch64`): `repodata/` with
  hash-prefixed `primary`, `filelists` and `other` XML files, `repomd.xml`
  and `repomd.xml.asc`.

A generation is written in full before `repo_state.current` points at it.
Hash-named files are looked up in every kept generation, so a client that
read the previous `InRelease` or `repomd.xml` still finds what it names. The
Worker keeps the newest five generations and any written in the last 14
days.

### Serving

| Path | Answer | Cache |
|---|---|---|
| Hash-named files under `/apt/dists/` and `/rpm/repodata/` | From the kept generations | `immutable`, one year |
| `InRelease`, `Release`, `Release.gpg`, `repomd.xml`, `repomd.xml.asc` | From the current generation | 5 minutes |
| `/apt/pool/<version>/<file>`, `/rpm/pool/<version>/<file>` | 302 to the GitHub Release file, only for files in the `assets` table; otherwise 404 | 1 day |
| `/apt/agent-sessions.gpg`, `/apt/agent-sessions.asc` | The repository's public key | 1 hour |
| `/rpm/agent-sessions.repo`, `/rpm/RPM-GPG-KEY-agent-sessions` | dnf's repository file, listing both public keys | 1 hour |

apt checks each `.deb` against the SHA256 in the signed index, and dnf checks
each RPM's signature, so the download host does not have to be trusted.

### Keys

| Key | Type | Held by | Signs |
|---|---|---|---|
| Repository key | Ed25519 OpenPGP | Worker secret `REPO_SIGNING_KEY` | `InRelease`, `Release.gpg`, `repomd.xml.asc` |
| RPM key | RSA 4096 OpenPGP, unless LR-02 shows Ed25519 works in nfpm and on Fedora 40 | This repository's secret, used by the linux job | Each `.rpm` |

Both get an expiry date and a written renewal and rotation procedure
(LR-15).

### In the app

On Linux the update toast checks whether the repository is set up: an apt
source (`/etc/apt/sources.list`, or a `.list` or `.sources` file in
`/etc/apt/sources.list.d/`) or a `/etc/yum.repos.d/*.repo` section that
names our domain. Sources that are commented out (as `do-release-upgrade`
leaves third-party ones), `Enabled: no` or `enabled=0` do not count. If it
is, the toast shows `sudo apt update && sudo apt upgrade` (or
`sudo dnf upgrade`) with **Copy command**. On Arch (`/etc/arch-release`) it
says to update with the AUR helper (`paru -Syu agent-sessions-bin`).
Otherwise it links to the website's install guide
(`https://agent-sessions.ginklab.cv/download`) and the release page.

## Decisions

- **Every stable release stays listed.** Indexes stay small: a few hundred
  KB with hundreds of releases.
- **Package files stay on GitHub**, behind a 302 that only answers for files
  the Worker has recorded. GitHub Pages cannot redirect, which is why the
  repositories live on the Worker.
- **The ingest call carries only a tag.** Everything else is read from
  GitHub and checked against `SHA256SUMS`, so a leaked ingest token can only
  make the Worker re-read GitHub.
- **GitHub is the one place to pull a release**, by marking it a pre-release.
  The Worker has no admin endpoints.
- **Postgres through Hyperdrive**, since the maintainer already runs it.
  Reads go through Cloudflare's cache, so the database is needed mainly
  when a release is added.
- **Schema changes run outside the Worker**, so a deploy never migrates a
  database by surprise.
- **One `stable` suite** for every supported distribution: each release is a
  single binary built on Ubuntu 24.04 (`nfpm.yaml` targets Ubuntu 24.04+,
  Debian 13+, Fedora 40+).
- **Stable releases only**, as on the AUR. A `beta` suite can come later.
- **The repository starts with the first release that ships
  `repo-meta.json` and signed RPMs.** Older releases are recorded by the
  backfill but not listed: their RPMs are unsigned. Users of older
  `.deb`/`.rpm` installs upgrade as soon as they add the repository.
- **Packages do not add the repository on install.** Users run the
  documented commands; changing their package sources silently is not ours
  to do.
- **Two keys.** The Worker can sign indexes without being able to sign
  packages, so dnf users keep a check the Worker cannot forge.

## Risks

- **The repository key is root access to apt users' machines.** Whoever
  holds it can get any package installed. It is a dedicated key, stored only
  as a Worker secret, with 2FA on the Cloudflare account and a rotation
  procedure (LR-15).
- **Key expiry.** An expired key stops `apt update` everywhere. Renewal is a
  dated task in the procedure, well before expiry.
- **Postgres or Hyperdrive down:** ingest fails and uncached index reads
  fail. The cron retries ingest; cached files keep serving for their TTL.
- **Worker CPU limits.** Rebuilding and signing in one request may need more
  than the free plan's CPU time (10 ms per invocation). In local workerd
  (LR-01), a full rebuild takes 2.6 ms for 50 releases, 6.4 ms for 300 and
  17 ms for 1000, before database work. The free plan fits today; Workers
  Paid is needed at a few hundred releases, or sooner if the preview's
  measurement is higher. A rebuild skips work when its inputs are
  unchanged.
- **Redirects across hosts.** apt and dnf follow the 302 from our domain to
  github.com and on to GitHub's file host; some corporate proxies may not.
  LR-02 checks both clients.
- **Ubuntu's newer apt checks signatures with Sequoia (`sqv`)** and enforces
  its own algorithm policy. LR-02 checks Ed25519 with both `gpgv` and `sqv`.
- **Two repositories change together.** The Worker code lives in
  `agent-sessions-web`; CI and these docs live here. Each task names the
  repository it changes.

## Release criteria

- In clean Ubuntu 24.04, Debian 13 and Fedora 40 containers, plus the newest
  Ubuntu and Fedora, the documented commands add the repository and install
  the app. `apt policy` / `dnf --showduplicates list` show every stable
  release.
- An install of the oldest listed version upgrades straight to the newest.
- A container whose package list is several releases old runs
  `apt upgrade` without `apt update` and downloads what its list names.
- A stable release appears in both repositories within a minute of
  publishing; a pre-release does not.
- With the workflow's ingest call skipped, the cron adds the release within
  an hour. A release marked as a pre-release on GitHub drops out within an
  hour.
- `npm run backfill` run twice changes nothing the second time.
- The app's Linux toast gives the right instruction with and without the
  repository set up.

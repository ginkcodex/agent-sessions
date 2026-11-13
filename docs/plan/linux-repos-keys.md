# Linux repositories — signing keys

Part of [linux-repos.md](linux-repos.md) (Keys). Task: [LR-15](../tasks/linux-repos/LR-15.md).

The repositories use two keys:

- the **repository key** signs apt's `Release` and dnf's `repomd.xml`;
- the **RPM key** signs each `.rpm` in the release job.

Both are v4 EdDSA (algorithm 22) Ed25519 OpenPGP keys, valid for three years.
`packaging/linux/repo-keys.sh` makes them and looks after them.

## The keys

Not made yet. The maintainer makes them as in [Making the keys](#making-the-keys)
and fills in this table.

| Key | Fingerprint | Expires | Renew by | Secret | Public |
|---|---|---|---|---|---|
| Repository | — | — | — | Worker secret `REPO_SIGNING_KEY` (production) | `/apt/agent-sessions.gpg`, `/apt/agent-sessions.asc`, first block of `/rpm/RPM-GPG-KEY-agent-sessions` |
| RPM | — | — | — | `RPM_SIGNING_KEY`, a secret of this repository's `release` environment | `RPM_PUBLIC_KEYS` in the website's `worker/repo/keys.ts`, served after the repository key in `/rpm/RPM-GPG-KEY-agent-sessions` |

"Renew by" is six months before the expiry date. Put it in a calendar with a
reminder a month earlier. By that date the repository key is extended, and
the RPM key is replaced by a new one (see
[Rotating the RPM key](#rotating-the-rpm-key)). `repo-keys.sh show` says which
to do.

Previews never get the production repository key. A preview runs code that
nobody has reviewed yet, and whoever holds the repository key can get any
package installed on apt users' machines. Previews sign with a throwaway key
made the same way.

## Making the keys

Make the keys on an offline machine with GnuPG 2.4, keeping `<dir>` on
encrypted storage. The keys have no passphrase, because the Worker and
`sign-rpms.sh` read them as they are, so `<dir>` and its backups are what need protecting.

```sh
packaging/linux/repo-keys.sh new repo <dir>
packaging/linux/repo-keys.sh new rpm <dir>
packaging/linux/repo-keys.sh show <dir>
```

Each key leaves three files in `<dir>`: `<role>-<key ID>.pub.asc`,
`.secret.asc` and `.rev.asc` (the revocation certificate).

The script refuses a key that is not v4 EdDSA, in case a later gpg makes
another kind by default. RFC 9580's Ed25519 (algorithm 27) fails on
Ubuntu 24.04's gpgv and Fedora 40's rpm (LR-02). Each key's user ID gives the
year it was made, for example `Agent Sessions repository signing key (2026)`,
so the two keys of a rotation can be told apart.

### Backups

```sh
packaging/linux/repo-keys.sh backup <dir> agent-sessions-keys-2026.asc
packaging/linux/repo-keys.sh restore agent-sessions-keys-2026.asc <new dir>   # to check it
```

`backup` encrypts the whole of `<dir>` with a passphrase (AES-256):

- the GnuPG home;
- the exported keys;
- the revocation certificates.

Keep two copies in different places, and the passphrase in a password
manager.

### Storing them

From the machine that holds `<dir>`, with the files going straight into each
tool's standard input:

```sh
# Website (in agent-sessions-web): production only.
packaging/linux/repo-keys.sh secret <dir> <repository fingerprint> | npx wrangler secret put REPO_SIGNING_KEY

# This repository: a secret of the release environment, which the release
# jobs' sign-rpms.sh reads.
gh secret set RPM_SIGNING_KEY --env release -R ginkcodex/agent-sessions < <dir>/rpm-<key ID>.secret.asc
```

Then:

1. Copy `<dir>/rpm-<key ID>.pub.asc` into `RPM_PUBLIC_KEYS` in the website's
   `worker/repo/keys.ts`, and deploy.
2. The next rebuild picks the change up. It runs on the hourly cron or on the
   next ingest. A rebuild hashes every key's public packets, so any change of
   key writes a new generation.
3. Check what is served. Each fingerprint must match the table above:

```sh
curl -fsSL https://agent-sessions.ginklab.cv/apt/agent-sessions.asc | gpg --show-keys
curl -fsSL https://agent-sessions.ginklab.cv/rpm/RPM-GPG-KEY-agent-sessions | gpg --show-keys
```

The first command must list the repository key. The second must list the
repository key, then the RPM key.

A `REPO_SIGNING_KEY` that is missing or cannot be used does not stop the
site. The rebuild logs the reason (`SigningKeyError`) and the current
generation stays served.

## Renewal (extending the expiry)

This is for the repository key only. Renew it by the "Renew by" date in the
table.

The RPM key is never extended: `repo-keys.sh extend` refuses it. rpm 4.19
(Fedora 40) keeps the copy of the key it imported first. Importing the
extended key changes nothing, so once the old expiry date passes, packages
signed after the renewal fail on those machines (see
[RPM signatures](#rpm-signatures-lr-08)). Make a new RPM key instead, as in
[Rotating the RPM key](#rotating-the-rpm-key).

1. Run `packaging/linux/repo-keys.sh extend <dir> <fingerprint>`. The key gets
   a new self-signature valid for three years from today; its fingerprint
   stays the same.
2. Store it again, as in [Storing them](#storing-them):
   `secret … | wrangler secret put REPO_SIGNING_KEY`. The next rebuild serves
   the renewed public key and signs with it.
3. Update the table, the calendar entry and the backup.
4. **apt users have to fetch the keyring again before the old expiry date.**
   apt checks against the copy in `/usr/share/keyrings/agent-sessions.gpg`,
   not the served one. The tests below show that a copy past its old expiry
   is refused even though the served key was extended. Say so in the next
   release's notes and on the install page, with the `curl … | sudo tee`
   line from the install steps. [LR-21](../tasks/linux-repos/LR-21.md) would
   make this automatic by shipping the keyring in the `.deb`.
5. dnf users need to do nothing. dnf accepted signatures from the extended key
   with the old copy imported, even after that copy's expiry date.

A renewal is a lot cheaper than a rotation. Renew rather than letting a key
run out.

## Rotating the repository key

Rotate when moving to a new key: at the end of the key's planned life, or
right away if it may have leaked (see [If a key leaks](#if-a-key-leaks)).

`REPO_SIGNING_KEY` may hold several keys:

- **apt:** every key signs `Release`, and apt needs one signature it can
  check;
- **dnf:** only the first key signs `repomd.xml`, because dnf refuses a
  `repomd.xml.asc` with two signatures (LR-13).

Steps (A is the old key, B the new one):

1. Make B with `repo-keys.sh new repo <dir>`.
2. Set `REPO_SIGNING_KEY` to A then B:
   `repo-keys.sh secret <dir> <A> <B> | npx wrangler secret put REPO_SIGNING_KEY`.

   The next rebuild has the following effects:
   - `Release` is signed by both keys;
   - `repomd.xml` is still signed by A;
   - the key files list A and B.

   Nothing changes for users yet.
3. Ask apt users to fetch the keyring again, in the release notes and on the
   install page. Leave as long as possible between this step and step 5: at
   least two releases, and months rather than weeks.
4. Set `REPO_SIGNING_KEY` to B then A. `repomd.xml` is now signed by B, and
   dnf imports B from the key file on its next refresh. It asks first, as for
   the first key, unless `-y` is given. apt still accepts A's signature.
5. Set `REPO_SIGNING_KEY` to B. apt users who still have only A get
   `NO_PUBKEY` (gpgv) or `Missing key` (sqv) until they fetch the keyring
   again.
6. Update the table and the backup. Keep A's files and revocation
   certificate in the backup.

## Rotating the RPM key

Rotate the RPM key by its "Renew by" date, as well as when moving to a new
key or after a leak.

Every release stays listed, and each `.rpm` keeps the signature it was
signed with. So an old RPM key stays in `RPM_PUBLIC_KEYS` for as long as
packages signed with it are listed, which means for good. Its expiry does not
matter for those packages: rpm checks a signature against the key as it was
when the signature was made, and the packages it signed kept verifying after
it expired (see [RPM signatures](#rpm-signatures-lr-08)).

1. Make the new key: `repo-keys.sh new rpm <dir>`.
2. Add its public block to `RPM_PUBLIC_KEYS` after the old one. Deploy the
   website and wait for the next rebuild.
3. Only then switch the `RPM_SIGNING_KEY` secret of the `release`
   environment.

When a user first installs a package signed by the new key, dnf should ask
to import it from the key file, as it did for a new repository key. This has
not been tried with signed packages from the repository yet:
[LR-16](../tasks/linux-repos/LR-16.md).

## If a key leaks

**The repository key.** Whoever has it can get any package installed on
every apt user's machine.

1. Make a new key and set `REPO_SIGNING_KEY` to it alone, at once.
2. Tell users to fetch the keyring again: in the release notes, on the
   install page, and on the project's channels.
3. Publish the old key's revocation certificate (`.rev.asc` in the backup).
   Revoking it in the served key files is not done by the Worker today.
   apt users who replace their keyring drop the old key anyway.

**The RPM key.** Make a new RPM key and replace the `RPM_SIGNING_KEY`
secret. Then
decide whether the packages signed with the old key stay listed. Their
`.rpm` files are on GitHub and cannot be re-signed in place.

## What the procedure is based on (LR-15 checks)

**Where the checks ran**

- The script ran with throwaway keys in three containers: Ubuntu 24.04
  (gpg 2.4.4), Debian 13 (gpg 2.4.7) and Fedora 45 (gpg 2.4.9).
- The repositories were built by the Worker's own `buildFiles` and
  `loadSigningKeys` for each key set.
- They were served from a static server at fixed URLs, so the content changed
  under the same URL as it would for users.

**Expired test keys.** These were made with `--faked-system-time`:

- E was made three days earlier, valid two days, and expired on 2026-11-12;
- E2 was made three days earlier and was valid for 30 days.

**apt.** Ubuntu 24.04 uses apt 2.8.3 with gpgv 2.4.4; Debian 13 uses
apt 3.0.4 with sqv 1.3.0; Ubuntu 26.10 uses apt 3.3.3 with sqv 1.4.0. Each
cell is `apt update`'s result.

| User's keyring | Repository signed by | Ubuntu 24.04 | Debian 13 | Ubuntu 26.10 |
|---|---|---|---|---|
| A | A | ✅ | ✅ | ✅ |
| A | A and B | ✅ | ✅ | ✅ |
| A | B and A (also from `repo-keys.sh secret B A`) | ✅ | ✅ | ✅ |
| A | B | ❌ `NO_PUBKEY` (expected) | ❌ `Missing key` (expected) | ❌ `Missing key` (expected) |
| A and B (fetched in step 3) | B | ✅ | ✅ | ✅ |
| E2 before its extension (not expired) | E2 extended, signed now | ✅ | ✅ | ✅ |
| E before its extension (expired) | E extended, signed now | ❌ `EXPKEYSIG` | ❌ key not live | ❌ key not live |
| E before its extension (expired) | E, signed while it was valid | ❌ `EXPKEYSIG` | ✅ | ✅ |
| E extended | E extended, signed now | ✅ | ✅ | ✅ |
| E extended | E, signed before the extension | ✅ | ❌ no binding signature at that time | ❌ no binding signature at that time |

**What the apt results mean**

- **Rotations work only for users who fetch the keyring again.**
- **A renewal works only if users fetch the keyring before the old expiry
  date.**
- **gpg keeps only the newest self-signature.** It replaces the
  self-signature when it extends a key, so a renewed keyring has no binding
  signature for earlier times, and sqv then refuses signatures made before
  the extension. The Worker signs at every rebuild, and a renewal changes
  the key's packets, so a new generation signs after the extension. The
  only gap is the up to 5 minutes an older `InRelease` stays in caches.

**dnf.** These checks ran on Fedora 40 (dnf 4.22, rpm 4.19) and Fedora 45
(dnf5 5.4.6, rpm 6.1), with `repo_gpgcheck=1` and `makecache --refresh`.
Each sequence ran in the same container, so the imported keys carried over
from one step to the next.

| Sequence | Fedora 40 | Fedora 45 |
|---|---|---|
| Rotation 1: A; key file A + RPM key | ✅ imports both | ✅ imports both |
| Rotation 2: `repomd.xml` by A; key file A, B | ✅ nothing imported | ✅ nothing imported |
| Rotation 3: `repomd.xml` by B; key file B, A | ✅ fetches the key file again and imports B | ✅ same |
| Rotation 4: B only | ✅ | ✅ |
| Renewal in time: E2, then E2 extended | ✅, ✅ | ✅, ✅ |
| Late renewal: E (expired copy imported), then E extended | ✅, ✅ | ✅, ✅ |

**What the dnf results mean**

- dnf follows a repository key rotation by itself.
- dnf did not refuse an expired imported copy of the repository key. Do not
  rely on that: renew in time anyway.

**Not checked**

- A real key, and the steps on a preview deployment. Both need the
  maintainer.
- The RPM key's rotation through the repository with signed packages
  (LR-16). Its expiry and renewal were checked with `rpm` (below).
- dnf-automatic, which may not answer the import prompt in rotation step 4.

### RPM signatures (LR-08)

`packaging/linux/sign-rpms.sh` signs with rpmsign 4.18.2 (Ubuntu 24.04, as on
the release job's runner). The packages were the v1.5.6 release's two `.rpm`
files and a small test package, signed with throwaway keys from the LR-15
checks.

| Check | Fedora 40 (rpm 4.19.1.1) | Fedora 45 (rpm 6.1.0) |
|---|---|---|
| Signed v1.5.6, key imported: `rpm -K` | ✅ `Signature, key ID 306c3b25: OK` | ✅ `signature, key fingerprint: …306c3b25: OK` |
| Signed v1.5.6, key not imported: `rpm -i --test` | ✅ installs with a `NOKEY` warning | ❌ `does not verify: … NOKEY` |
| Unsigned v1.5.6 (as released today): `rpm -i --test` | ✅ | ❌ `does not verify: no signature` |
| Payload changed after signing: `rpm -K` | ❌ `Payload SHA256 digest: BAD` | ❌ same |
| `dnf install ./file.rpm`, signed or not, key imported or not | ✅ | ✅ with `skipped OpenPGP checks … @commandline` |
| Signed while the key was valid, checked with the expired copy | ✅ | ✅ |
| Signed while the key was valid, checked with the extended key | ✅ | ✅ |
| Signed after an extension, checked with the expired copy | ❌ `NOTTRUSTED`: key not alive | ❌ same |
| The same, after `rpm --import` of the extended key | ❌ the import exits 0 but keeps the old copy | ✅ the import updates it |

What they mean:

- rpm 6 refuses to install an unsigned package, or one whose key is not
  imported, with plain `rpm -i`. dnf does not check local files.
- Old packages keep verifying after their key expires. So an expired RPM key
  can stay in `RPM_PUBLIC_KEYS`.
- Extending the RPM key breaks rpm 4.19 machines once the old expiry passes.
  Hence the rule: rotate the RPM key, never extend it.

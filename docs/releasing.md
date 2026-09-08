# Releasing

Pushing a `v*` tag publishes the release. `.github/workflows/release.yml` verifies
the tag and calls `gh release create`; there is no manual step on GitHub.

## Cutting a release

```bash
# 1. Bump both version fields. They must agree — `make check-version` fails otherwise.
#    plasmoid/metadata.json  "Version": "0.3.0"
#    PKGBUILD                pkgver=0.3.0

# 2. Land it through a PR like anything else, so CI runs on the commit.

# 3. Tag the merged commit and push the tag.
git checkout main && git pull --ff-only
git tag -a v0.3.0 -m "codecap 0.3.0"
git push origin v0.3.0
```

The workflow then refuses the tag unless all of these hold, before anything is
published:

- the tagged commit is contained in `main` — `ci.yml` runs on `main` and on pull
  requests and never on a tag, so a tag pushed from a local branch would
  otherwise publish code no gate has run;
- the tag names the version the tree declares (`scripts/check-release-tag.sh`);
- `go test -race ./...` and the Node suite pass on the tagged commit.

The QML suites are not repeated there. They need Qt and Kirigami, they ran on
`main`, and tagging does not change them.

## The `PKGBUILD` checksum comes after the tag

`PKGBUILD` pins `sha256sums` for one tag's archive, and GitHub does not generate
that archive until the tag is pushed. The checksum therefore cannot be correct at
tagging time — it is a step *after* the release, not before:

```bash
updpkgsums          # rewrites sha256sums from the now-published archive
git commit -am "chore(packaging): pin the v0.3.0 archive checksum"
```

This is not a race in practice. `README.md` tells people to fetch `PKGBUILD` from
`main`, not from the tag, so what they get is whatever is committed there — the
tagged tree carrying a stale checksum never reaches anyone.

The release workflow checks this for you and leaves a **warning**, not a failure,
with the exact value to paste. Failing would be wrong: the release itself is
published and valid, and only `PKGBUILD` needs the follow-up.

## What is not automated

- **Publishing to the AUR.** It needs an AUR account, and AUR registration was
  closed as of 2026-09-08. `PKGBUILD` builds from the pinned release archive and
  `makepkg -si` works from that file alone, so the packaging half is done; see
  [`roadmap.md`](./roadmap.md).
- **Get New Widgets / store.kde.org.** Deliberately not started. The store installs
  a plasmoid and has no way to install the Go helper or register D-Bus activation,
  so a store-only install would show a widget that never reports numbers.

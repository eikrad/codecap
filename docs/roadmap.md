# Roadmap

v1 sequence. Domain is [`CONTEXT.md`](../CONTEXT.md); architecture and visual are [`design.md`](./design.md). This file is **order of work**, not new product decisions.

Each phase should be runnable on this machine before the next starts. Phase 4 may begin once phase 1’s snapshot JSON is real.

Phases 0–5 are implemented on `main`. Optional follow-ups: AUR publish, Get New
Widgets packaging.

**AUR publish is blocked externally, not unfinished.** `PKGBUILD` builds from the
pinned v0.2.0 release archive and `makepkg -si` works from the file alone, so the
packaging half is done. Uploading needs an AUR account, and AUR registration was
closed as of 2026-09-08. Nothing in this repository unblocks it.

That makes **Get New Widgets the live distribution path**: its stated precondition
here — "a documented helper install for non-Arch (release tarball is enough)" — is
met as of the v0.2.0 release, which the README now documents.

A full-tree audit on 2026-08-22 found that several of those phases are implemented but not
working. Findings: [`audit-2026-08-22.md`](./audit-2026-08-22.md). Order of work to close them:
[`plan-hardening.md`](./plan-hardening.md).

## v1

0. **Repo hygiene** — rename `Claude-counter` → `codecap`; licence **GPL-2.0-or-later** (SPDX/REUSE headers + `LICENSES/`); `git init`; GitHub repo **`eikrad/codecap`** (public — AUR and Get New Widgets need a public source tarball). English UI with `i18n()` wrappers.

1. **Helper skeleton** — D-Bus `GetSnapshot` / `Changed` (`dev.codecap.Helper`). Faces Unbound / Signed Out / Unknown Allowance. Empty Account Home is local to the plasmoid (no bus call). No vendor HTTP yet.

2. **Consumed Usage** — watch Account Home logs. Session / today / week / month with List Price (USD in the snapshot) and tokens. Desktop timezone and locale week start come from the plasmoid arguments.

3. **Allowance** — Session Allowance, Weekly Allowance, Usage Credit, Last-Known (XDG cache, not written into Account Home). Conservative vendor fetch. Unofficial HTTP stays in the helper.

4. **Plasmoid** — compact Session ring, hover tooltip, expanded bars + FormLayout, Configure (Account Home + Display Currency), HIG/Breeze. FX in QML. Missing helper → Unknown Allowance.

5. **Install** — FHS layout + local `PKGBUILD` / `make install`. AUR after that works here. Get New Widgets last, once there is a documented helper install for non-Arch (release tarball is enough).

## Not v1

Agents beyond Claude and Codex, colour picker, threshold sliders, idle-exit helper, Flatpak/Snap helper, `.deb` / RPM / OBS as a ship gate, Weekly % on the compact tooltip.

## Codex extension · 2026-10-28

Requested: reuse Session and Weekly Allowance presentation for Codex, and fix
repeated Claude request accounting affecting every Consumed Usage period.
Implementation and verification contract: [ADR 0017](adr/0017-codex-account-home.md).
Real local log fixtures and a live Codex Allowance response are captured;
final installed-widget desktop acceptance remains separate from automated gates.

## Optional widget marker · 2026-11-02

Requested: distinguish widget instances with a per-widget color marker.
The user selected color identification without panel short names or Agent logos.
The explicit visual revision is documented in design.md and ADR 0018.
Desktop acceptance completed 2026-11-02: the user confirmed persistence after
reloading plasmashell and accepted the small marker's appearance.

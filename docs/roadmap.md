# Roadmap

v1 sequence. Domain is [`CONTEXT.md`](../CONTEXT.md); architecture and visual are [`design.md`](./design.md). This file is **order of work**, not new product decisions.

Each phase should be runnable on this machine before the next starts. Phase 4 may begin once phase 1’s snapshot JSON is real.

## v1

0. **Repo hygiene** — rename `Claude-counter` → `codecap`; licence **GPL-2.0-or-later** (SPDX/REUSE headers + `LICENSES/`); `git init`; GitHub repo **`eikrad/codecap`** (public — AUR and Get New Widgets need a public source tarball). English UI with `i18n()` wrappers.

1. **Helper skeleton** — D-Bus `GetSnapshot` / `Changed` (`dev.codecap.Helper`). Faces Unbound / Signed Out / Unknown Allowance. Empty Account Home is local to the plasmoid (no bus call). No vendor HTTP yet.

2. **Consumed Usage** — watch Account Home logs. Session / today / week / month with List Price (USD in the snapshot) and tokens. Desktop timezone and locale week start come from the plasmoid arguments.

3. **Allowance** — Session Allowance, Weekly Allowance, Usage Credit, Last-Known (XDG cache, not written into Account Home). Conservative vendor fetch. Unofficial HTTP stays in the helper.

4. **Plasmoid** — compact Session ring, hover tooltip, expanded bars + FormLayout, Configure (Account Home + Display Currency), HIG/Breeze. FX in QML. Missing helper → Unknown Allowance.

5. **Install** — FHS layout + local `PKGBUILD` / `make install`. AUR after that works here. Get New Widgets last, once there is a documented helper install for non-Arch (release tarball is enough).

## Not v1

Other Agents, colour picker, threshold sliders, idle-exit helper, Flatpak/Snap helper, `.deb` / RPM / OBS as a ship gate, Weekly % on the compact tooltip.

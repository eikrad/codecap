# Design

A Plasma 6 QML applet plus a Go session helper. Product name **codecap**. Domain language is in [`CONTEXT.md`](../CONTEXT.md) (**Agent** stays a glossary term; it is not the package name). Work order is [`roadmap.md`](./roadmap.md).

## Names

- D-Bus service + interface: `dev.codecap.Helper`
- Object path: `/dev/codecap/Helper`
- Helper binary: `codecap`
- Plasmoid id: `dev.codecap.plasmoid`

## Licence

<!-- REUSE-IgnoreStart -->
**GPL-2.0-or-later** (GNU GPL v2 or later) for plasmoid and helper. Stated with SPDX/REUSE headers (`SPDX-License-Identifier: GPL-2.0-or-later`) and `LICENSES/GPL-2.0-or-later.txt`, which is KDE’s current format — not a different licence from “GNU2+”. AUR `license=('GPL-2.0-or-later')` uses the same identifier.
<!-- REUSE-IgnoreEnd -->

## Two artifacts

**Store plasmoid** — QML-only KPackage for Get New Widgets. Never reads Account Home credentials, never holds tokens, never uses Plasma5Support `executable`. Talks to the helper over the session bus via `org.kde.plasma.workspace.dbus`.

**Session helper** — Go binary, separate user-session install (not inside the `.plasmoid` zip). Sole process that uses the Account Home `/login`, fetches Session / Weekly Allowance, watches local logs for Consumed Usage, and serves Last-Known Allowance. If the helper is missing, the plasmoid shows Unknown Allowance — not fake 0%, not a fallback that copies the token into plasmashell.

## Why this split

There is no official individual plan-% API. Claude Code’s statusline `rate_limits` exist only inside a running CLI session. `claude setup-token` is documented as model-requests-only. Get New Widgets cannot ship a C++ plugin. Plasma5Support executable is a compat dataengine, not the long-term store API. QML `XMLHttpRequest` can do HTTPS but not `file://` logs, and would keep the login in plasmashell.

Research: [`research-store-without-plasma5support.md`](./research-store-without-plasma5support.md).

## Distribution

The **code is not Arch-specific.** The plasmoid is a Plasma 6 KPackage (any distro). The helper is Go plus FHS files: `/usr/bin/codecap`, a session D-Bus `.service`, a systemd `--user` unit. That is the same layout on Debian, Fedora, and Arch.

**v1 channel:** local `PKGBUILD` / AUR first (this machine is Manjaro). Get New Widgets after there is a documented helper install for non-Arch (release tarball is enough). `.deb` / RPM / OBS are later packaging, not a rewrite. Flatpak/Snap for the helper is out of v1.

## IPC

The plasmoid calls a session-bus service (`org.kde.plasma.workspace.dbus`). JSON payload shape stays transport-agnostic. Localhost HTTP is not the plasmoid path; it can exist later as a debug extra.

The helper is **D-Bus activated** by the helper package (session `.service` file). The first widget call starts it; several widgets share one process. After that it **stays up for the graphical session** (systemd `--user` restarts on crash; logout ends it). No idle-exit in v1. The plasmoid never execs the helper. If the package is not installed, the call fails → Unknown Allowance.

## Bus contract

One method: `GetSnapshot(accountHome, timezone, weekStart) → json`. One signal: `Changed(accountHome)`. The helper owns the two clocks. Compact and expanded both render that snapshot. List Price in the JSON is USD; Display Currency conversion stays in the plasmoid.

Display Currency defaults to the **desktop locale’s currency** (Denmark / `da_DK` → **DKK**, Germany → EUR, US → USD), not a hard-coded euro. Per-widget override is any ISO code the FX feed knows. Daily public USD→Display Currency rate via `XMLHttpRequest` (ECB-style, no API key); cache in plasmoid config. Offline or failed fetch → show USD with a quiet note.


Empty Account Home: the plasmoid does not call the helper (Unbound is local).

Snapshot JSON:

`schema_version` is the shape of this object; `degraded` says why a snapshot is
incomplete, as stable codes rather than message text (the plasmoid owns the wording,
and an error string could carry a filesystem path onto the bus). `GetSnapshot` serves
what is cached and returns; refreshes run in the helper and announce themselves with
`Changed`, per ADR 0011.

```json
{
  "schema_version": 1,
  "face": "unbound | signed_out | unknown_allowance | ready",
  "account_home": "",
  "account_label": "",
  "session_allowance": { "used_percent": 0, "resets_at": 0, "stale": false },
  "weekly_allowance": { "used_percent": 0, "resets_at": 0, "stale": false },
  "usage_credit": "none | enabled | available | exhausted",
  "consumed_usage": {
    "session": { "list_price_usd": 0, "tokens": 0 },
    "today": { "list_price_usd": 0, "tokens": 0 },
    "week": { "list_price_usd": 0, "tokens": 0 },
    "month": { "list_price_usd": 0, "tokens": 0 }
  },
  "fetched_at": 0,
  "degraded": ["pending | usage_unavailable | allowance_unavailable"]
}
```

## Visual

Frozen 2026-08-20. Chrome only. Compact **content** (Session used % + time to reset; never fake 0%) is domain, in [`CONTEXT.md`](../CONTEXT.md). Change look only with an explicit visual revision, not as a side effect of implementation.

**Authority:** [KDE Human Interface Guidelines](https://develop.kde.org/hig/) plus Plasma 6 widget APIs. The applet must look like a first-party Plasma widget (battery / network analog), not a branded overlay. Implementation uses `org.kde.plasma.components`, Kirigami, `Kirigami.Theme` / `PlasmaCore.Theme`, and `Kirigami.Units` / `PlasmaCore.Units` (`smallSpacing`, `largeSpacing`, `gridUnit`, `IconSizes`). No bundled custom icons, no hardcoded hex colours, no `QtQuick.Text` (use `PlasmaComponents.Label` / `Kirigami.Heading`). Copy follows HIG: sentence case for labels/tooltips, real ellipsis `…`, locale for numbers and Display Currency. Colour is never the only signal (fill amount + tooltip text + labels).

**Motion:** static ring and bars. No spinner, pulse, or colour-cycle. Quiet panel.

**Compact** (panel / system tray): circular Session-used ring. Percent numeral inside the ring when there is room (typical panel); omitted in a tight tray. Empty compact icons are Breeze symbolic (theme-coloured, no vendor logo): Unbound `folder-open-symbolic`, Signed Out `unlock-symbolic`, Unknown Allowance `question-symbolic`. Never a 0% ring.

**Compact tooltip** (hover): `Session {used%} · {time to reset}`. If Last-Known: `Last-Known` on that line. Mute the ring (`disabledTextColor`); no extra overlay icon. Weekly %, List Price, tokens, and Account Home stay out of the tooltip — those are the click popup.

**Colour:** Plasma theme only (`Kirigami.Theme` / `PlasmaCore.Theme`). Ring fill is the desktop accent (`highlightColor`). **≥80% used** → `neutralTextColor`. **100% used** → `negativeTextColor`. Stale uses `disabledTextColor`. Same bands on expanded Session / Weekly bars. Thresholds are not settings in v1. No per-widget colour picker; look follows System Settings → Appearance.

> **Visual revision 2026-08-26 — D3, decided: Usage Credit does not colour the ring.** Was: "100% used **or Usage Credit** → `negativeTextColor`". A fill's colour now reports only the fill it sits on. Usage Credit is a monthly state on a different clock from the Allowance Window the ring draws, so an exhausted credit painted a Session at 37% as if it were spent — the compact face's one job is reporting the Session, and that was the part it got wrong. Usage Credit keeps its status line in the expanded view, which is where `CONTEXT.md` already puts it.

**Expanded** (click popup; also the desktop face): no second ring. Session Allowance and Weekly Allowance are **horizontal bars** with `%` and time-to-reset under each label, same colour bands as the compact ring. Usage Credit is a status line, not a third bar. Consumed Usage is a `Kirigami.FormLayout` (session / today / week / month; List Price then tokens). Account label at the top. Breeze spacing; no custom brand cards.

**Empty faces:** never draw 0% bars. Unbound → `Kirigami.PlaceholderMessage` (folder icon) plus Configure…. Signed Out → placeholder (unlock icon) asking to sign in with that Agent for this Account Home (no login form in QML). Unknown Allowance → keep Account label and Consumed Usage; hide Session/Weekly bars; question icon + one-line note. Account Home is picked in **Configure Widget** (Plasma folder picker), not as a live path field in the popup.

**Configure Widget:** two fields only in v1 — Account Home (folder picker; placeholder hint `~/.claude`) and Display Currency (default desktop locale, optional ISO override). No colour picker, no threshold sliders, no compact-layout checkboxes. Compact % numeral is automatic from available width.






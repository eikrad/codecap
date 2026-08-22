# Hardening plan — v1.1

Order of work coming out of [`audit-2026-08-22.md`](./audit-2026-08-22.md).
Same role as [`roadmap.md`](./roadmap.md): **this file is sequence, not new product
decisions.** Every item below closes a gap between what the ADRs already decided and
what the code does. Where something genuinely needs a product call, it is not in a
phase — it is in [Decisions needed](#decisions-needed) at the end.

Finding IDs (`C-2`, `P-C1`, …) refer to the audit.

## Status

| Phase | State |
|---|---|
| **0 · Gates** | **Done.** `make lint` and `make ci` are green locally; CI runs three parallel jobs. |
| **1 · Make it work** | **Code done, not yet run on a Plasma 6 desktop.** See the caveat below. |
| **2 · Stop the bleeding** | **Done.** One deviation from 2.1, recorded below. |
| 3–6 | Not started. |

**What Phase 1 has not had.** None of the QML changes have been executed on a real
Plasma 6 session — this work was done in a container with no Plasma, no Kirigami and
no D-Bus session bus. What they have had:

- `qmllint` over every `.qml` file (syntax and structure only; Plasma and Kirigami
  types still cannot be resolved, so a wrong property name on a Plasma type would
  still pass).
- 18 Node tests over `logic.js`, including a regression test for each JS-side bug.
- Go tests for the timezone contract, including one that fails if day boundaries stop
  following the zone they are given.

Four of the eight critical findings were *non-existent names* — `onReceivedSignal`,
`onConfigurationChanged`, `onHighlightColorChanged` — and the replacements
(`dbusChanged`, `onConfiguredAccountHomeChanged`) are the same kind of name. They are
written from the upstream sources cited in the audit, but only a real desktop can
confirm they fire. **Please run the Phase 1 "Done when" checklist below before trusting
it.** Phase 5.2 and 5.3 exist to remove exactly this blind spot.

**Deviation in 2.1.** The plan said to move the vendor call outside the credentials
lock. It is still inside it, deliberately: two processes rotating the same refresh
grant would invalidate each other, and the lock is the only thing preventing that.
What made holding it dangerous was the *absence of a timeout* — an indefinite hang
wedged the helper for the session. Now the call is bounded by `httpx.Timeout` (15 s),
the lock wait is 20 s so a waiter cannot give up while the holder is legitimately
refreshing, and `EnsureAccessToken` has a lock-free fast path for the common case
where the token is still valid, so concurrent `GetSnapshot` calls never touch the
lock at all. If the grant ever stops being rotated on refresh, moving the call out
becomes the better trade.

**Coverage after phase 2** — `accounthome` 100 %, `usage` 80.5 %, `allowance` 77.8 %,
`dbusapi` 72.3 %, `usagewatch` 66.7 %, `creds` 62.2 %. The poller went from 0 % to
covered; `Export` is no longer untestable, because `NewServer` now takes its
dependencies (finding M-14, pulled forward from 5.8).

---

## The shape of the problem

The audit found 4 + 4 critical items, and they split cleanly in two:

**The plasmoid does not work.** It cannot be opened with the mouse (`P-C3`), never
receives the `Changed` signal (`P-C2`), rejects the `~/.claude` path the README tells
users to type (`P-C1`), and shows USD amounts labelled as the user's currency
(`P-C4`). Each is a one-line-to-one-function fix. All four are *non-existent names* or
*missing elements* — `onReceivedSignal` instead of `dbusChanged`, no `MouseArea` at
all — the exact class of bug a linter catches and no test in this repo can.

**The helper does the right things in the wrong place.** ADR 0011 says the helper owns
polling and file watch; the implementation does everything synchronously inside the
D-Bus method instead, with no timeouts (`C-2`, `C-3`), which is also why one hung TCP
connection wedges it for the session.

Underneath both sits the same root cause: **CI is green and blind** (`H-7`). `gofmt`
already fails on `main`. That is why Phase 0 comes first and costs an afternoon.

## Sequencing rationale

| | Why here |
|---|---|
| **0 · Gates** | Half a day, and every later phase is verified by it instead of by hand. Landing it first also means Phase 1's fixes arrive with a linter that would have caught them. |
| **1 · Make it work** | Highest user-visible value per line changed. Small, local, independently testable. Nothing else matters while the widget is inert. |
| **2 · Stop the bleeding** | Bounded, mechanical safety fixes. Deliberately *before* the architecture change, so Phase 3 is a refactor of code that is already safe. |
| **3 · Move the work** | The ADR 0011 architecture. Big, and it subsumes the performance problem — do it once, on a stable base. |
| **4 · Ship-ability** | Nothing here is felt until someone installs from a package. Needed before a release, not before a test drive. |
| **5 · Test depth** | Expensive (a QML harness in CI). Worth it only once the code under it has stopped moving. |
| **6 · Polish** | Correctness and HIG debt with no failure mode attached. |

Phases 0–2 are independently shippable. Phase 3 should land as one reviewable change.

---

## Phase 0 · Gates that cost minutes

**Goal:** the pipeline can fail for a reason. Land each step already green.

| # | Work | Finding | Size |
|---|---|---|---|
| 0.1 | `gofmt` check in CI; reformat `internal/dbusapi/server_test.go` | `H-7` | 10 min |
| 0.2 | `go test -race ./...` in `make test` | `H-5`, `H-7` | 5 min |
| 0.3 | `.golangci.yml` (`errcheck`, `staticcheck`, `govet`, `ineffassign`, `unused`, `gosec`) + job; fix the 5 current findings | `H-7` | 1 h |
| 0.4 | `timeout-minutes: 10` and a `concurrency` group | `L-18` | 5 min |
| 0.5 | `shellcheck -s sh scripts/*.sh` | `M-13` | 15 min |
| 0.6 | `qmllint` over `plasmoid/contents/**/*.qml`, syntax-gating first, warnings ratcheted later | `P-M · test coverage` | 2 h |
| 0.7 | `.github/dependabot.yml` (gomod + github-actions); pin actions to SHAs | `M-12`, `L-17` | 20 min |
| 0.8 | `govulncheck` step; `go get -u ./... && go mod tidy` to clear the 2-year `x/sys` gap | `M-12` | 30 min |
| 0.9 | `reuse lint` step; SPDX header on `logic.js`, `REUSE.toml` block for the 5 other files | `M-11`, `P-L12` | 30 min |

**Done when:** a PR that reintroduces any Phase-0 finding fails CI, and `qmllint` runs
against every `.qml` file. 0.6 is the one with real setup cost — Plasma import
resolution in a CI container is fiddly; gate on syntax first and ratchet.

---

## Phase 1 · Make the widget work

**Goal:** a user can install this, type `~/.claude`, click the widget, and see correct
numbers in their own currency and their own timezone.

| # | Work | Finding | Size |
|---|---|---|---|
| 1.1 | Strip the `file://` scheme from `homeDir`; make `expandPath` reject a scheme prefix | `P-C1` | 30 min |
| 1.2 | `MouseArea` on the compact representation toggling `expanded`, mirroring `DefaultCompactRepresentation` | `P-C3` | 30 min |
| 1.3 | Rename the signal handler to `dbusChanged(accountHome)` — arguments arrive decoded | `P-C2` | 15 min |
| 1.4 | FX: accept both quote styles; return `0` for "no rate" so the caller falls into the visible USD fallback | `P-C4`, `P-H5` | 1 h |
| 1.5 | Timezone: send `""`, use `time.Local` when empty, surface an unresolvable zone as an error | `C-1`, `P-H2` | 1 h |
| 1.6 | Build the snapshot object, then assign once — never mutate after assignment | `P-H3` | 30 min |
| 1.7 | Replace the phantom `onConfigurationChanged` with declarative `onAccountHomeChanged` / `onCfgCurrencyChanged` | `P-H1` | 30 min |
| 1.8 | Add `"signature": "ssi"` to the `asyncCall` message | `P-H8` | 5 min |
| 1.9 | Key the FX cache by currency (`fxCurrency` in `main.xml`); record `fxFetchedAt` on failure; hourly timer; write config only on change | `P-H4`, `P-H6`, `P-H7` | 1 h |
| 1.10 | In-flight guard: drop replies whose `accountHome` no longer matches | `P-M3` | 30 min |
| 1.11 | Keep Last-Known visible on a rejected call instead of blanking; map unparseable payloads to `unknown_allowance`, not `unbound` | `P-M4`, `P-M6` | 1 h |

**Tests that must land with it:** `expandPath("~/.claude", "file:///home/me")`;
`parseEcbRates` against a verbatim excerpt of the live ECB document;
`usdToDisplayRate` with missing `USD`, missing target, empty map; `computeAt` with an
empty timezone string.

**Done when:** on this machine, from a clean `make install` — the tilde path binds,
a left click opens the popup, `touch`ing a JSONL under `projects/` updates Consumed
Usage within a second (proving `Changed` arrives), and a non-USD Display Currency shows
either a converted amount or a visible USD fallback, never a mislabelled one.

---

## Phase 2 · Stop the bleeding

**Goal:** the helper cannot be wedged, cannot be hung, and cannot be aimed at a path it
should not touch.

| # | Work | Finding | Size |
|---|---|---|---|
| 2.1 | `context.Context` through `Resolve` / `EnsureAccessToken` / `FetchUsage`; `http.NewRequestWithContext`; a client with an explicit `Timeout`. Do the network call **outside** the credentials lock | `C-3` | 3 h |
| 2.2 | Validate `accountHome`: absolute, `filepath.Clean`, length-capped; reject anything else. See [Decisions](#d2--how-far-to-lock-down-accounthome) for how far to go | `C-4`, `M-6`, `H-3` | 2 h |
| 2.3 | Credentials file: `O_NOFOLLOW` + `fstat` (owner, regular, `st_nlink == 1`) on read; `Lstat` the destination before `Rename` | `C-4` | 2 h |
| 2.4 | Replace the lockfile with `syscall.Flock` on the credentials file — the kernel releases it on process death, which deletes the whole stale-lock and PID-reuse class | `M-1` | 2 h |
| 2.5 | Log walk: filter on `d.Type().IsRegular()`; open with `O_NOFOLLOW\|O_NONBLOCK` and `fstat` for `S_ISREG` | `H-1` | 1 h |
| 2.6 | Bound the watcher registry (small LRU, evict); only watch a home that passed classification; drop watches for homes that disappear | `H-3` | 3 h |
| 2.7 | Bound the poller registry the same way; per-account timeouts; wire up `Stop` | `H-4` | 2 h |
| 2.8 | `signal.NotifyContext` for SIGTERM/SIGINT; close the bus, the watch manager and the poller on the way out; sweep stale `.credentials.*.tmp` at startup | `L-6`, `L-3` | 1 h |
| 2.9 | Force-refresh only on 401; treat 403/429 as transient with exponential backoff and a per-hour refresh cap; honour `Retry-After` | `M-2` | 2 h |
| 2.10 | Last-known cache: `os.CreateTemp` instead of a fixed `.tmp` name; verify `account_home` on load; reject a future `fetched_at`; refuse a world-writable cache root | `M-3`, `M-4`, `M-5` | 2 h |
| 2.11 | `truncate` the body in `client.go:67`; add `CheckRedirect` returning `ErrUseLastResponse` | `L-1`, `L-4` | 20 min |

**Tests that must land with it:** the concurrency test (`GetSnapshot` in parallel with
`pollAccountHome`, under `-race`); a `poller_test.go` covering the five 0 % functions; a
FIFO in `projects/` must not hang `Compute`; a symlinked `.credentials.json` must be
refused; `RefreshAccessToken` on both the success and `ErrRefreshRejected` paths.

**Done when:** killing the helper with `SIGKILL` mid-refresh and restarting it recovers
with no manual cleanup, and a blackholed `api.anthropic.com` degrades to Last-Known
within the timeout instead of wedging.

---

## Phase 3 · Move the work off the D-Bus call

**Goal:** implement ADR 0011 as written — *"the helper owns polling and file watch"*.
`GetSnapshot` becomes a cache read.

Land as one reviewable change; the pieces do not work separately.

| # | Work | Finding |
|---|---|---|
| 3.1 | A per-Account-Home snapshot cache in the helper, guarded by a mutex, with a `fetched_at` and a computed-at stamp | `C-2`, `H-5` |
| 3.2 | `GetSnapshot` serves the cache and returns immediately; on a cold miss it returns the classified face with a `pending` marker and kicks a background refresh | `C-2`, `M-7` |
| 3.3 | One background refresh loop per bound Account Home: allowance on the ADR 0006 cadence, usage on a debounced watcher signal. `Changed` is emitted by the loop, not the method | `C-2`, `H-2` |
| 3.4 | Debounce/coalesce fsnotify events (~1 s) before recomputing | `H-2` |
| 3.5 | Incremental log parsing: per-file offset + mtime, parse only appended bytes; cap total bytes and file count per pass; bound the `seen` dedup set to the active windows | `C-2`, `L-16` |
| 3.6 | Admission control: cap concurrent `GetSnapshot` work | `M-6` |
| 3.7 | Add `schema_version` to the snapshot, plus a `degraded` field naming what failed, so the UI can distinguish "used nothing" from "could not read" | `M-7`, `L-15` |
| 3.8 | Surface `degraded` in the plasmoid — one quiet line, per the HIG rules in `design.md:73` | `M-7` |

**Measure it.** Land a benchmark with the change, against a generated Account Home of
~300 MB. Current baseline, measured during the audit:

```
usage.Compute   2.54 s/op   825 MB allocated/op   1.80M allocs/op   (per GetSnapshot)
```

**Done when:** `GetSnapshot` returns in single-digit milliseconds against that corpus,
an active Claude Code session produces at most one recompute per second, and the
benchmark is in CI so a regression is visible.

---

## Phase 4 · Make it installable

**Goal:** someone who is not you can install this and have it work.

| # | Work | Finding | Size |
|---|---|---|---|
| 4.1 | `SystemdService=codecap.service` in the D-Bus file; `[Install] WantedBy=default.target` in the unit; exit 0 rather than 1 when the bus name is already owned | `H-6` | 30 min |
| 4.2 | Template `@BINDIR@` into both service files at install time; take `PREFIX` in `verify-install.sh` | `H-11` | 1 h |
| 4.3 | Split `install` from `build` in the Makefile so `sudo make install` cannot rebuild as root | `H-9` | 30 min |
| 4.4 | Stop shipping `plasmoid/test/`; add the negative assertion to `verify-install.sh` | `H-10` | 30 min |
| 4.5 | Rewrite `PKGBUILD`: `depends=('libplasma' 'plasma-workspace' 'kirigami' 'dbus')`, real `source=`/`sha256sums=`, `$srcdir` not `$startdir`, add `check()` | `H-8` | 2 h |
| 4.6 | CI assertion that `PKGBUILD` `pkgver` matches `metadata.json` `Version` | `H-8` | 20 min |
| 4.7 | `make uninstall` + `scripts/uninstall.sh` (binary, both service files, plasmoid tree, `~/.cache/codecap`); confirm before the `rm -rf` in `install.sh` | `M-13` | 1 h |
| 4.8 | Extend `verify-install.sh`: binary mode, `BusName` ↔ `Name` consistency, `metadata.json` validity via `kpackagetool6 --show` | `L-20`, `P-M13` | 1 h |
| 4.9 | Notification-area keys in `metadata.json`; bind `Plasmoid.status`; fill in `Authors`/`Website`/`BugReportUrl`/`FormFactors` | `P-M13`, `P-M14`, `P-L7` | 1 h |
| 4.10 | Anchor `/pkg/` and `/src/` in `.gitignore` | `L-19` | 2 min |

**Done when:** `make install PREFIX=/usr/local DESTDIR=…` produces a tree whose service
files point at the right binary and `verify-install.sh` passes for both prefixes; and
`makepkg` builds, runs `check()`, and installs a package that resolves on Plasma 6.

---

## Phase 5 · Test depth

**Goal:** the four Phase-1 criticals could not have shipped.

| # | Work | Finding |
|---|---|---|
| 5.1 | One golden snapshot fixture per face, generated from `internal/snapshot`, consumed by the Go tests, `logic.test.mjs`, **and** an assertion on `Export`'s introspection XML. Contract drift becomes impossible | `M-8`, `H-7` |
| 5.2 | `qmltestrunner6` under `QT_QPA_PLATFORM=offscreen` for the pure components — `CompactRing` and `AllowanceBar` take only plain properties, so colour/visibility/geometry are testable with no D-Bus | `P-M · coverage` |
| 5.3 | `dbus-run-session` + a stub `dev.codecap.Helper` driving `main.qml`. This is what covers the failure and partial-data faces | `P-C2`, `P-C3`, `P-H3`, `P-M3`–`P-M6` |
| 5.4 | Stop monkey-patching `Number.prototype.toLocaleString`; inject a formatting seam so the tests exercise real behaviour | `P-M7` |
| 5.5 | Fuzz `FromUsagePayload` and `consumeReader` — both parse external JSON | `M-9` |
| 5.6 | Table tests for the untested branches: `normalizeWeekStart` (22 %), `forModel` fallback (40 %), `parseExpiresAt` (25 %), `DefaultCacheRoot` (0 %) | `M-15`, `L-9` |
| 5.7 | Nested-directory watch test — a `projects/<new>/` created *after* `Ensure`, which is the real refresh path when Claude Code starts a project | `M-9` |
| 5.8 | Replace the package-level `stat`/`readFile` seams in `face` with injected dependencies; give `dbusapi.Export` constructor injection so it is testable at all | `M-14` |
| 5.9 | Coverage threshold at 60 %, ratcheting. Only after the above — adding it first just cements today's number | `M-1` |

---

## Phase 6 · Polish

| # | Work | Finding |
|---|---|---|
| 6.1 | Fix the allowance bar fill opacity — the primary colour signal currently renders at 35 % | `P-M1` |
| 6.2 | Repaint the ring on `ringColor` change; delete the four dead theme `Connections` | `P-M2` |
| 6.3 | `toLocaleString(locale, 'f', 0)` for tokens; `toLocaleCurrencyString` for money | `P-M7`, `P-M8` |
| 6.4 | Return `{hours, minutes}` from `logic.js`, format with `i18nc` in QML — `.pragma library` has no `i18n()` | `P-M9` |
| 6.5 | `helpfulAction` on the Unbound placeholder | `P-M15` |
| 6.6 | `PlasmaExtras.Representation` instead of `Kirigami.ScrollablePage`; explicit size hints | `P-M16` |
| 6.7 | Drop the poll to a 2–5 min safety net once `Changed` works; gate the `nowUnix` ticker on visibility | `P-M11` |
| 6.8 | RTL mirroring; HiDPI ring geometry; compact-numeral threshold | `P-L3`, `P-L4`, `P-L5` |
| 6.9 | Rate table: name the models explicitly, and make an unknown model visibly an estimate rather than silently Sonnet-priced | `M-15` |
| 6.10 | XHR timeout, abort, and `Component.onDestruction` cleanup | `P-L2` |
| 6.11 | Remaining `L-` items: `errors.Join`, the `time.Now()` seam, `parseExpiresAt(nil)`, the `.In(loc)` no-op, the `"/"` account label, unqualified property access | `L-8`–`L-14`, `P-L6` |

---

## Decisions needed

Four things the audit cannot settle. Each blocks a specific item above.

### D1 · The spoofed User-Agent
`internal/allowance/client.go:18-19` sends `claude-code/2.1.0`, with a comment saying the
purpose is to avoid rate-limiting on an endpoint the code itself calls unofficial. That is
an account-suspension risk carried by every user who installs this, and it is disclosed
nowhere in the README or the design docs. It also feeds `M-2`: a 403 from the resulting
anti-abuse handling is exactly what drives refresh-token churn.

*Recommendation:* send an honest `codecap/<version> (+<url>)`, accept the rate limit, and
back off properly. If the endpoint turns out to be unusable that way, document the risk
prominently in the README rather than leaving it in a code comment. **Blocks 2.9.**

### D2 · How far to lock down `accountHome`
`C-4` is a confused deputy: the helper lends its filesystem authority to any session-bus
peer. Three options —

1. **Allowlist.** The helper keeps its own list of Account Homes and the bus argument only
   selects among them. Strongest; changes ADR 0011's contract.
2. **Validate + `O_NOFOLLOW`.** Keep the argument, require absolute/clean/bounded, refuse
   symlinked credentials, verify ownership. Closes the credential-theft path; leaves the
   file-creation primitive (`M-8`) partly open.
3. **Accept and document.** Note that any session-bus peer is trusted, and rely on the
   session bus boundary.

*Recommendation:* 2 now (it is Phase 2 work regardless), 1 when a second Account Home
becomes a real use case. **Blocks 2.2.**

### D3 · Does Usage Credit colour the ring?
`design.md:81` reads "**100% used or Usage Credit** → `negativeTextColor`".
`logic.js:105-110` implements only the first half — the second condition is dead code
strictly subsumed by the first, which suggests the intent was `usageCredit !== "none"`.
`logic.test.mjs:65` locks in the current behaviour. The visual spec is frozen; this is
yours to read. **Blocks 6.1's test updates.**

### D4 · Em dash or question icon for Unknown Allowance
`CONTEXT.md:72` says "Compact shows an em dash"; `design.md:77`, frozen a day later, says
`question-symbolic`; `logic.js:84` implements the icon. One of the two frozen documents
needs a revision line. Related: `design.md:79` keeps Account Home out of the tooltip, but
`main.qml:245-250` puts `account_label` in `toolTipSubText`.

---

## ADRs this implies

Written by you, not by the audit — listing them so they are not forgotten:

- **Timezone on the bus.** Phase 1.5 changes the contract: empty string means "helper's
  local zone". ADR 0011's bus contract paragraph needs the line.
- **Snapshot schema versioning and degraded reporting.** Phase 3.7 adds two fields to a
  contract that two separately installed artifacts share. That is exactly what an ADR is
  for.
- **The Account Home trust boundary.** Whatever D2 resolves to. ADR 0009 and 0010 both
  assume the plasmoid is the only caller; nothing states it as a decision.

---

## Effort

Rough, assuming one person who knows this codebase:

| Phase | | |
|---|---|---|
| 0 | Gates | ~1 day (0.6 carries the risk) |
| 1 | Make it work | ~1½ days |
| 2 | Stop the bleeding | ~3 days |
| 3 | Move the work | ~4 days |
| 4 | Make it installable | ~1½ days |
| 5 | Test depth | ~4 days (5.3 carries the risk) |
| 6 | Polish | ~2 days |

Phases 0–2 — a working, safe widget — are about a week. Phases 0–4, which is what a v1.1
release needs, are about two.

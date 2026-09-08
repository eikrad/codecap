# Working on codecap

A Plasma 6 QML applet plus a Go session helper. Product language is
[`CONTEXT.md`](./CONTEXT.md), architecture is [`docs/design.md`](./docs/design.md),
decisions are [`docs/adr/`](./docs/adr/), work order is
[`docs/roadmap.md`](./docs/roadmap.md) and [`docs/plan-hardening.md`](./docs/plan-hardening.md).

`CONTEXT.md` and the visual section of `design.md` are **frozen**. Use their terms
exactly — Account Home, Allowance, Consumed Usage, List Price, Face — in type names,
JSON keys and UI copy. Changing them is a glossary revision, never a side effect of
implementation.

## Read this part first

On 2026-08-22 an audit found eight critical bugs in code that had a green pipeline,
15 ADRs and a complete design document. Four of them meant the widget did not work at
all: the popup could not be opened, the push signal never arrived, the documented
`~/.claude` path was rejected, and non-USD users saw USD amounts labelled with their
own currency. Every one is written up in [`docs/audit-2026-08-22.md`](./docs/audit-2026-08-22.md).

They were not eight separate mistakes. They were four habits:

1. **Plausible API names nothing could contradict.** `onReceivedSignal`,
   `onConfigurationChanged`, `onHighlightColorChanged` are not typos — they are what
   anyone would guess. None of them exist.
2. **Tests written from the same assumption as the code.** The ECB fixture was
   hand-written with the same wrong quote style as the regex it tested. The token
   formatter was tested against a stub with different defaults from the real one.
3. **Silent fallbacks that turn "I don't know" into a plausible number.** An
   unresolvable timezone became UTC. A missing exchange rate became 1.0.
4. **Contracts that lived only in prose.** ADR 0011 says the helper owns polling; the
   code did it inside the D-Bus method instead.

The rules below exist because of those four. Each one names the finding it comes from.

## Hard rules

### Never write a platform API name you have not seen in a source

`P-C2`, `P-H1`, `P-M2`, `P-C1`, `P-M7`.

Applies to every Plasma, Kirigami, Qt and QML identifier: signal handler names,
attached properties, enum values, return types, and the default arguments of built-in
methods. Qt's QML API is **not** the same as its C++ API — `StandardPaths.writableLocation`
returns a `url` in QML and a `QString` in C++, and QML's `Number.toLocaleString`
defaults to two decimal places where ECMAScript's does not.

Before writing such a name, confirm it in one of:

- the upstream header or `.cpp` (`libplasma/src/plasma/applet.h`,
  `plasma-workspace/components/dbus/`, `kirigami/src/platform/platformtheme.h`)
- an in-tree Plasma applet doing the same thing
- the type's `.qmltypes` / qmldoc

`SignalWatcher` is the cautionary example: it has no `receivedSignal` signal at all.
It looks up a function named `"dbus" + member` and calls it with the decoded
arguments. A handler named anything else is silently never called — no warning, no
error, the feature simply does not exist.

**`qmllint` passing does not mean the QML is correct.** In CI it runs as a syntax and
structure gate only: Plasma and Kirigami types cannot be resolved there, so
import, type, property and unqualified checks are switched off (see `Makefile`).
A wrong property or handler name on a Plasma type passes it.

### A fallback is visible or it is an error — never a plausible default

`C-1`, `P-H5`, `P-M6`, `M-7`.

When a value cannot be determined, the code must either surface that fact or return
an error. It must never substitute something that looks like a real answer.

```go
// No. Every user outside UTC silently got the wrong day, week and month.
loc, err := time.LoadLocation(timezone)
if err != nil {
    loc = time.UTC
}
```

```js
// No. 1.0 is indistinguishable from a real USD rate, so the caller labelled
// raw USD amounts with the user's currency code.
if (!rates[targetCurrency]) {
    return 1.0
}
```

Ask: *if this fallback fires, can the user tell?* If not, it is a bug even when every
test passes. The user reading this widget cannot check the number against anything —
that is the entire reason the widget exists.

### A fixture is a verbatim excerpt of the real artifact

`P-C4`.

Never hand-write a sample of an external format from memory. Paste a real one. The
ECB feed uses double-quoted attributes; the regex accepted only single quotes; the
test fixture was written with single quotes by the same author, so it passed while
production was broken for every non-USD user.

If the real artifact cannot be fetched, say so in the test comment and treat the
parser as unverified.

### Never stub a platform global in a test

`P-M7`.

`Number.prototype.toLocaleString` was replaced with a Node function whose
`format`/`precision` defaults differ from QML's. The test then asserted the stub's
behaviour. Inject a seam instead, or test the caller's output shape rather than the
platform's formatting.

### Before writing a test, ask what it would take to fail

`P-C4`, `P-M5`, `P-M7`.

A test written from the same mental model as the code proves the model is
self-consistent, not that it is right. Three tests in this repo asserted the buggy
behaviour as correct. For each new test: *if my understanding of the platform is
wrong, does this test go red?* If not, it is documentation, not verification.

### A handler reads its own input, never a value derived from it

`ADR 0016`.

QML bindings are push-based. Inside `onXChanged`, every binding that depends on
`x` still holds its previous value, and reading one is reading stale state.

```qml
// No. isBound is a binding on accountHome, so this took the unbound branch
// and the widget never called the helper at all.
readonly property bool isBound: accountHome.trim() !== ""
onAccountHomeChanged: refresh()
function refresh() { if (!isBound) { applyLocalFace("unbound"); return } }
```

The same rule covers what a platform hands a handler. `SignalWatcher` decodes a
`"s"` argument to `{value: "..."}`, not to a string, so `arg === accountHome`
was false for every `Changed` the helper ever sent — the right handler name, and
still no push. Decode explicitly in `logic.js` against a shape captured from a
real bus, the way `extractSnapshotPayload` and `signalAccountHome` do.

Neither of these is visible to `qmllint` or to the Node tests.
`make test-plasmoid-qml-dbus` is the only gate that can see them.

### QML: build the object, then assign it once

`P-H3`.

Assigning a `var` property fires the change signal. Mutating that object afterwards
fires nothing, so every binding keeps seeing the first value.

```qml
// No. Bindings observe face === "unbound" forever, and the popup renders blank.
snapshot = Logic.emptySnapshot()
snapshot.face = "unknown_allowance"
```

### Every ADR claim needs something that executes

`C-2`, `H-6`, `M-8`.

ADR 0015 promised systemd would restart the helper on crash. The D-Bus service file
had no `SystemdService=` and the unit had no `[Install]`, so systemd was never
involved at all. ADR 0011 says the helper owns polling and file watch; the code did
both synchronously inside the D-Bus method.

When you implement an ADR, add the assertion that would fail if the claim stopped
being true — a test, a `verify-install.sh` check, a CI step. If you cannot, write
down in the ADR that the claim is unverified.

Contracts stated in more than one place get one shared fixture, not two hand-written
copies. The snapshot JSON currently exists in `internal/snapshot`, in `logic.js`, in
`design.md` and in the introspection XML.

### Account Home is caller input

`C-4`, `M-6`.

`GetSnapshot` takes a path from any session-bus peer and the helper uses it as a
filesystem base. Validate it through `internal/accounthome`. Open credentials with
`O_NOFOLLOW|O_NONBLOCK` and check owner, regular-file and link count — renaming over a
symlink replaces the link, not its target. `O_NONBLOCK` is not optional: opening a
FIFO blocks *before* any regular-file check can reject it.

### Anything unbounded gets a bound; anything that blocks gets a deadline

`C-3`, `H-1`, `H-3`, `H-4`.

There was no `http.Client` timeout and no `context.Context` anywhere in this tree. Use
`internal/httpx`. Registries keyed on caller input are capped with eviction
(`internal/allowance`, `internal/usagewatch`) — inotify instances are a per-user
resource, so exhausting them breaks the whole desktop session, not just this helper.

## Before you say it works

```
make lint     # gofmt, go vet, golangci-lint+gosec, shellcheck, qmllint
make test     # go test -race, node --test, qmltestrunner
make ci       # the above plus the install-layout check
```

Two of the QML suites skip themselves unless the machine can display the widget,
and both print why rather than passing quietly:

```
make test-plasmoid-qml-plasma  # needs Kirigami
make test-plasmoid-qml-dbus    # needs org.kde.plasma.workspace.dbus (Plasma 6)
```

`test-plasmoid-qml-dbus` is the only thing here that executes the plasmoid's
D-Bus code. It starts a private session bus and a stub `dev.codecap.Helper`
(`tests/helperstub`), then drives the applet's own `SnapshotSource` against it.
**Run it on the desktop after any change to `SnapshotSource.qml`.** In CI it
gets as far as building the stub and taking the bus name, and then skips.

`scripts/pre-pr-check.sh` runs all of that in one go and says which gates it
could not run. A `PreToolUse` hook in `.claude/settings.json` runs it before
`gh pr create` and blocks the PR if anything fails. It is not a substitute for
CI: golangci-lint, shellcheck and reuse are frequently absent from a developer
machine, and the script reports them as skipped rather than as passed.

Green does not mean done. Also state, in the message you hand back:

- **what you could not verify, and why.** No Plasma session, no session bus, no real
  ECB response — say it. Four of the eight critical findings were non-existent names,
  and no gate in this repo can catch that class. A confident report on unverifiable
  QML is worse than an honest one.
- **which behaviour a human has to check on a real desktop**, concretely enough to
  follow: bind the path, click the widget, touch a JSONL and watch the number move.

Never report a phase complete on the strength of the pipeline alone.

## Conventions

- Licence header on every file, or a `REUSE.toml` entry. `reuse lint` is in CI.
- Comments explain *why*, especially where the obvious code is wrong — the
  `O_NONBLOCK` above is worth a line, `// increment i` is not.
- The plasmoid never reads credentials and never holds a token (ADR 0007, 0008).
  Keep vendor HTTP in the helper.
- Testable logic goes in `logic.js`, not in `.qml` — it is the only plasmoid code CI
  can actually execute. What genuinely has to be QML goes in a component taking
  plain properties, like `CompactRing` and `SnapshotSource`, so a test runner can
  instantiate it. `main.qml` itself never can: `PlasmoidItem` only works inside
  Plasma's applet machinery, and `Plasmoid.configuration` is null anywhere else.
- Decisions belong in an ADR. If you find yourself choosing between two designs
  mid-implementation, stop and ask.

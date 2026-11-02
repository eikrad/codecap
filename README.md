# codecap

[![CI](https://github.com/eikrad/codecap/actions/workflows/ci.yml/badge.svg)](https://github.com/eikrad/codecap/actions/workflows/ci.yml)

Plasma 6 applet plus a Go session helper that shows coding-agent usage and allowance.

Supports **Claude Code and Codex** on **Plasma 6**. One widget instance is bound to one Account Home (for example `~/.claude` or `~/.codex`). The Agent is identified from that directory's artifacts, so two widget instances can show separate Claude and Codex Accounts.

## What you see

**Compact (panel / tray)** — a Session Allowance ring: percent used and time until reset. Hover for a short tooltip. The widget uses Plasma theme colours and never shows a fake 0% ring when data is missing.

**Expanded (click the widget)** — Session and Weekly Allowance bars, Usage Credit status (if applicable), and Consumed Usage from local logs:

| Period | Shows |
|--------|--------|
| Session | List Price + tokens (local session window) |
| Today | List Price + tokens |
| This week | List Price + tokens |
| This month | List Price + tokens |

List Price is estimated at published API token rates (USD in the helper). The widget converts to your **Display Currency** (locale default, overridable in Configure).

**Allowance** (plan remaining) comes from the Agent's subscription signal and includes usage on other devices. **Consumed Usage** comes only from logs on this machine — so both numbers together answer “what did I use here?” and “how much plan is left overall?”.

For Codex, the helper reads live Allowance through the installed `codex app-server`
(verified with Codex 0.153.4). The primary five-hour and secondary seven-day
windows appear as Session and Weekly Allowance, just as for Claude. Missing or
different windows show Unknown Allowance; a failed read can show visibly stale
Last-Known Allowance. Local `sessions` and `archived_sessions` logs supply
Consumed Usage. List Price uses Standard published USD rates when the log does
not identify a service tier. Unknown models or unpriceable cache writes show
"List Price unavailable" while retaining tokens.

To distinguish widget instances, open **Configure widget → Widget marker** and
enable **Show color marker**. Choose a color independently for each widget.
The marker is off by default and appears beside the compact ring and expanded
Account label. Session percentages and Allowance colors keep their meaning.

### When the widget looks “empty”

| What you see | Meaning |
|--------------|---------|
| Folder icon | No Account Home chosen — open **Configure widget** |
| Unlock icon | Account Home exists but you are not signed in with its Agent there |
| Question icon | Helper missing, or Allowance not available yet (Consumed Usage may still update) |
| Ring with % | Signed in and Allowance loaded |

## Requirements

- **KDE Plasma 6** (Plasma 5 is not supported)
- **Claude Code or Codex** with a signed-in Account Home on this machine; live Codex Allowance requires `codex` on the helper's PATH
- **codecap helper** installed (Go session service — see Install). The widget alone is not enough; it talks to the helper over D-Bus and never reads your credentials itself.

## Install

When upgrading for Codex support, update both helper and plasmoid: snapshot
schema 2 adds explicit unavailable pricing. The new plasmoid also accepts
schema-1 snapshots from older helpers, but those helpers remain Claude-only.

### Arch / Manjaro (recommended)

`PKGBUILD` builds from the published release archive and verifies it against a
pinned checksum, so it needs nothing else from this repository:

```bash
curl -O https://raw.githubusercontent.com/eikrad/codecap/main/PKGBUILD
makepkg -si
```

This builds the **released** version, not your working tree — `makepkg` downloads
the tagged tarball rather than using the directory it runs in. To package what
you are actually working on, use `./scripts/install.sh` below.

Or install directly from a clone (helper + plasmoid + D-Bus + systemd in one step):

```bash
git clone https://github.com/eikrad/codecap.git
cd codecap
./scripts/install.sh
```

Equivalent manual install (build as your user, then install — never `sudo make install` alone before `make build`):

```bash
make build
sudo make install
```

`PREFIX` is honoured end-to-end: both service files are templated at install time, so `sudo make install PREFIX=/usr/local` activates `/usr/local/bin/codecap`. Both scripts take it from the environment — `PREFIX=/usr/local ./scripts/install.sh`, and the same value when you later uninstall.

This installs:

- `$PREFIX/bin/codecap` — session helper
- D-Bus activation for `dev.codecap.Helper` (via systemd user unit)
- User systemd unit `codecap.service` (`WantedBy=default.target`)
- Plasmoid under `$PREFIX/share/plasma/plasmoids/dev.codecap.plasmoid/`

To remove:

```bash
./scripts/uninstall.sh
# or: sudo make uninstall
```

CI runs on every push to `main` and on pull requests: `go vet`, `make test`, and `make test-install`.

```bash
make ci             # same checks as CI locally
go test ./...       # helper unit tests only
make test-install   # verify FHS install tree
go run ./cmd/codecap
```

### Other distros

Packaging for Debian/Fedora is not shipped yet. The helper layout is standard FHS, so
a manual install works from the [release tarball](https://github.com/eikrad/codecap/releases):

```bash
curl -L https://github.com/eikrad/codecap/archive/v0.2.0.tar.gz | tar xz
cd codecap-0.2.0
make build
sudo make install PREFIX=/usr/local
systemctl --user daemon-reload
```

That needs Go to build the helper, and Plasma 6 to run the widget — see
[Requirements](#requirements). The plasmoid is a normal Plasma 6 KPackage once the
helper is on `PATH` and D-Bus activation works.

## Configure the widget

1. Add **codecap** from the Plasma widget gallery (panel, desktop, or tray).
2. Right-click the widget → **Configure codecap…**
3. Set **Account Home** to your Agent's directory (Browse… or type `~/.claude` / `~/.codex`).
4. Optionally set **Display Currency** to an ISO code (for example `DKK`, `EUR`). Leave blank to follow your desktop locale.
5. Optionally enable **Widget marker → Show color marker** and choose an individual color.

The widget does not store OAuth tokens — only the path to Account Home and display preferences.

## Privacy and security

- Credentials stay in your Account Home (where Claude Code put them). The helper reads them; the plasmoid does not.
- Allowance is fetched over HTTPS inside the helper only, using the same OAuth session as Claude Code.
- Nothing is sent to third parties except the public FX rate feed (ECB daily XML) for Display Currency conversion.

## Troubleshooting

**New settings do not enable Apply or do not persist after an upgrade**

The running Plasma applet retains the `contents/config/main.xml` schema it loaded
when it started. Newly installed QML can show controls whose keys are still absent
from that in-memory schema. Close the settings dialog and restart Plasma once:

```bash
systemctl --user restart plasma-plasmashell.service
```

Alternatively, log out and back in. Reopening the dialog or restarting only the
codecap helper does not reload the applet's configuration schema. Existing saved
widget preferences are retained.

**Question icon / “Helper unavailable”**  
Install the package or run `go run ./cmd/codecap` for development. Check: `busctl --user status dev.codecap.Helper`.

**Unlock icon / “Signed out”**  
Run `claude` and sign in for that Account Home, or fix the path in Configure.

**Allowance stale or “Last-Known” in tooltip**  
Vendor fetch failed; the helper shows the last good bars for up to an hour. Consumed Usage from logs still updates.

**Widget not in gallery after install**  
Restart `plasmashell` or re-login. If you previously used `kpackagetool6 --install`, run `./scripts/install.sh` so the system-wide copy under `/usr/share` is used instead of a stale user-local one.

**`kpackagetool6` says plasmoid already exists**  
Use `./scripts/install-plasmoid.sh` (upgrade) or `./scripts/install.sh` (full system install, removes user-local copy first).

**Manual snapshot check**

```bash
busctl --user call dev.codecap.Helper /dev/codecap/Helper dev.codecap.Helper \
  GetSnapshot ssi "$HOME/.claude" "Europe/Copenhagen" 1
```

## Development

```bash
go test ./...           # helper unit tests
make test-install       # verify FHS install tree
go run ./cmd/codecap    # helper in foreground

./scripts/install.sh           # full system install (first time)
./scripts/install-plasmoid.sh  # user-local plasmoid only (QML dev)
```

Architecture, domain language, and design decisions: [`docs/design.md`](docs/design.md), [`CONTEXT.md`](CONTEXT.md), [`docs/roadmap.md`](docs/roadmap.md). Cutting a release: [`docs/releasing.md`](docs/releasing.md).

## Contributing

Contributions are welcome — issues, docs, helper fixes, and plasmoid UI improvements.

1. Fork and clone the repo.
2. Create a branch from `main`.
3. Run tests before opening a PR:

   ```bash
   make test
   make test-install
   ```

4. Follow existing conventions: SPDX headers (`GPL-2.0-or-later`), domain terms from `CONTEXT.md`, Go tests at public seams, QML via Kirigami/PlasmaComponents (no hardcoded colours).
5. Open a pull request against `main` with a short description and test plan.

For large changes, check [`docs/roadmap.md`](docs/roadmap.md) and open an issue first if the direction is unclear.

## License

GNU General Public License v2 or later (`GPL-2.0-or-later`). See [`LICENSES/GPL-2.0-or-later.txt`](LICENSES/GPL-2.0-or-later.txt).

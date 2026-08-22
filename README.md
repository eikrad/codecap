# codecap

[![CI](https://github.com/eikrad/codecap/actions/workflows/ci.yml/badge.svg)](https://github.com/eikrad/codecap/actions/workflows/ci.yml)

Plasma 6 applet plus a Go session helper that shows coding-agent usage and allowance.

v1 targets **Claude Code** on **Plasma 6**. One widget instance is bound to one Account Home (for example `~/.claude`).

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

**Allowance** (plan remaining) comes from Claude’s subscription signal and includes usage on other devices. **Consumed Usage** comes only from logs on this machine — so both numbers together answer “what did I use here?” and “how much plan is left overall?”.

### When the widget looks “empty”

| What you see | Meaning |
|--------------|---------|
| Folder icon | No Account Home chosen — open **Configure widget** |
| Unlock icon | Account Home exists but you are not signed in with Claude Code there |
| Question icon | Helper missing, or Allowance not available yet (Consumed Usage may still update) |
| Ring with % | Signed in and Allowance loaded |

## Requirements

- **KDE Plasma 6** (Plasma 5 is not supported)
- **Claude Code** with a signed-in Account Home on this machine
- **codecap helper** installed (Go session service — see Install). The widget alone is not enough; it talks to the helper over D-Bus and never reads your credentials itself.

## Install

### Arch / Manjaro (recommended)

From a clone of this repository:

```bash
git clone https://github.com/eikrad/codecap.git
cd codecap
makepkg -fd
sudo pacman -U codecap-*.pkg.tar.zst
```

Or install directly without a package:

```bash
sudo make install
```

This installs:

- `/usr/bin/codecap` — session helper
- D-Bus activation for `dev.codecap.Helper`
- User systemd unit `codecap.service`
- Plasmoid under `/usr/share/plasma/plasmoids/dev.codecap.plasmoid/`

CI runs on every push to `main` and on pull requests: `go vet`, `make test`, and `make test-install`.

```bash
make ci             # same checks as CI locally
go test ./...       # helper unit tests only
make test-install   # verify FHS install tree
go run ./cmd/codecap
```

### Other distros

Packaging for Debian/Fedora is not shipped yet. The helper layout is standard FHS; a future release tarball will document manual install. The plasmoid is a normal Plasma 6 KPackage once the helper is on `PATH` and D-Bus activation works.

## Configure the widget

1. Add **codecap** from the Plasma widget gallery (panel, desktop, or tray).
2. Right-click the widget → **Configure codecap…**
3. Set **Account Home** to your Claude Code directory (Browse… or type `~/.claude`).
4. Optionally set **Display Currency** to an ISO code (for example `DKK`, `EUR`). Leave blank to follow your desktop locale.

The widget does not store OAuth tokens — only the path to Account Home and display preferences.

## Privacy and security

- Credentials stay in your Account Home (where Claude Code put them). The helper reads them; the plasmoid does not.
- Allowance is fetched over HTTPS inside the helper only, using the same OAuth session as Claude Code.
- Nothing is sent to third parties except the public FX rate feed (ECB daily XML) for Display Currency conversion.

## Troubleshooting

**Question icon / “Helper unavailable”**  
Install the package or run `go run ./cmd/codecap` for development. Check: `busctl --user status dev.codecap.Helper`.

**Unlock icon / “Signed out”**  
Run `claude` and sign in for that Account Home, or fix the path in Configure.

**Allowance stale or “Last-Known” in tooltip**  
Vendor fetch failed; the helper shows the last good bars for up to an hour. Consumed Usage from logs still updates.

**Widget not in gallery after install**  
Restart `plasmashell` or re-login.

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

kpackagetool6 --type Plasma/Applet --install ./plasmoid   # user-local plasmoid
# use --upgrade on later runs
```

Architecture, domain language, and design decisions: [`docs/design.md`](docs/design.md), [`CONTEXT.md`](CONTEXT.md), [`docs/roadmap.md`](docs/roadmap.md).

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

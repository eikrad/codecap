# codecap

Plasma 6 applet plus a Go session helper that shows coding-agent usage and allowance.

The applet is QML-only and talks to a local session helper over D-Bus. The helper owns Account Home access and never stores credentials in plasmoid config.

## Status

Roadmap phase 4 in progress on `feature/plasmoid-ui`:

- helper: Consumed Usage + Session / Weekly Allowance over D-Bus
- plasmoid: Session ring, tooltip, expanded bars + Consumed Usage form, Configure, FX in QML

## Run helper (development)

```bash
go run ./cmd/codecap
```

## Manual smoke

```bash
go run ./cmd/codecap
busctl --user call dev.codecap.Helper /dev/codecap/Helper dev.codecap.Helper GetSnapshot ssi "$HOME/.claude" "Europe/Copenhagen" 1
busctl --user call dev.codecap.Helper /dev/codecap/Helper dev.codecap.Helper GetSnapshot ssi "" "Europe/Copenhagen" 1
kpackagetool6 --type Plasma/Applet --install ./plasmoid
```

Then add the `codecap` widget in Plasma:

- empty Account Home → Unbound placeholder (folder icon)
- helper stopped → Unknown Allowance (question icon)
- signed in with helper running → Session ring and expanded Allowance bars
- Configure: Account Home folder + Display Currency override

## License

GNU General Public License v2 or later (`GPL-2.0-or-later`).

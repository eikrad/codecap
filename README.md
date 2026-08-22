# codecap

Plasma 6 applet plus a Go session helper that shows coding-agent usage and allowance.

The applet is QML-only and talks to a local session helper over D-Bus. The helper owns Account Home access and never stores credentials in plasmoid config.

## Status

This repository is currently implementing roadmap phase 3+:

- helper skeleton (`GetSnapshot` / `Changed`)
- Consumed Usage from Account Home logs
- Session / Weekly Allowance via unofficial OAuth usage fetch + Last-Known cache
- smoke plasmoid to verify D-Bus integration (visual rings/bars still phase 4)

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

- empty Account Home → Unbound placeholder
- helper stopped → Unknown Allowance
- helper running with Account Home set → shows returned face JSON

## License

GNU General Public License v2 or later (`GPL-2.0-or-later`).

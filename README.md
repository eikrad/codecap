# codecap

Plasma 6 applet plus a Go session helper that shows coding-agent usage and allowance.

The applet is QML-only and talks to a local session helper over D-Bus. The helper owns Account Home access and never stores credentials in plasmoid config.

## Status

This repository is currently implementing roadmap phases 0-1:

- repo hygiene
- helper skeleton (`GetSnapshot` / `Changed`)
- smoke plasmoid to verify D-Bus integration

## Run helper (development)

```bash
go run ./cmd/codecap
```

## Manual smoke

```bash
busctl --user call dev.codecap.Helper /dev/codecap/Helper dev.codecap.Helper GetSnapshot ssi "$HOME/.claude" "Europe/Copenhagen" 1
```

## License

GNU General Public License v2 or later (`GPL-2.0-or-later`).

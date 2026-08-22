# codecap

Plasma 6 applet plus a Go session helper that shows coding-agent usage and allowance.

The applet is QML-only and talks to a local session helper over D-Bus. The helper owns Account Home access and never stores credentials in plasmoid config.

## Status

v1 roadmap complete through phase 5 (install). Next optional channel: AUR publish, then Get New Widgets once helper install is documented for non-Arch.

## Install (Arch / Manjaro)

Build and install from a checkout:

```bash
make test          # Go unit tests
make test-install  # verify FHS tree under DESTDIR
sudo make install  # /usr/bin/codecap, D-Bus service, user unit, plasmoid
```

Or build a local package:

```bash
makepkg -fd
sudo pacman -U codecap-0.2.0-1-*.pkg.tar.zst
```

After install, log out and back in (or restart `plasmashell`) so Plasma picks up the plasmoid. The helper starts on first widget D-Bus call; optionally keep it running across crashes:

```bash
systemctl --user enable --now codecap.service
```

Add the **codecap** widget from the widget gallery and set Account Home in Configure (for example `~/.claude`).

## Development

Run helper without installing:

```bash
go run ./cmd/codecap
```

Install plasmoid to your user for iteration:

```bash
kpackagetool6 --type Plasma/Applet --install ./plasmoid
# or --upgrade on later runs
```

Manual D-Bus smoke:

```bash
busctl --user call dev.codecap.Helper /dev/codecap/Helper dev.codecap.Helper GetSnapshot ssi "$HOME/.claude" "Europe/Copenhagen" 1
```

## License

GNU General Public License v2 or later (`GPL-2.0-or-later`).

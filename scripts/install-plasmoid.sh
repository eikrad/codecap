#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
PLASMOID="$ROOT/plasmoid"

if ! command -v kpackagetool6 >/dev/null 2>&1; then
	echo "kpackagetool6 not found (install plasma-framework)" >&2
	exit 1
fi

cd "$ROOT"

if [ -d "$HOME/.local/share/plasma/plasmoids/dev.codecap.plasmoid" ]; then
	kpackagetool6 --type Plasma/Applet --upgrade "$PLASMOID"
	echo "Plasmoid upgraded in ~/.local/share/plasma/plasmoids/dev.codecap.plasmoid"
else
	kpackagetool6 --type Plasma/Applet --install "$PLASMOID"
	echo "Plasmoid installed in ~/.local/share/plasma/plasmoids/dev.codecap.plasmoid"
fi

cat <<EOF

User-local plasmoid updated. The helper is unchanged — install it once with:
  ./scripts/install.sh

If Plasma was already running, restart its shell to load the new QML and settings:
  systemctl --user restart plasma-plasmashell.service
Or log out and back in. Existing saved widget preferences are retained.

New settings may otherwise appear without enabling Apply: reopening the settings
dialog or restarting the helper does not reload Plasma's cached main.xml schema.
EOF

#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
LOCAL_PLASMOID="$HOME/.local/share/plasma/plasmoids/dev.codecap.plasmoid"

cd "$ROOT"

echo "Building codecap helper..."
make build

echo "Installing system-wide (sudo required)..."
sudo make install

# The helper stays up for the graphical session (ADR 0015), so installing a new
# binary leaves the old process serving until logout. Stop it; the next widget
# call re-activates it over D-Bus, now running what was just installed.
if command -v pkill >/dev/null 2>&1; then
	if pkill -x codecap 2>/dev/null; then
		echo "Stopped the running helper; it restarts on the next widget call."
	fi
fi

if [ -d "$LOCAL_PLASMOID" ]; then
	echo "Removing user-local plasmoid (system install uses /usr/share)..."
	if command -v kpackagetool6 >/dev/null 2>&1; then
		kpackagetool6 --type Plasma/Applet --remove dev.codecap.plasmoid
	else
		rm -rf "$LOCAL_PLASMOID"
	fi
fi

cat <<EOF

codecap installed.

Next steps:
  1. Add the widget from the Plasma gallery (search "codecap").
  2. Configure Account Home (for example ~/.claude).
  3. If the widget was already on a panel, restart plasmashell or re-login.

Helper check:
  busctl --user call dev.codecap.Helper /dev/codecap/Helper dev.codecap.Helper \\
    GetSnapshot ssi "\$HOME/.claude" "UTC" 1

Plasmoid-only dev updates (after this install):
  ./scripts/install-plasmoid.sh
EOF

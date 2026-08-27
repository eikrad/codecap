#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
PREFIX="${PREFIX:-/usr}"
LOCAL_PLASMOID="$HOME/.local/share/plasma/plasmoids/dev.codecap.plasmoid"

cd "$ROOT"

echo "Building codecap helper..."
make build

echo "Installing into $PREFIX (sudo required)..."
sudo make install PREFIX="$PREFIX"

# systemd caches the unit tree per user manager. Without a reload it does not
# know codecap.service exists until the next login, so the D-Bus activation
# that SystemdService= points at fails for the rest of this session — which
# looks exactly like the helper being broken.
if command -v systemctl >/dev/null 2>&1; then
	if ! systemctl --user daemon-reload; then
		echo "WARNING: systemd user units could not be reloaded; re-login before using codecap." >&2
	fi
fi

# The helper stays up for the graphical session (ADR 0015), so installing a new
# binary leaves the old process serving until logout. Stop it; the next widget
# call re-activates it over D-Bus, now running what was just installed.
if command -v pkill >/dev/null 2>&1; then
	if pkill -x codecap 2>/dev/null; then
		echo "Stopped the running helper; it restarts on the next widget call."
	fi
fi

if [ -d "$LOCAL_PLASMOID" ]; then
	printf "Remove user-local plasmoid at %s so the system package is used? [y/N] " "$LOCAL_PLASMOID"
	read -r answer
	case "$answer" in
	y | Y | yes | YES)
		if command -v kpackagetool6 >/dev/null 2>&1; then
			kpackagetool6 --type Plasma/Applet --remove dev.codecap.plasmoid
		else
			rm -rf -- "$LOCAL_PLASMOID"
		fi
		;;
	*)
		echo "Keeping $LOCAL_PLASMOID (it shadows the system install)"
		;;
	esac
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

To remove later:
  ./scripts/uninstall.sh
EOF

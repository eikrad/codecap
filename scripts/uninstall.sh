#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later
#
# Remove a system install of codecap and the optional per-user cache.
# Does not touch Account Homes (credentials / logs stay where Claude Code put them).

set -eu

ROOT="$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)"
PREFIX="${PREFIX:-/usr}"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/codecap"
LOCAL_PLASMOID="$HOME/.local/share/plasma/plasmoids/dev.codecap.plasmoid"

cd "$ROOT"

echo "Removing system files (sudo required)..."
sudo make uninstall PREFIX="$PREFIX"

# The helper stays up for the graphical session (ADR 0015). Stop it so a
# leftover process is not serving after the binary is gone.
if command -v systemctl >/dev/null 2>&1; then
	systemctl --user disable --now codecap.service 2>/dev/null || true
fi
if command -v pkill >/dev/null 2>&1; then
	pkill -x codecap 2>/dev/null || true
fi

if [ -d "$LOCAL_PLASMOID" ]; then
	printf "Remove user-local plasmoid at %s? [y/N] " "$LOCAL_PLASMOID"
	read -r answer
	case "$answer" in
	y | Y | yes | YES)
		if command -v kpackagetool6 >/dev/null 2>&1; then
			kpackagetool6 --type Plasma/Applet --remove dev.codecap.plasmoid || \
				rm -rf -- "$LOCAL_PLASMOID"
		else
			rm -rf -- "$LOCAL_PLASMOID"
		fi
		;;
	*)
		echo "Keeping $LOCAL_PLASMOID"
		;;
	esac
fi

if [ -d "$CACHE_DIR" ]; then
	printf "Remove Last-Known Allowance cache at %s? [y/N] " "$CACHE_DIR"
	read -r answer
	case "$answer" in
	y | Y | yes | YES)
		rm -rf -- "$CACHE_DIR"
		;;
	*)
		echo "Keeping $CACHE_DIR"
		;;
	esac
fi

cat <<EOF

codecap uninstalled from $PREFIX.

Account Homes were not touched. Sign-in state for Claude Code is unchanged.
EOF

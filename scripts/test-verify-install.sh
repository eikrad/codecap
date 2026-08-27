#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later
#
# Negative tests for verify-install.sh.
#
# Every run of that script in CI has been against a tree that was already
# correct, so a check which quietly stopped checking — a `grep` for a line that
# no longer exists anywhere, a `case` glob that matches everything — would look
# exactly like a passing gate. Each case below breaks one thing the gate claims
# to catch and requires a non-zero exit.
#
# Usage: test-verify-install.sh GOOD_DESTDIR [PREFIX]

set -eu

GOOD="${1:?usage: test-verify-install.sh GOOD_DESTDIR [PREFIX]}"
PREFIX="${2:-/usr}"
GOOD="${GOOD%/}"
PREFIX="/${PREFIX#/}"
PREFIX="${PREFIX%/}"

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
VERIFY="$SCRIPT_DIR/verify-install.sh"

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
trap 'rm -rf "$WORK"; exit 1' INT TERM

BIN_REL="$PREFIX/bin/codecap"
DBUS_REL="$PREFIX/share/dbus-1/services/dev.codecap.Helper.service"
UNIT_REL="$PREFIX/lib/systemd/user/codecap.service"
PLASMOID_REL="$PREFIX/share/plasma/plasmoids/dev.codecap.plasmoid"
META_REL="$PLASMOID_REL/metadata.json"

fail() {
	echo "$*" >&2
	exit 1
}

case_count=0

# A fresh copy of the known-good tree. Written to stdout so a case reads as one
# line: `dir=$(scratch)`. That runs it in a subshell, so it cannot keep a
# counter — mktemp supplies the unique name and `reject`, which does run in the
# parent, does the counting.
scratch() {
	dir=$(mktemp -d "$WORK/case.XXXXXX")
	rmdir "$dir"
	cp -a "$GOOD" "$dir"
	printf '%s\n' "$dir"
}

# Remove every line matching a basic regex, in place.
drop_line() {
	grep -v "$2" "$1" >"$1.tmp"
	mv "$1.tmp" "$1"
}

reject() {
	if "$VERIFY" "$2" "$PREFIX" >/dev/null 2>&1; then
		fail "FAIL: verify-install.sh accepted a tree it must reject: $1"
	fi
	case_count=$((case_count + 1))
	echo "  rejected: $1"
}

# Positive control. If this fails, every rejection below proves nothing.
"$VERIFY" "$GOOD" "$PREFIX" >/dev/null ||
	fail "FAIL: verify-install.sh rejected the known-good tree at $GOOD"
echo "  accepted: the known-good tree"

dir=$(scratch)
rm -f "$dir$BIN_REL"
reject "missing helper binary" "$dir"

# H-6: without SystemdService= the bus activates the binary directly and
# systemd never learns the unit exists, so Restart= is dead prose.
dir=$(scratch)
drop_line "$dir$DBUS_REL" '^SystemdService='
reject "D-Bus file without SystemdService=" "$dir"

dir=$(scratch)
drop_line "$dir$UNIT_REL" '^WantedBy='
reject "unit without [Install] WantedBy=" "$dir"

dir=$(scratch)
drop_line "$dir$UNIT_REL" '^Restart='
reject "unit without Restart= (ADR 0015)" "$dir"

dir=$(scratch)
drop_line "$dir$UNIT_REL" '^Type='
reject "unit without Type=dbus" "$dir"

# H-11: the install templated nothing, so the marker survives.
dir=$(scratch)
printf '[D-BUS Service]\nName=dev.codecap.Helper\nExec=@BINDIR@/codecap\nSystemdService=codecap.service\n' \
	>"$dir$DBUS_REL"
reject "untemplated @BINDIR@ in the D-Bus file" "$dir"

# H-11 again, and the shape the bug actually took: a real path, just the wrong
# prefix. Nothing about the file looks unfinished.
dir=$(scratch)
drop_line "$dir$DBUS_REL" '^Exec='
printf 'Exec=/opt/elsewhere/bin/codecap\n' >>"$dir$DBUS_REL"
reject "D-Bus Exec= under a different prefix" "$dir"

dir=$(scratch)
drop_line "$dir$UNIT_REL" '^BusName='
printf 'BusName=dev.codecap.NotTheHelper\n' >>"$dir$UNIT_REL"
reject "BusName= that does not match the D-Bus Name=" "$dir"

# The helper opens other people's credentials. set-uid is the one mode the
# binary check exists to refuse, and a `*755` glob would have let it through.
dir=$(scratch)
chmod 4755 "$dir$BIN_REL"
reject "set-uid helper binary" "$dir"

dir=$(scratch)
drop_line "$dir$META_REL" 'X-Plasma-NotificationArea"'
reject "metadata without X-Plasma-NotificationArea (P-M13)" "$dir"

# H-10: `cp -a plasmoid/.` used to ship the QML test suite into /usr/share.
dir=$(scratch)
mkdir -p "$dir$PLASMOID_REL/test"
reject "plasmoid package shipping test/" "$dir"

echo "verify-install.sh gate ok: 1 accepted, $case_count rejected (PREFIX=$PREFIX)"

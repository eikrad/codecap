#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later
#
# Assert the install tree matches the ADR 0010 / 0015 contract.
# Usage: verify-install.sh DESTDIR [PREFIX]
# PREFIX defaults to /usr so `make test-install` stays a one-liner.

set -eu

ROOT="${1:?usage: verify-install.sh DESTDIR [PREFIX]}"
PREFIX="${2:-/usr}"

# Strip a trailing slash so joins stay clean; DESTDIR may be empty for a live
# prefix install, so do not require it to be absolute.
ROOT="${ROOT%/}"
PREFIX="/${PREFIX#/}"
PREFIX="${PREFIX%/}"

fail() {
	echo "$*" >&2
	exit 1
}

require() {
	if [ ! -e "$1" ]; then
		fail "missing: $1"
	fi
}

# Paths under DESTDIR+PREFIX. PREFIX is always absolute after normalisation.
BIN="$ROOT$PREFIX/bin/codecap"
DBUS="$ROOT$PREFIX/share/dbus-1/services/dev.codecap.Helper.service"
SYSTEMD="$ROOT$PREFIX/lib/systemd/user/codecap.service"
PLASMOID="$ROOT$PREFIX/share/plasma/plasmoids/dev.codecap.plasmoid"
META="$PLASMOID/metadata.json"

require "$BIN"
require "$DBUS"
require "$SYSTEMD"
require "$META"

# --- binary ---------------------------------------------------------------

mode=$(stat -c '%a' "$BIN" 2>/dev/null || stat -f '%OLp' "$BIN")
case "$mode" in
*755 | 755 | 0755) ;;
*) fail "codecap mode is $mode, expected 755" ;;
esac

if [ ! -x "$BIN" ]; then
	fail "codecap is not executable: $BIN"
fi

# --- D-Bus ↔ systemd contract (ADR 0010, 0015 / H-6) -----------------------

dbus_name=$(sed -n 's/^Name=//p' "$DBUS" | head -n 1)
dbus_exec=$(sed -n 's/^Exec=//p' "$DBUS" | head -n 1)
dbus_systemd=$(sed -n 's/^SystemdService=//p' "$DBUS" | head -n 1)
unit_bus=$(sed -n 's/^BusName=//p' "$SYSTEMD" | head -n 1)
unit_exec=$(sed -n 's/^ExecStart=//p' "$SYSTEMD" | head -n 1)

[ -n "$dbus_name" ] || fail "D-Bus service missing Name="
[ -n "$dbus_exec" ] || fail "D-Bus service missing Exec="
[ -n "$dbus_systemd" ] || fail "D-Bus service missing SystemdService= (H-6: without it systemd never restarts the helper)"
[ -n "$unit_bus" ] || fail "systemd unit missing BusName="
[ -n "$unit_exec" ] || fail "systemd unit missing ExecStart="

[ "$dbus_name" = "$unit_bus" ] || fail "BusName ($unit_bus) does not match D-Bus Name ($dbus_name)"
[ "$dbus_systemd" = "codecap.service" ] || fail "SystemdService= is '$dbus_systemd', expected codecap.service"

expected_bin="$PREFIX/bin/codecap"
[ "$dbus_exec" = "$expected_bin" ] || fail "D-Bus Exec=$dbus_exec, expected $expected_bin (H-11)"
[ "$unit_exec" = "$expected_bin" ] || fail "ExecStart=$unit_exec, expected $expected_bin (H-11)"

if ! grep -q '^WantedBy=default.target$' "$SYSTEMD"; then
	fail "systemd unit missing [Install] WantedBy=default.target (H-6)"
fi

# No leftover template markers.
if grep -q '@BINDIR@' "$DBUS" "$SYSTEMD"; then
	fail "installed service files still contain @BINDIR@ — templating did not run"
fi

# --- plasmoid -------------------------------------------------------------

if ! grep -q '"Id": "dev.codecap.plasmoid"' "$META"; then
	fail "plasmoid metadata Id mismatch"
fi

if ! grep -q '"X-Plasma-NotificationArea": "true"' "$META"; then
	fail "metadata.json missing X-Plasma-NotificationArea (tray placement, P-M13)"
fi

if ! grep -q 'X-Plasma-NotificationAreaCategory' "$META"; then
	fail "metadata.json missing X-Plasma-NotificationAreaCategory (P-M13)"
fi

# The package must contain only what a KPackage needs. `cp -a plasmoid/.` used
# to ship the test suite into /usr/share.
for unexpected in "$PLASMOID/test" "$PLASMOID/tests"; do
	if [ -e "$unexpected" ]; then
		fail "unexpected entry in the installed plasmoid: $unexpected"
	fi
done

# Prefer kpackagetool6 when present — it is the real parser Plasma uses.
if command -v kpackagetool6 >/dev/null 2>&1; then
	if ! kpackagetool6 --type Plasma/Applet --show "$PLASMOID" >/dev/null; then
		fail "kpackagetool6 rejected $PLASMOID"
	fi
fi

# --- version sync (H-8) ---------------------------------------------------
# PKGBUILD lives at the repo root relative to this script; when verifying a
# DESTDIR staging tree from `make test-install`, the checkout is still here.
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd)
if [ -f "$REPO_ROOT/PKGBUILD" ]; then
	pkgver=$(sed -n 's/^pkgver=//p' "$REPO_ROOT/PKGBUILD" | head -n 1)
	metaver=$(sed -n 's/.*"Version": "\([^"]*\)".*/\1/p' "$META" | head -n 1)
	[ -n "$pkgver" ] || fail "could not read pkgver from PKGBUILD"
	[ -n "$metaver" ] || fail "could not read Version from metadata.json"
	[ "$pkgver" = "$metaver" ] || fail "PKGBUILD pkgver=$pkgver does not match metadata Version=$metaver"
fi

echo "install tree ok: $ROOT (PREFIX=$PREFIX)"

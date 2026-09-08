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

# Exactly 755. A leading-wildcard match here would also accept 4755 and 6755 —
# a set-uid helper reading credentials out of other users' Account Homes is the
# one mode this check exists to refuse.
mode=$(stat -c '%a' "$BIN" 2>/dev/null || stat -f '%OLp' "$BIN")
case "$mode" in
755 | 0755) ;;
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
unit_type=$(sed -n 's/^Type=//p' "$SYSTEMD" | head -n 1)
unit_restart=$(sed -n 's/^Restart=//p' "$SYSTEMD" | head -n 1)

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

# Type=dbus is what makes BusName= mean anything: it is how systemd knows the
# helper has finished starting, and it is the half of the H-6 handshake the
# D-Bus side cannot assert on its own.
[ "$unit_type" = "dbus" ] || fail "systemd unit Type=$unit_type, expected dbus (H-6)"

# ADR 0015 says systemd restarts the helper when it crashes. That claim was
# prose for three months while the unit had no [Install] section at all, so it
# gets an assertion rather than a promise.
case "$unit_restart" in
on-failure | always | on-abnormal) ;;
*) fail "systemd unit Restart=$unit_restart, expected on-failure (ADR 0015 / H-6)" ;;
esac

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

# PKGBUILD pkgver ↔ metadata Version (H-8) is `make check-version`, not this
# script. It is a property of the checkout, not of the installed tree: reading
# the repo's PKGBUILD while verifying a tree installed from an older tag would
# fail on a version skew that is entirely legitimate.

echo "install tree ok: $ROOT (PREFIX=$PREFIX)"

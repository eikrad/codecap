#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

set -eu

ROOT="${1:?usage: verify-install.sh DESTDIR}"

require() {
	if [ ! -e "$1" ]; then
		echo "missing: $1" >&2
		exit 1
	fi
}

require "$ROOT/usr/bin/codecap"
require "$ROOT/usr/share/dbus-1/services/dev.codecap.Helper.service"
require "$ROOT/usr/lib/systemd/user/codecap.service"
require "$ROOT/usr/share/plasma/plasmoids/dev.codecap.plasmoid/metadata.json"

if ! grep -q '"Id": "dev.codecap.plasmoid"' "$ROOT/usr/share/plasma/plasmoids/dev.codecap.plasmoid/metadata.json"; then
	echo "plasmoid metadata Id mismatch" >&2
	exit 1
fi

if ! grep -q 'Exec=/usr/bin/codecap' "$ROOT/usr/share/dbus-1/services/dev.codecap.Helper.service"; then
	echo "dbus service Exec path mismatch" >&2
	exit 1
fi

# The package must contain only what a KPackage needs. `cp -a plasmoid/.` used
# to ship the test suite into /usr/share.
PLASMOID_ROOT="$ROOT/usr/share/plasma/plasmoids/dev.codecap.plasmoid"
for unexpected in "$PLASMOID_ROOT/test" "$PLASMOID_ROOT/tests"; do
	if [ -e "$unexpected" ]; then
		echo "unexpected entry in the installed plasmoid: $unexpected" >&2
		exit 1
	fi
done

echo "install tree ok: $ROOT"

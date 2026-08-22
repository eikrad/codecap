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

echo "install tree ok: $ROOT"

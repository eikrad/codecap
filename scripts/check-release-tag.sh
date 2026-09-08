#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

# Asserts that a release tag names the version this tree actually declares.
#
# `make check-version` already keeps PKGBUILD pkgver and metadata.json Version
# in step with each other. Neither of them knows what the tag says, so a
# `git tag v0.3.0` on a tree that still declares 0.2.0 produces a release whose
# archive URL is v0.3.0 and whose PKGBUILD fetches v0.2.0 — a package that
# builds the wrong source with a checksum that matches, which is worse than one
# that fails.
#
# Usage: check-release-tag.sh v0.3.0

set -eu

tag=${1:-}
if [ -z "$tag" ]; then
	echo "usage: $0 <tag>" >&2
	exit 2
fi

REPO_ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$REPO_ROOT"

# The one source of truth for the two version fields agreeing.
make check-version

pkgver=$(sed -n 's/^pkgver=//p' PKGBUILD | head -n 1)
if [ -z "$pkgver" ]; then
	echo "could not read pkgver from PKGBUILD" >&2
	exit 1
fi

expected="v$pkgver"
if [ "$tag" != "$expected" ]; then
	echo "tag '$tag' does not name this tree's version." >&2
	echo "PKGBUILD declares pkgver=$pkgver, so the tag must be '$expected'." >&2
	echo "Bump pkgver and plasmoid/metadata.json first, or retag." >&2
	exit 1
fi

echo "release tag ok: $tag matches pkgver=$pkgver"

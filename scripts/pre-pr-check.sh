#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

# Runs what CI runs, before a PR is opened.
#
# `make ci` is the real gate but cannot be used here directly: it stops at the
# first missing tool. golangci-lint and shellcheck are installed in CI and not
# necessarily on a developer machine, and `make ci` exiting 127 for that reason
# is indistinguishable from a genuine failure. Each gate is therefore run on its
# own and reported as pass, FAIL or skipped.
#
# A skipped gate is printed, never silently passed: PR #21 was opened green
# locally and failed CI on the first target neither developer machine could run.
#
# Exit 0 when nothing failed, 1 when something did. Gates that could not run are
# listed on stdout either way -- they are the part of CI this cannot vouch for.

set -eu

REPO_ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$REPO_ROOT"

failed=''
skipped=''

# run <name> <tool-that-must-exist-or-empty> <command...>
run() {
	name=$1
	needs=$2
	shift 2

	if [ -n "$needs" ] && ! command -v "$needs" >/dev/null 2>&1; then
		skipped="$skipped $name($needs)"
		echo "SKIP  $name -- $needs is not installed"
		return 0
	fi

	if out=$("$@" 2>&1); then
		echo "ok    $name"
	else
		echo "FAIL  $name"
		printf '%s\n' "$out" | sed 's/^/      /'
		failed="$failed $name"
	fi
}

echo "Running the CI gates locally (see .github/workflows/ci.yml)"

run fmt-check    ''             make fmt-check
run go-vet       ''             go vet ./...
# golangci-lint is the only gate CI pins to an exact version, and a local build
# that differs reports checks CI does not have or misses ones it does. That is
# how a locally green tree fails CI -- and how a locally red tree turns out to
# be fine, which is what a v2.13.2 here against a v2.5.0 pin produced. The pin
# is read from the workflow rather than restated, so the two cannot drift apart.
if command -v golangci-lint >/dev/null 2>&1; then
	pinned=$(sed -n 's,.*golangci-lint/v2/cmd/golangci-lint@v\([0-9.]*\).*,\1,p' \
		.github/workflows/ci.yml | head -n 1)
	installed=$(golangci-lint --version 2>/dev/null \
		| sed -n 's/.*version \([0-9][0-9.]*\) .*/\1/p' | head -n 1)
	if [ -n "$pinned" ] && [ -n "$installed" ] && [ "$pinned" != "$installed" ]; then
		echo "WARN  golangci is $installed here, CI pins $pinned -- results may differ"
		skipped="$skipped golangci-version($installed!=$pinned)"
	fi
fi
run golangci     golangci-lint  golangci-lint run ./...
run lint-sh      shellcheck     make lint-sh
run lint-qml     ''             make lint-qml
# CI runs `pipx run reuse lint`. reuse is the one gate here written in Python,
# so an ephemeral run of it is the same thing pipx does in CI -- unpinned latest
# on both sides. uv is checked second because a system reuse is what a developer
# who installed one expects to be tested against.
if command -v reuse >/dev/null 2>&1; then
	run reuse '' reuse lint
elif command -v uv >/dev/null 2>&1; then
	run reuse '' uv tool run reuse lint
else
	run reuse reuse reuse lint
fi
run test         ''             make test
run test-install ''             make test-install

echo
if [ -n "$skipped" ]; then
	echo "Not verified here:$skipped"
fi

if [ -n "$failed" ]; then
	echo "FAILED:$failed"
	exit 1
fi

echo "All gates that could run passed."
exit 0

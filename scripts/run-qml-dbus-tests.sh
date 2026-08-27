#!/bin/sh
# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

# Phase 5.3 of docs/plan-hardening.md. Runs the applet's own SnapshotSource
# against a stub dev.codecap.Helper on a private session bus.
#
# It skips itself, loudly, on a machine that cannot run it. The suite needs
# org.kde.plasma.workspace.dbus, which is a Plasma 6 module: CI does not install
# KDE, and no Plasma 5 distribution ships it at all. Skipping prints the reason
# — a suite that quietly never runs is the failure mode this whole branch has
# been chasing.

set -eu

REPO_ROOT=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
INPUT="$REPO_ROOT/tests/plasmoid/qml-dbus"
MISSING_MODULE='module "org.kde.plasma.workspace.dbus" is not installed'

skip() {
	echo "skipping tests/plasmoid/qml-dbus: $1"
	exit 0
}

# The applet is Qt 6. On a distro that ships both, `qmltestrunner` on PATH is
# the Qt 5 one, and it fails on the version-less `import QtQuick` with exit 1
# and no output — which reads as a broken test rather than as the wrong binary.
find_runner() {
	if [ -n "${QMLTESTRUNNER:-}" ] && [ -x "${QMLTESTRUNNER}" ]; then
		echo "$QMLTESTRUNNER"
		return 0
	fi
	if command -v qmltestrunner6 >/dev/null 2>&1; then
		command -v qmltestrunner6
		return 0
	fi
	if [ -x /usr/lib/qt6/bin/qmltestrunner ]; then
		echo /usr/lib/qt6/bin/qmltestrunner
		return 0
	fi
	if command -v qmltestrunner >/dev/null 2>&1; then
		command -v qmltestrunner
		return 0
	fi
	return 1
}

# The second half of this script, run inside the private bus that the first half
# starts. It is one file rather than two so the two halves cannot drift apart.
run_in_session() {
	runner=$1
	stub=$2

	stub_log=$(mktemp)
	"$stub" >"$stub_log" 2>&1 &
	stub_pid=$!
	# Expanded now on purpose: the values are known here, and the trap must not
	# depend on variables a later failure might have changed.
	# shellcheck disable=SC2064
	trap "kill $stub_pid 2>/dev/null || true; rm -f '$stub_log'" EXIT INT TERM

	# The stub prints one line once it owns dev.codecap.Helper. Waiting for the
	# name rather than for a fixed delay is what keeps this from being flaky on
	# a loaded machine.
	waited=0
	while [ "$waited" -lt 100 ]; do
		if grep -q '^ready$' "$stub_log" 2>/dev/null; then
			break
		fi
		if ! kill -0 "$stub_pid" 2>/dev/null; then
			echo "the helper stub exited before it owned the bus name:" >&2
			cat "$stub_log" >&2
			return 1
		fi
		sleep 0.1
		waited=$((waited + 1))
	done
	if ! grep -q '^ready$' "$stub_log" 2>/dev/null; then
		echo "the helper stub did not take dev.codecap.Helper within 10 s:" >&2
		cat "$stub_log" >&2
		return 1
	fi

	# Captured rather than streamed, so a missing Plasma module can be told
	# apart from a real failure without running the suite twice.
	set +e
	output=$(QT_QPA_PLATFORM=offscreen "$runner" -input "$INPUT" 2>&1)
	status=$?
	set -e

	case "$output" in
	*"$MISSING_MODULE"*)
		skip "$MISSING_MODULE (a Plasma 6 desktop is needed)"
		;;
	esac

	printf '%s\n' "$output"
	if [ -s "$stub_log" ] && [ "$status" -ne 0 ]; then
		echo "--- helper stub output ---" >&2
		cat "$stub_log" >&2
	fi
	return "$status"
}

if [ "${1:-}" = "--in-session" ]; then
	run_in_session "$2" "$3"
	exit $?
fi

RUNNER=$(find_runner) || skip "no Qt 6 qmltestrunner on this machine"
command -v dbus-run-session >/dev/null 2>&1 || skip "dbus-run-session is not installed"
command -v go >/dev/null 2>&1 || skip "no Go toolchain to build the helper stub with"

STUB_DIR=$(mktemp -d)
trap 'rm -rf "$STUB_DIR"' EXIT INT TERM
( cd "$REPO_ROOT" && go build -o "$STUB_DIR/helperstub" ./tests/helperstub )

# A private bus, so the stub can own dev.codecap.Helper without displacing a
# real helper on the developer's own session — and so nothing on that session
# can answer these calls instead of the stub. Not exec'd: the trap above still
# has a temporary directory to clean up.
set +e
dbus-run-session -- "$0" --in-session "$RUNNER" "$STUB_DIR/helperstub"
status=$?
set -e
exit "$status"

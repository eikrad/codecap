# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

PREFIX ?= /usr
DESTDIR ?=
GO ?= go

BINARY := codecap
BINDIR := $(PREFIX)/bin
DBUSDIR := $(PREFIX)/share/dbus-1/services
SYSTEMDUSERDIR := $(PREFIX)/lib/systemd/user
PLASMOIDDIR := $(PREFIX)/share/plasma/plasmoids/dev.codecap.plasmoid

BUILD_FLAGS := -trimpath -ldflags="-s -w"

# Lint tooling. `make lint` needs golangci-lint, shellcheck and qmllint; CI
# installs all three. qmllint is not on PATH on Debian/Ubuntu, and where a
# distro ships both Qt versions the one on PATH is Qt 5 — which rejects
# QMLLINT_FLAGS below as unknown options. Qt 6 is looked up first.
QMLLINT ?= $(shell command -v qmllint6 2>/dev/null \
	|| { test -x /usr/lib/qt6/bin/qmllint && echo /usr/lib/qt6/bin/qmllint; } \
	|| command -v qmllint 2>/dev/null \
	|| echo /usr/lib/qt6/bin/qmllint)

# Plasma and Kirigami types cannot be resolved without a Plasma 6 install, so
# import/type checking is off and qmllint runs as a syntax and structure gate.
# Re-enable a category here once the CI image can resolve the imports.
#
# Category names are not stable across Qt 6 point releases: `property` became
# `property-override`, `alias` became `alias-cycle`, `signal` became
# `signal-handler-parameters`, and `type` is gone. qmllint rejects an unknown
# category outright, so naming them statically pins the Makefile to one Qt
# version. Each line below lists the names for one category, newest first, and
# only the first one this qmllint advertises is passed. A category that no
# longer exists contributes nothing.
QMLLINT_HELP := $(shell $(QMLLINT) --help 2>/dev/null)
# `--name=disable` rather than `--name disable` so one candidate stays one word
# and $(firstword) can pick it.
qmllint-off = $(firstword $(foreach n,$(1),$(if $(findstring --$(n) ,$(QMLLINT_HELP)),--$(n)=disable)))
QMLLINT_FLAGS := $(call qmllint-off,import) \
	$(call qmllint-off,property-override property) \
	$(call qmllint-off,unqualified) \
	$(call qmllint-off,alias-cycle alias) \
	$(call qmllint-off,signal-handler-parameters signal) \
	$(call qmllint-off,deprecated) \
	$(call qmllint-off,unused-imports) \
	$(call qmllint-off,type)

# The test suites are linted too. tests/plasmoid/qml-plasma and
# tests/plasmoid/qml-dbus skip themselves wherever their Plasma imports are
# missing, which is everywhere except a Plasma 6 desktop — so a syntax error in
# one of them would otherwise sit undetected until someone with that desktop
# happened to run it.
QML_SOURCES := plasmoid/contents/ui/*.qml plasmoid/contents/config/*.qml \
	tests/plasmoid/qml/*.qml tests/plasmoid/qml-plasma/*.qml \
	tests/plasmoid/qml-dbus/*.qml

# qmltestrunner executes logic.js inside a real QML engine. The Node tests
# cannot see that QML types are not JavaScript types, or that QML's
# Number.toLocaleString has different defaults from ECMAScript's — both of
# which shipped as bugs.
# The applet is Qt 6, so the Qt 6 runner is looked up first. On a distro that
# ships both, `qmltestrunner` on PATH is the Qt 5 one, and it fails on the
# version-less `import QtQuick` with exit 1 and no output at all — which reads
# as a broken test rather than as the wrong binary.
QMLTESTRUNNER ?= $(shell command -v qmltestrunner6 2>/dev/null \
	|| { test -x /usr/lib/qt6/bin/qmltestrunner && echo /usr/lib/qt6/bin/qmltestrunner; } \
	|| command -v qmltestrunner 2>/dev/null \
	|| echo /usr/lib/qt6/bin/qmltestrunner)

.PHONY: build install install-all test test-plasmoid test-plasmoid-qml \
	test-plasmoid-qml-plasma test-plasmoid-qml-dbus test-install \
	lint fmt-check lint-go lint-sh lint-qml ci clean

build:
	$(GO) build $(BUILD_FLAGS) -o bin/$(BINARY) ./cmd/codecap

# install deliberately does NOT depend on build. It runs under sudo, and a
# rebuild as root inside the user's checkout fails outright — git refuses to
# operate on a repository it does not own, and go build reports that as
# "error obtaining VCS status: exit status 128". Even when it succeeds it
# leaves root-owned files in bin/. Build as yourself, then install.
install:
	@test -f bin/$(BINARY) || { \
		echo "bin/$(BINARY) not found. Run 'make build' as your own user first,"; \
		echo "or use 'make install-all' to build and install in one step."; \
		exit 1; \
	}
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 bin/$(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -d $(DESTDIR)$(DBUSDIR)
	install -m 644 contrib/dbus/dev.codecap.Helper.service $(DESTDIR)$(DBUSDIR)/dev.codecap.Helper.service
	install -d $(DESTDIR)$(SYSTEMDUSERDIR)
	install -m 644 contrib/systemd/codecap.service $(DESTDIR)$(SYSTEMDUSERDIR)/codecap.service
	install -d $(DESTDIR)$(PLASMOIDDIR)
	# Replace the package directory rather than merging into it. Naming the
	# contents below keeps this install from *adding* anything unwanted, but it
	# cannot remove what an earlier version put there: plasmoid/test/ shipped
	# until the tests moved under tests/, and a copy from 2026-08-22 was still
	# sitting in /usr after every upgrade since. Anything this version does not
	# ship has no business surviving in the package directory.
	#
	# The case guard is why this is safe to run under sudo: PLASMOIDDIR is built
	# from PREFIX, and a mis-set PREFIX would otherwise aim the rm somewhere
	# else entirely. Only the contents go — the directory itself stays, so a
	# DESTDIR staging root keeps whatever permissions it was created with.
	@case "$(DESTDIR)$(PLASMOIDDIR)" in \
		*/dev.codecap.plasmoid) \
			rm -rf -- "$(DESTDIR)$(PLASMOIDDIR)"/* ;; \
		*) \
			echo "refusing to clear '$(DESTDIR)$(PLASMOIDDIR)': not a dev.codecap.plasmoid package directory"; \
			exit 1 ;; \
	esac
	# Named explicitly. The tests live in tests/ rather than under plasmoid/
	# so that kpackagetool6, which installs the whole directory, cannot ship
	# them either; naming the contents here keeps that true by construction.
	install -m 644 plasmoid/metadata.json $(DESTDIR)$(PLASMOIDDIR)/metadata.json
	cp -a plasmoid/contents $(DESTDIR)$(PLASMOIDDIR)/

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed:"; echo "$$unformatted"; \
		gofmt -d $$unformatted; \
		exit 1; \
	fi

lint-go: fmt-check
	$(GO) vet ./...
	golangci-lint run ./...

lint-sh:
	shellcheck -s sh scripts/*.sh

lint-qml:
	$(QMLLINT) $(QMLLINT_FLAGS) $(QML_SOURCES)

lint: lint-go lint-sh lint-qml

# The bus tests need a session bus. dbus-run-session gives them a private one;
# without it they skip locally and fail in CI, rather than quietly passing.
test:
	@if command -v dbus-run-session >/dev/null 2>&1; then \
		dbus-run-session -- $(GO) test -race ./...; \
	else \
		echo "dbus-run-session not found: the bus tests will skip"; \
		$(GO) test -race ./...; \
	fi
	$(MAKE) test-plasmoid

test-plasmoid:
	node --test tests/plasmoid/*.test.mjs
	$(MAKE) test-plasmoid-qml

test-plasmoid-qml:
	QT_QPA_PLATFORM=offscreen $(QMLTESTRUNNER) -input tests/plasmoid/qml
	$(MAKE) test-plasmoid-qml-plasma

# These load the applet's own components, so they need Kirigami. CI installs the
# Qt QML modules only, and pulling KDE into it to run two colour assertions is a
# bad trade — so this skips itself rather than failing there, and runs for real
# on any machine that can actually display the widget.
test-plasmoid-qml-plasma:
	@if QT_QPA_PLATFORM=offscreen $(QMLTESTRUNNER) -input tests/plasmoid/qml-plasma 2>&1 | grep -q "module \"org.kde.kirigami\" is not installed"; then \
		echo "skipping tests/plasmoid/qml-plasma: Kirigami not installed"; \
	else \
		QT_QPA_PLATFORM=offscreen $(QMLTESTRUNNER) -input tests/plasmoid/qml-plasma; \
	fi
	$(MAKE) test-plasmoid-qml-dbus

# Phase 5.3: the applet's own SnapshotSource against a stub helper on a private
# session bus. This is the only thing in the repo that executes the D-Bus half
# of the plasmoid — half of the criticals of 2026-08-22 were handler names that
# nothing called, and no other gate here can see that.
#
# The script starts the bus and the stub, and skips itself with a reason when
# the Plasma 6 QML D-Bus module is missing (CI, and every Plasma 5 distro).
test-plasmoid-qml-dbus:
	QMLTESTRUNNER="$(QMLTESTRUNNER)" scripts/run-qml-dbus-tests.sh

install-all: build install

ci: lint test test-install

test-install: build
	rm -rf .install-test
	@if $(MAKE) -n install DESTDIR=$(CURDIR)/.install-test | grep -q '$(GO) build'; then \
		echo "FAIL: make install would run the Go toolchain."; \
		echo "It runs under sudo, and a rebuild as root fails on VCS ownership and"; \
		echo "leaves root-owned files in bin/. See the comment on the install target."; \
		exit 1; \
	fi
	$(MAKE) install DESTDIR=$(CURDIR)/.install-test
	scripts/verify-install.sh $(CURDIR)/.install-test
	rm -rf .install-test

clean:
	rm -rf bin/ .install-test

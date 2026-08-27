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

VERSION := $(shell sed -n 's/.*"Version": "\([^"]*\)".*/\1/p' plasmoid/metadata.json | head -n 1)
DIST := $(BINARY)-$(VERSION).tar.gz

.PHONY: build install uninstall install-all dist test test-plasmoid \
	test-plasmoid-qml test-plasmoid-qml-plasma test-plasmoid-qml-dbus \
	test-install check-version lint fmt-check lint-go lint-sh lint-qml ci clean

build:
	$(GO) build $(BUILD_FLAGS) -o bin/$(BINARY) ./cmd/codecap

# Substitute @BINDIR@ into the service templates. PREFIX is a real install
# prefix now (H-11): without this, a PREFIX=/usr/local install still pointed
# D-Bus at /usr/bin/codecap and the helper could never activate.
define render-service
	sed 's|@BINDIR@|$(BINDIR)|g' $(1) > $(2)
endef

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
	$(call render-service,contrib/dbus/dev.codecap.Helper.service.in,$(DESTDIR)$(DBUSDIR)/dev.codecap.Helper.service)
	chmod 644 $(DESTDIR)$(DBUSDIR)/dev.codecap.Helper.service
	install -d $(DESTDIR)$(SYSTEMDUSERDIR)
	$(call render-service,contrib/systemd/codecap.service.in,$(DESTDIR)$(SYSTEMDUSERDIR)/codecap.service)
	chmod 644 $(DESTDIR)$(SYSTEMDUSERDIR)/codecap.service
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

# Same path guards as install: never rm outside the codecap package layout.
uninstall:
	@case "$(DESTDIR)$(PLASMOIDDIR)" in \
		*/dev.codecap.plasmoid) ;; \
		*) \
			echo "refusing to uninstall '$(DESTDIR)$(PLASMOIDDIR)': not a dev.codecap.plasmoid package directory"; \
			exit 1 ;; \
	esac
	rm -f -- "$(DESTDIR)$(BINDIR)/$(BINARY)"
	rm -f -- "$(DESTDIR)$(DBUSDIR)/dev.codecap.Helper.service"
	rm -f -- "$(DESTDIR)$(SYSTEMDUSERDIR)/codecap.service"
	rm -rf -- "$(DESTDIR)$(PLASMOIDDIR)"

# Reproducible source tarball for the PKGBUILD. Prefer git archive so the
# checksum is stable across clean checkouts; fall back to tar of the tree when
# this is not a git checkout (e.g. an already-extracted release).
dist:
	@test -n "$(VERSION)" || { echo "could not read Version from plasmoid/metadata.json"; exit 1; }
	@if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then \
		git archive --format=tar.gz --prefix=$(BINARY)-$(VERSION)/ \
			-o $(DIST) HEAD; \
	else \
		tar --exclude=$(DIST) --exclude=./.git --exclude=./bin \
			--exclude=./.install-test --exclude=./pkg --exclude=./src \
			--exclude=./.gocache \
			--transform='s,^\./,$(BINARY)-$(VERSION)/,' \
			-czf $(DIST) .; \
	fi
	@echo "wrote $(DIST)"

check-version:
	@test -n "$(VERSION)" || { echo "could not read Version from plasmoid/metadata.json"; exit 1; }
	@pkgver=$$(sed -n 's/^pkgver=//p' PKGBUILD | head -n 1); \
	if [ "$$pkgver" != "$(VERSION)" ]; then \
		echo "PKGBUILD pkgver=$$pkgver does not match metadata Version=$(VERSION)"; \
		exit 1; \
	fi
	@echo "version ok: $(VERSION)"

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

ci: check-version lint test test-install

test-install: build
	rm -rf .install-test .install-test-local
	@if $(MAKE) -n install DESTDIR=$(CURDIR)/.install-test | grep -q '$(GO) build'; then \
		echo "FAIL: make install would run the Go toolchain."; \
		echo "It runs under sudo, and a rebuild as root fails on VCS ownership and"; \
		echo "leaves root-owned files in bin/. See the comment on the install target."; \
		exit 1; \
	fi
	$(MAKE) install DESTDIR=$(CURDIR)/.install-test
	scripts/verify-install.sh $(CURDIR)/.install-test
	# H-11: PREFIX must rewrite both service files. A /usr/local install that
	# still pointed at /usr/bin/codecap left the helper unreachable forever.
	$(MAKE) install DESTDIR=$(CURDIR)/.install-test-local PREFIX=/usr/local
	scripts/verify-install.sh $(CURDIR)/.install-test-local /usr/local
	$(MAKE) uninstall DESTDIR=$(CURDIR)/.install-test-local PREFIX=/usr/local
	@if [ -e $(CURDIR)/.install-test-local/usr/local/bin/$(BINARY) ]; then \
		echo "FAIL: uninstall left $(BINARY) behind"; exit 1; \
	fi
	@if [ -e $(CURDIR)/.install-test-local/usr/local/share/plasma/plasmoids/dev.codecap.plasmoid ]; then \
		echo "FAIL: uninstall left the plasmoid tree behind"; exit 1; \
	fi
	rm -rf .install-test .install-test-local

clean:
	rm -rf bin/ .install-test .install-test-local $(DIST)

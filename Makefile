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
# installs all three. qmllint is not on PATH on Debian/Ubuntu.
QMLLINT ?= $(shell command -v qmllint 2>/dev/null || echo /usr/lib/qt6/bin/qmllint)

# Plasma and Kirigami types cannot be resolved without a Plasma 6 install, so
# import/type checking is off and qmllint runs as a syntax and structure gate.
# Re-enable a category here once the CI image can resolve the imports.
QMLLINT_FLAGS := --import disable --type disable --property disable \
	--unqualified disable --alias disable --signal disable \
	--deprecated disable --unused-imports disable

QML_SOURCES := plasmoid/contents/ui/*.qml plasmoid/contents/config/*.qml

# qmltestrunner executes logic.js inside a real QML engine. The Node tests
# cannot see that QML types are not JavaScript types, or that QML's
# Number.toLocaleString has different defaults from ECMAScript's — both of
# which shipped as bugs.
QMLTESTRUNNER ?= $(shell command -v qmltestrunner 2>/dev/null || echo /usr/lib/qt6/bin/qmltestrunner)

.PHONY: build install install-all test test-plasmoid test-plasmoid-qml test-install \
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

test:
	$(GO) test -race ./...
	$(MAKE) test-plasmoid

test-plasmoid:
	node --test tests/plasmoid/*.test.mjs
	$(MAKE) test-plasmoid-qml

test-plasmoid-qml:
	QT_QPA_PLATFORM=offscreen $(QMLTESTRUNNER) -input tests/plasmoid/qml

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

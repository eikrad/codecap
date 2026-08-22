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

.PHONY: build install test test-plasmoid test-install \
	lint fmt-check lint-go lint-sh lint-qml ci clean

build:
	$(GO) build $(BUILD_FLAGS) -o bin/$(BINARY) ./cmd/codecap

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 755 bin/$(BINARY) $(DESTDIR)$(BINDIR)/$(BINARY)
	install -d $(DESTDIR)$(DBUSDIR)
	install -m 644 contrib/dbus/dev.codecap.Helper.service $(DESTDIR)$(DBUSDIR)/dev.codecap.Helper.service
	install -d $(DESTDIR)$(SYSTEMDUSERDIR)
	install -m 644 contrib/systemd/codecap.service $(DESTDIR)$(SYSTEMDUSERDIR)/codecap.service
	install -d $(DESTDIR)$(PLASMOIDDIR)
	cp -a plasmoid/. $(DESTDIR)$(PLASMOIDDIR)/

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
	node --test plasmoid/test/*.test.mjs

ci: lint test test-install

test-install:
	rm -rf .install-test
	$(MAKE) install DESTDIR=$(CURDIR)/.install-test
	scripts/verify-install.sh $(CURDIR)/.install-test
	rm -rf .install-test

clean:
	rm -rf bin/ .install-test

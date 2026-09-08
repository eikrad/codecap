# SPDX-FileCopyrightText: 2026 eikrad <eikef.rades@protonmail.com>
# SPDX-License-Identifier: GPL-2.0-or-later

pkgname=codecap
pkgver=0.2.0
pkgrel=1
pkgdesc="Plasma 6 coding-agent usage and Allowance widget"
arch=('x86_64' 'aarch64')
url="https://github.com/eikrad/codecap"
license=('GPL-2.0-or-later')
depends=(
  'libplasma'
  'plasma-workspace'
  'kirigami'
  'dbus'
)
makedepends=(
  'go'
)
options=('!strip')
# The GitHub release archive for the v$pkgver tag, pinned. makepkg fetches and
# verifies it, so this builds on a machine that has never seen this repository —
# which a local `make dist` tarball with sha256sums=('SKIP') could not do, and
# which the AUR requires.
#
# On every version bump: change pkgver, then run `updpkgsums` to refresh the
# checksum. `make check-version` (run from check() below) fails the build if
# pkgver and plasmoid/metadata.json disagree.
source=("$pkgname-$pkgver.tar.gz::https://github.com/eikrad/codecap/archive/v$pkgver.tar.gz")
sha256sums=('755f2d5385be280fb252b65492668a2247d2080a6cad4f814b84b02089db5882')

build() {
  cd "$srcdir/$pkgname-$pkgver"
  export CGO_ENABLED=0
  make build
}

check() {
  cd "$srcdir/$pkgname-$pkgver"
  make check-version
  if command -v dbus-run-session >/dev/null 2>&1; then
    dbus-run-session -- go test ./...
  else
    go test ./...
  fi
}

package() {
  cd "$srcdir/$pkgname-$pkgver"
  make install DESTDIR="$pkgdir" PREFIX=/usr
}

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
  'git'
)
options=('!strip')
# Local tarball from `make dist`. When publishing to the AUR, switch to the
# GitHub release archive and replace SKIP with the pinned checksum:
#   source=("$pkgname-$pkgver.tar.gz::https://github.com/eikrad/codecap/archive/v$pkgver.tar.gz")
#   sha256sums=('…')
source=("$pkgname-$pkgver.tar.gz")
sha256sums=('SKIP')

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

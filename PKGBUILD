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
  'plasma-framework'
)
makedepends=(
  'go'
)
options=('!strip')
source=()
sha256sums=()

build() {
  cd "$startdir"
  make build
}

package() {
  cd "$startdir"
  make install DESTDIR="$pkgdir"
}

# Maintainer: Aleks Clark <aleks@example.com>
pkgname=turing-smart-screen
pkgver=0.1.0
pkgrel=1
pkgdesc="System monitor displays for Turing Smart Screen USB-C LCD panels"
arch=('x86_64' 'aarch64')
url="https://github.com/aleksclark/go-turing-smart-screen"
license=('MIT')
depends=('glibc')
makedepends=('go')
optdepends=('ttf-jetbrains-mono: Default font for displays')
backup=('etc/udev/rules.d/99-turing-lcd.rules')
source=("$pkgname-$pkgver.tar.gz::$url/archive/v$pkgver.tar.gz")
sha256sums=('SKIP')

build() {
    cd "go-$pkgname-$pkgver"
    export CGO_CPPFLAGS="${CPPFLAGS}"
    export CGO_CFLAGS="${CFLAGS}"
    export CGO_CXXFLAGS="${CXXFLAGS}"
    export CGO_LDFLAGS="${LDFLAGS}"
    export GOFLAGS="-buildmode=pie -trimpath -mod=vendor -modcacherw"
    go build -ldflags="-s -w -linkmode=external" -o turing-screens ./cmd/screens
}

package() {
    cd "go-$pkgname-$pkgver"
    
    # Binary
    install -Dm755 turing-screens "$pkgdir/usr/bin/turing-screens"
    
    # Systemd service
    install -Dm644 install/turing-screens.service "$pkgdir/usr/lib/systemd/system/turing-screens.service"
    
    # udev rules (template - user must configure)
    install -Dm644 install/99-turing-lcd.rules "$pkgdir/etc/udev/rules.d/99-turing-lcd.rules"
    
    # Documentation
    install -Dm644 README.md "$pkgdir/usr/share/doc/$pkgname/README.md"
    install -Dm644 AGENT_STATUS_REPORTING.md "$pkgdir/usr/share/doc/$pkgname/AGENT_STATUS_REPORTING.md"
    install -Dm644 agent-status.schema.json "$pkgdir/usr/share/doc/$pkgname/agent-status.schema.json"
    install -Dm644 install/README.md "$pkgdir/usr/share/doc/$pkgname/INSTALL.md"
    
    # License
    install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
}

post_install() {
    echo ">>> Configure udev rules for your displays:"
    echo ">>>   sudo nano /etc/udev/rules.d/99-turing-lcd.rules"
    echo ">>> Then reload: sudo udevadm control --reload-rules && sudo udevadm trigger"
    echo ""
    echo ">>> Enable the service:"
    echo ">>>   sudo systemctl enable --now turing-screens"
}

post_upgrade() {
    post_install
}

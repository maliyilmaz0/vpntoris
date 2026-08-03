#!/bin/bash
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
VERSION=${VERSION:-1.1.0}
ARCH=${ARCH:-universal}
APP_NAME=VPNToris
BUILD_DIR="$ROOT_DIR/build/release"
APP="$BUILD_DIR/$APP_NAME.app"
DIST_DIR="$ROOT_DIR/dist"
DMG="$DIST_DIR/$APP_NAME-$VERSION-$ARCH.dmg"
SIGN_IDENTITY=${SIGN_IDENTITY:-Developer ID Application: RAKORT BILGI VE ILETISIM TEKNOLOJILERI LIMITED SIRKETI (8Y8RYA7N3L)}
NOTARY_PROFILE=${NOTARY_PROFILE:-FASTNAC_NOTARIZE}
UNSIGNED=false

if [[ ${1:-} == "--unsigned" ]]; then
    UNSIGNED=true
fi

for command in go xcrun sips iconutil hdiutil lipo; do
    command -v "$command" >/dev/null || { echo "Missing prerequisite: $command" >&2; exit 1; }
done

rm -rf "$BUILD_DIR"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/Licenses" "$DIST_DIR"
cp "$ROOT_DIR/vpntoris-tray/Info.plist" "$APP/Contents/Info.plist"
cp "$ROOT_DIR/assets/vpntoris-logo.png" "$APP/Contents/Resources/VPNTorisLogo.png"
cp -R "$ROOT_DIR/vpntoris-tray/Resources/." "$APP/Contents/Resources/"
mkdir -p "$APP/Contents/Resources/DockerContext"
cp -R "$ROOT_DIR/docker/." "$APP/Contents/Resources/DockerContext/"

export GOCACHE=${GOCACHE:-/tmp/vpntoris-release-go-cache}
export CLANG_MODULE_CACHE_PATH=${CLANG_MODULE_CACHE_PATH:-/tmp/vpntoris-release-clang-cache}
export SWIFT_MODULECACHE_PATH=${SWIFT_MODULECACHE_PATH:-/tmp/vpntoris-release-swift-cache}

ARCHES=("$ARCH")
if [[ "$ARCH" == "universal" ]]; then
    ARCHES=(arm64 x86_64)
fi

(cd "$ROOT_DIR/vpntoris-tray" && go test ./...)
for TARGET_ARCH in "${ARCHES[@]}"; do
    GO_ARCH="$TARGET_ARCH"
    if [[ "$TARGET_ARCH" == "x86_64" ]]; then GO_ARCH=amd64; fi
    (
        cd "$ROOT_DIR/vpntoris-tray"
        CGO_ENABLED=0 GOARCH="$GO_ARCH" go build -trimpath -ldflags "-s -w" -o "$BUILD_DIR/vpntorisd-$TARGET_ARCH" .
        CGO_ENABLED=0 GOARCH="$GO_ARCH" go build -trimpath -ldflags "-s -w" -o "$BUILD_DIR/vpntoris-route-helper-$TARGET_ARCH" ./routerhelper
        CGO_ENABLED=0 GOARCH="$GO_ARCH" go build -trimpath -ldflags "-s -w" -o "$BUILD_DIR/vpntorisctl-$TARGET_ARCH" ./cli
    )
    xcrun swiftc -O -whole-module-optimization -parse-as-library \
        -target "$TARGET_ARCH-apple-macos13.0" \
        "$ROOT_DIR/vpntoris-tray/swift/VPNTorisApp.swift" \
        -o "$BUILD_DIR/VPNToris-$TARGET_ARCH"
    (cd "$ROOT_DIR/vpntoris-tray" && GOARCH="$GO_ARCH" go build -o "$BUILD_DIR/tun2socks-bin-$TARGET_ARCH" github.com/xjasonlyu/tun2socks/v2)
done

for binary in vpntorisd vpntoris-route-helper vpntorisctl VPNToris tun2socks-bin; do
    OUTPUT_NAME="$binary"
    if [[ "$binary" == "tun2socks-bin" ]]; then OUTPUT_NAME=tun2socks; fi
    if [[ "$ARCH" == "universal" ]]; then
        lipo -create "$BUILD_DIR/$binary-arm64" "$BUILD_DIR/$binary-x86_64" -output "$APP/Contents/MacOS/$OUTPUT_NAME"
    else
        cp "$BUILD_DIR/$binary-$ARCH" "$APP/Contents/MacOS/$OUTPUT_NAME"
    fi
done
cp "$ROOT_DIR/third_party/tun2socks-LICENSE" "$APP/Contents/Resources/Licenses/tun2socks-LICENSE"

ICONSET="$BUILD_DIR/AppIcon.iconset"
mkdir -p "$ICONSET"
for size in 16 32 128 256 512; do
    sips -z "$size" "$size" "$ROOT_DIR/assets/vpntoris-logo.png" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
    double=$((size * 2))
    sips -z "$double" "$double" "$ROOT_DIR/assets/vpntoris-logo.png" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"

chmod 755 "$APP/Contents/MacOS/"*

if [[ "$UNSIGNED" == false ]]; then
    for binary in tun2socks vpntoris-route-helper vpntorisd vpntorisctl VPNToris; do
        codesign --force --options runtime --timestamp --sign "$SIGN_IDENTITY" "$APP/Contents/MacOS/$binary"
    done
    codesign --force --deep --options runtime --timestamp --sign "$SIGN_IDENTITY" "$APP"
    codesign --verify --deep --strict --verbose=2 "$APP"
else
    for binary in tun2socks vpntoris-route-helper vpntorisd vpntorisctl VPNToris; do
        codesign --remove-signature "$APP/Contents/MacOS/$binary" 2>/dev/null || true
    done
    codesign --remove-signature "$APP" 2>/dev/null || true
fi

STAGE="$BUILD_DIR/dmg"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/$APP_NAME.app"
ln -s /Applications "$STAGE/Applications"
rm -f "$DMG"
hdiutil create -volname "$APP_NAME" -srcfolder "$STAGE" -ov -format UDZO "$DMG"

if [[ "$UNSIGNED" == false ]]; then
    codesign --force --timestamp --sign "$SIGN_IDENTITY" "$DMG"
    xcrun notarytool submit "$DMG" --keychain-profile "$NOTARY_PROFILE" --wait
    xcrun stapler staple "$DMG"
    xcrun stapler validate "$DMG"
fi

shasum -a 256 "$DMG" > "$DMG.sha256"
echo "Release ready: $DMG"

#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD="$ROOT/ios/build"
mkdir -p "$BUILD"
SDKROOT="$(xcrun --sdk iphoneos --show-sdk-path)"
export SDKROOT IPHONEOS_DEPLOYMENT_TARGET=16.0

if [[ ! -f "$BUILD/unicorn/libunicorn.a" ]]; then
  curl -fL --retry 3 https://github.com/unicorn-engine/unicorn/archive/refs/tags/2.1.4.tar.gz -o "$BUILD/unicorn.tar.gz"
  echo "ea8863f095a0136388694e5a6063afd9bb7650e30243dd6251af59c5ce5601f4  $BUILD/unicorn.tar.gz" | shasum -a 256 -c -
  tar -xzf "$BUILD/unicorn.tar.gz" -C "$BUILD"
  patch -d "$BUILD/unicorn-2.1.4" -p1 < "$ROOT/third_party/ipatool/tools/patches/unicorn-ios.patch"
  ARCHFLAGS='-arch arm64' cmake -S "$BUILD/unicorn-2.1.4" -B "$BUILD/unicorn" \
    -DCMAKE_SYSTEM_NAME=iOS -DCMAKE_OSX_SYSROOT="$SDKROOT" \
    -DCMAKE_OSX_ARCHITECTURES=arm64 -DCMAKE_OSX_DEPLOYMENT_TARGET=16.0 \
    -DCMAKE_C_FLAGS='-target arm64-apple-ios16.0' -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF -DUNICORN_ARCH=x86 -DUNICORN_BUILD_TESTS=OFF -DUNICORN_INSTALL=OFF
  cmake --build "$BUILD/unicorn" --parallel "$(sysctl -n hw.ncpu)"
fi

if [[ "${BUILD_UNICORN_ONLY:-0}" == 1 ]]; then exit 0; fi

export GOOS=ios GOARCH=arm64 CGO_ENABLED=1
export CC="$(xcrun --sdk iphoneos --find clang) -target arm64-apple-ios16.0 -isysroot $SDKROOT"
export CGO_CFLAGS="-target arm64-apple-ios16.0 -isysroot $SDKROOT"
cd "$ROOT/authbridge"
go build -trimpath -buildmode=c-archive -o "$BUILD/libQingzhou.a" .

APP="$BUILD/Payload/QingzhouIPA.app"
mkdir -p "$APP"
xcrun --sdk iphoneos swiftc -swift-version 5 -parse-as-library -O \
  -target arm64-apple-ios16.0 -sdk "$SDKROOT" \
  -import-objc-header "$ROOT/ios/QingzhouBridge.h" \
  "$ROOT/ios/Models.swift" "$ROOT/ios/StoreModel.swift" "$ROOT/ios/QingzhouApp.swift" \
  "$BUILD/libQingzhou.a" \
  -framework UIKit -framework SwiftUI -framework Foundation -framework Security \
  -framework CoreFoundation -framework SystemConfiguration -lz -lresolv -liconv -lc++ \
  -Xlinker -force_load -Xlinker "$BUILD/unicorn/libunicorn.a" -Xlinker -export_dynamic \
  -o "$APP/QingzhouIPA"
cp "$ROOT/ios/Info.plist" "$APP/Info.plist"
cp "$ROOT/ios/Resources/regions.json" "$APP/regions.json"
cp "$ROOT/THIRD-PARTY-NOTICES.txt" "$APP/THIRD-PARTY-NOTICES.txt"
cp "$ROOT/third_party/ipatool/LICENSE" "$APP/ipatool-LICENSE"
cp -R "$ROOT/third-party-licenses" "$APP/third-party-licenses"
cp "$ROOT/ios/Resources/"*.png "$APP/"
plutil -lint "$APP/Info.plist"
codesign --force --sign - "$APP"
codesign --verify --deep --strict "$APP"
mkdir -p "$ROOT/dist"
cd "$BUILD"
/usr/bin/ditto -c -k --keepParent Payload "$ROOT/dist/Qingzhou-IPA-iOS-0.1.0-arm64.ipa"
unzip -t "$ROOT/dist/Qingzhou-IPA-iOS-0.1.0-arm64.ipa" | tail -1
echo "Built: dist/Qingzhou-IPA-iOS-0.1.0-arm64.ipa"

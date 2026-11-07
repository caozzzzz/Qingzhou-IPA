#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD="$ROOT/ios/build"
mkdir -p "$BUILD"
bash "$ROOT/ios/build-tci.sh"
if [[ "${BUILD_UNICORN_ONLY:-0}" == 1 ]]; then exit 0; fi

SDKROOT="$(xcrun --sdk iphoneos --show-sdk-path)"
export SDKROOT IPHONEOS_DEPLOYMENT_TARGET=16.0
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
  -Xlinker -force_load -Xlinker "$BUILD/unicorn-tci/libunicorn.a" -Xlinker -export_dynamic \
  -o "$APP/QingzhouIPA"
cp "$ROOT/ios/Info.plist" "$APP/Info.plist"
cp "$ROOT/ios/Resources/regions.json" "$APP/regions.json"
cp "$ROOT/THIRD-PARTY-NOTICES.txt" "$APP/THIRD-PARTY-NOTICES.txt"
cp "$ROOT/third_party/ipatool/LICENSE" "$APP/ipatool-LICENSE"
cp "$ROOT/ios/Resources/unicorn-COPYING" "$APP/unicorn-COPYING"
cp -R "$ROOT/third-party-licenses" "$APP/third-party-licenses"
cp "$ROOT/ios/Resources/"*.png "$APP/"
plutil -lint "$APP/Info.plist"
codesign --force --sign - "$APP"
codesign --verify --deep --strict "$APP"
mkdir -p "$ROOT/dist"
cd "$BUILD"
/usr/bin/ditto -c -k --keepParent Payload "$ROOT/dist/Qingzhou-IPA-iOS-0.2.0-NoJIT-arm64.ipa"
unzip -t "$ROOT/dist/Qingzhou-IPA-iOS-0.2.0-NoJIT-arm64.ipa" | tail -1
echo "Built: dist/Qingzhou-IPA-iOS-0.2.0-NoJIT-arm64.ipa"

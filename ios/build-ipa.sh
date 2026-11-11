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

xcrun --sdk iphoneos clang -fobjc-arc -O2 -target arm64-apple-ios16.0 -isysroot "$SDKROOT" -c "$ROOT/ios/NativeHTTP.m" -o "$BUILD/NativeHTTP.o"

APP="$BUILD/Payload/QingzhouIPA.app"
mkdir -p "$APP"
xcrun --sdk iphoneos swiftc -swift-version 5 -parse-as-library -O \
  -target arm64-apple-ios16.0 -sdk "$SDKROOT" \
  -import-objc-header "$ROOT/ios/QingzhouBridge.h" \
  "$ROOT/ios/StoreProofRuntime.swift" "$ROOT/ios/StoreProofServer.swift" "$ROOT/ios/ProofPropertyList.swift" "$ROOT/ios/ProofJavaScript.swift" "$ROOT/ios/Models.swift" "$ROOT/ios/StoreModel.swift" "$ROOT/ios/QingzhouApp.swift" \
  "$BUILD/libQingzhou.a" "$BUILD/NativeHTTP.o" \
  -framework WebKit -framework Network -framework UIKit -framework SwiftUI -framework Foundation -framework Security \
  -framework CoreFoundation -framework SystemConfiguration -lz -lresolv -liconv -lc++ \
  -Xlinker -force_load -Xlinker "$BUILD/unicorn-tci/libunicorn.a" -Xlinker -export_dynamic \
  -o "$APP/QingzhouIPA"
python3 "$ROOT/ios/tests/verify_store_runtime.py"
cp -R "$ROOT/third_party/ipadn-store-runtime/files" "$APP/StoreRuntime"
cp "$ROOT/third_party/ipadn-store-runtime/COPYING-GPL-2.0" "$APP/StoreRuntime/COPYING-GPL-2.0"
cp "$ROOT/third_party/ipadn-store-runtime/ORIGINAL-NOTICES.md" "$APP/StoreRuntime/ORIGINAL-NOTICES.md"
cp "$ROOT/third_party/ipadn-store-runtime/PROVENANCE.json" "$APP/StoreRuntime/PROVENANCE.json"
cp "$ROOT/third_party/ipadn-store-runtime/QINGZHOU-NOTICE.md" "$APP/StoreRuntime/QINGZHOU-NOTICE.md"
cp "$ROOT/ios/COPYING" "$APP/COPYING"
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
/usr/bin/ditto -c -k --keepParent Payload "$ROOT/dist/Qingzhou-IPA-iOS-0.4.3-NoJIT-arm64.ipa"
unzip -t "$ROOT/dist/Qingzhou-IPA-iOS-0.4.3-NoJIT-arm64.ipa" | tail -1
echo "Built: dist/Qingzhou-IPA-iOS-0.4.3-NoJIT-arm64.ipa"

#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BUILD="$(mktemp -d)"
trap 'rm -rf "$BUILD"' EXIT
APP="$BUILD/ProofSmoke.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp -R "$ROOT/third_party/ipadn-store-runtime/files" "$APP/Contents/Resources/StoreRuntime"
cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>local.qingzhou.proof-smoke</string>
<key>CFBundleExecutable</key><string>ProofSmoke</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>NSAppTransportSecurity</key><dict><key>NSAllowsArbitraryLoadsInWebContent</key><true/><key>NSAllowsLocalNetworking</key><true/></dict>
</dict></plist>
PLIST
swiftc -swift-version 5 -parse-as-library -O \
  "$ROOT/ios/tests/StoreWebKitSmoke.swift" "$ROOT/ios/StoreProofServer.swift" "$ROOT/ios/ProofPropertyList.swift" \
  -framework AppKit -framework WebKit -framework Network -framework Foundation \
  -o "$APP/Contents/MacOS/ProofSmoke"
codesign --force --sign - "$APP"
"$APP/Contents/MacOS/ProofSmoke"

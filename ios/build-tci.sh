#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD="$ROOT/ios/build"
SOURCE="$BUILD/unicorn-a53ddc9ac6d65b24936d4a37917333fcd816cfd0"
mkdir -p "$BUILD"
if [[ ! -f "$SOURCE/.qingzhou-prepared" ]]; then
  curl -fL --retry 3 https://codeload.github.com/mahee96/unicorn/tar.gz/a53ddc9ac6d65b24936d4a37917333fcd816cfd0 -o "$BUILD/unicorn-tci.tar.gz"
  echo "92d45a779879c1ddb541c40407980ad3573d4ee050e4a1679d0cec18ad9460c3  $BUILD/unicorn-tci.tar.gz" | shasum -a 256 -c -
  tar -xzf "$BUILD/unicorn-tci.tar.gz" -C "$BUILD"
  python3 "$ROOT/ios/prepare-tci.py" "$SOURCE"
  touch "$SOURCE/.qingzhou-prepared"
fi

if [[ ! -f "$BUILD/unicorn-host/libunicorn.2.dylib" ]]; then
  ARCHFLAGS='-arch arm64' cmake -S "$SOURCE" -B "$BUILD/unicorn-host" \
    -DCMAKE_OSX_ARCHITECTURES=arm64 -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=ON -DUNICORN_ARCH=x86 -DUNICORN_ENABLE_TCI=ON \
    -DUNICORN_BUILD_TESTS=OFF -DUNICORN_INSTALL=OFF
  cmake --build "$BUILD/unicorn-host" --parallel "$(sysctl -n hw.ncpu)"
fi
if [[ "${TCI_HOST_ONLY:-0}" == 1 ]]; then exit 0; fi

SDKROOT="$(xcrun --sdk iphoneos --show-sdk-path)"
if [[ ! -f "$BUILD/unicorn-tci/libunicorn.a" ]]; then
  ARCHFLAGS='-arch arm64' cmake -S "$SOURCE" -B "$BUILD/unicorn-tci" \
    -DCMAKE_SYSTEM_NAME=iOS -DCMAKE_OSX_SYSROOT="$SDKROOT" \
    -DCMAKE_OSX_ARCHITECTURES=arm64 -DCMAKE_OSX_DEPLOYMENT_TARGET=16.0 \
    -DCMAKE_C_FLAGS='-target arm64-apple-ios16.0' -DCMAKE_BUILD_TYPE=Release \
    -DBUILD_SHARED_LIBS=OFF -DUNICORN_ARCH=x86 -DUNICORN_ENABLE_TCI=ON \
    -DUNICORN_BUILD_TESTS=OFF -DUNICORN_INSTALL=OFF
  cmake --build "$BUILD/unicorn-tci" --parallel "$(sysctl -n hw.ncpu)"
fi

#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2026 Jérôme Bastian Winkel
#
# The encrypted copy's cipher on Android (ADR-0033 §4): SQLCipher takes its crypto from OpenSSL's
# libcrypto, as on Linux and Windows, but Android offers none to an app, so the shell builds it
# once per target from the pinned release, checked against its published SHA-256, and links it
# statically. `../.cargo/config.toml` points each Android target's OPENSSL_DIR at what this installs.
#
#   ./android-openssl.sh [rust-target ...]    (default: the four the universal APK carries)
set -euo pipefail

VERSION=3.5.9
SHA256=603f5602e2eef00d77fbd429d34dcd5822bb301757a1bc9cdb24c670f1eb859a
API=24

here=$(cd "$(dirname "$0")" && pwd)
out="$here/target/openssl"
ndk=${NDK_HOME:-${ANDROID_NDK_HOME:-}}
[ -d "$ndk" ] || { echo "android-openssl: NDK_HOME names no NDK" >&2; exit 1; }
case "$(uname -s)" in Darwin) host=darwin-x86_64 ;; *) host=linux-x86_64 ;; esac

targets=("$@")
[ ${#targets[@]} -gt 0 ] || targets=(aarch64-linux-android armv7-linux-androideabi i686-linux-android x86_64-linux-android)

mkdir -p "$out/src"
tarball="$out/src/openssl-$VERSION.tar.gz"
if [ ! -f "$tarball" ]; then
  curl -sSfL -o "$tarball.part" "https://github.com/openssl/openssl/releases/download/openssl-$VERSION/openssl-$VERSION.tar.gz"
  mv "$tarball.part" "$tarball"
fi
echo "$SHA256  $tarball" | shasum -a 256 -c - >/dev/null || { echo "android-openssl: checksum mismatch" >&2; rm -f "$tarball"; exit 1; }

for target in "${targets[@]}"; do
  case "$target" in
    aarch64-linux-android) os=android-arm64 ;;
    armv7-linux-androideabi) os=android-arm ;;
    i686-linux-android) os=android-x86 ;;
    x86_64-linux-android) os=android-x86_64 ;;
    *) echo "android-openssl: $target is not an Android target" >&2; exit 1 ;;
  esac
  prefix="$out/$target"
  if [ -f "$prefix/lib/libcrypto.a" ]; then continue; fi
  work=$(mktemp -d)
  tar -xzf "$tarball" -C "$work"
  (
    cd "$work/openssl-$VERSION"
    export ANDROID_NDK_ROOT="$ndk"
    export PATH="$ndk/toolchains/llvm/prebuilt/$host/bin:$PATH"
    ./Configure "$os" -D__ANDROID_API__=$API no-shared no-tests no-apps no-docs no-ui-console --prefix="$prefix" --libdir=lib >/dev/null
    make -j"$(getconf _NPROCESSORS_ONLN)" build_libs >/dev/null
    make install_dev >/dev/null
  )
  rm -rf "$work"
  echo "android-openssl: libcrypto $VERSION for $target"
done

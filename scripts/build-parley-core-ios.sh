#!/usr/bin/env bash
# Build ParleyCore.xcframework for the iOS Device Host. Run on macOS with Xcode
# 16+ (gomobile bind -target=ios does not work on Windows/WSL). Then embed
# client/ios/Frameworks/ParleyCore.xcframework in Runner -> Frameworks,
# Libraries, and Embedded Content as Embed & Sign; the guarded channel in
# AppDelegate.swift starts using it as soon as it can be imported.
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
: "${GOPATH:=$HOME/go}"
export PATH="${GO_BIN:-$repo/tmp/toolchains/go/bin}:$GOPATH/bin:$PATH"

command -v gomobile >/dev/null || {
  echo "gomobile not installed; see .scratch/v01/toolchain.md" >&2
  exit 1
}

mkdir -p "$repo/client/ios/Frameworks"
cd "$repo"
gomobile bind -target=ios -o client/ios/Frameworks/ParleyCore.xcframework ./mobile
ls -ld client/ios/Frameworks/ParleyCore.xcframework

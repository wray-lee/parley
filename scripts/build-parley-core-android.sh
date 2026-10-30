#!/usr/bin/env bash
# Build the gomobile Device Host AAR that client/android/app/build.gradle.kts
# depends on. Run inside WSL with the shared Flutter/Android/Go toolchain; see
# .scratch/v01/toolchain.md for the one-time go install commands.
set -eu

repo=$(cd "$(dirname "$0")/.." && pwd)
: "${ANDROID_HOME:?set ANDROID_HOME}"
export PATH="${GO_BIN:-$repo/tmp/toolchains/go/bin}:${GOPATH:-$HOME/go}/bin:$PATH"
export ANDROID_NDK_HOME="${ANDROID_NDK_HOME:-$ANDROID_HOME/ndk/28.2.13676358}"
export ANDROID_NDK_ROOT="$ANDROID_NDK_HOME"

command -v gomobile >/dev/null || {
  echo "gomobile not installed; see .scratch/v01/toolchain.md" >&2
  exit 1
}

mkdir -p "$repo/client/android/app/libs"
cd "$repo"
gomobile bind -target=android -androidapi 24 \
  -o client/android/app/libs/parleycore.aar ./mobile
ls -lh client/android/app/libs/parleycore.aar

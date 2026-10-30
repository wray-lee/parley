#!/usr/bin/env bash
# Verify a built APK the way the release acceptance does: unchanged application
# id, the versionCode from client/pubspec.yaml, 16 KiB page alignment, and the
# release signing certificate when one is expected.
#
# Usage: scripts/check-apk.sh --apk FILE [--version-code N] [--cert-sha256 HEX]
set -eu

apk=
version_code=
cert=
while [ $# -gt 0 ]; do
  case $1 in
    --apk) apk=${2:?--apk needs a value}; shift 2 ;;
    --version-code) version_code=${2:?--version-code needs a value}; shift 2 ;;
    --cert-sha256) cert=${2:?--cert-sha256 needs a value}; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$apk" ] && [ -f "$apk" ] || {
  echo "usage: scripts/check-apk.sh --apk FILE [--version-code N] [--cert-sha256 HEX]" >&2
  exit 2
}
: "${ANDROID_HOME:?set ANDROID_HOME to the Android SDK}"

build_tools=$ANDROID_HOME/build-tools/$(ls "$ANDROID_HOME/build-tools" | sort -V | tail -n 1)
[ -x "$build_tools/aapt2" ] || { echo "aapt2 not found in $build_tools" >&2; exit 1; }

badging=$("$build_tools/aapt2" dump badging "$apk")
package=$(printf '%s\n' "$badging" | sed -n "s/^package: name='\\([^']*\\)'.*/\\1/p")
code=$(printf '%s\n' "$badging" | sed -n "s/.*versionCode='\\([0-9]*\\)'.*/\\1/p")
[ "$package" = "app.parley.translate" ] || {
  echo "application id is '$package', expected app.parley.translate" >&2
  exit 1
}
if [ -n "$version_code" ] && [ "$code" != "$version_code" ]; then
  echo "versionCode is $code, expected $version_code" >&2
  exit 1
fi

"$build_tools/zipalign" -c -P 16 -v 4 "$apk" >/dev/null || {
  echo "zipalign -c -P 16 failed for $apk" >&2
  exit 1
}

if [ -n "$cert" ]; then
  digest=$("$build_tools/apksigner" verify --print-certs "$apk" |
    sed -n 's/.*certificate SHA-256 digest: //p' | head -n 1 |
    tr 'A-F' 'a-f' | tr -d ':')
  expected=$(printf '%s' "$cert" | tr 'A-F' 'a-f' | tr -d ':')
  [ "$digest" = "$expected" ] || {
    echo "signing certificate SHA-256 is '$digest', expected '$expected'" >&2
    exit 1
  }
fi

echo "apk ok: $apk package=$package versionCode=$code aligned=16KiB${cert:+ cert=$cert}"

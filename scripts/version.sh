#!/usr/bin/env bash
# The single version source is client/pubspec.yaml. versionCode must follow
# major * 1,000,000 + minor * 1,000 + patch (v0.1.0 -> 1000) so v0.1.0 is an
# in-place upgrade over v0.0.2. Tags are human-gated: this script never creates one.
#
# Usage: scripts/version.sh [--tag vX.Y.Z] [--require-tag-absent] [--github-output FILE]
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
tag=
require_absent=false
github_output=

while [ $# -gt 0 ]; do
  case $1 in
    --tag) tag=${2:?--tag needs a value}; shift 2 ;;
    --require-tag-absent) require_absent=true; shift ;;
    --github-output) github_output=${2:?--github-output needs a file}; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

line=$(sed -n 's/^version:[[:space:]]*//p' "$root/client/pubspec.yaml" | head -n 1)
full=${line%%+*}
build=${line#*+}
case $full in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "client/pubspec.yaml version is '$line'; expected X.Y.Z+N" >&2; exit 1 ;;
esac
case $build in
  ''|*[!0-9]*) echo "client/pubspec.yaml version '$line' has no numeric build number" >&2; exit 1 ;;
esac

IFS=. read -r major minor patch <<< "$full"
for part in "$major" "$minor" "$patch"; do
  case $part in ''|*[!0-9]*) echo "version '$full' is not numeric" >&2; exit 1 ;; esac
done
code=$((10#$major * 1000000 + 10#$minor * 1000 + 10#$patch))
if [ "$build" != "$code" ]; then
  echo "versionCode $build does not match the formula for $full (expected $code)" >&2
  exit 1
fi

tag_name="v$full"
if [ -n "$tag" ] && [ "$tag" != "$tag_name" ]; then
  echo "tag $tag does not match client/pubspec.yaml version $tag_name" >&2
  exit 1
fi

if $require_absent; then
  # Dry runs must not race a human-created release tag. Network failure is an
  # explicit error, never a silent pass.
  existing=$(git -C "$root" ls-remote --tags --refs origin "refs/tags/$tag_name")
  if [ -n "$existing" ]; then
    echo "tag $tag_name already exists; tags are human-gated and this run is a dry run" >&2
    exit 1
  fi
fi

out=$(printf 'version=%s\nversion_code=%s\ntag=%s\n' "$full" "$code" "$tag_name")
printf '%s\n' "$out"
if [ -n "$github_output" ]; then
  printf '%s\n' "$out" >> "$github_output"
fi

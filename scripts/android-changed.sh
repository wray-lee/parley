#!/usr/bin/env bash
# Print "true" when a diff touches anything the Android job builds, "false" for
# documentation-only changes. Usage: scripts/android-changed.sh BASE HEAD
set -eu

base=${1:?BASE revision required}
head=${2:?HEAD revision required}

if [ -z "$base" ] || [ "$base" = "0000000000000000000000000000000000000000" ]; then
  # New branch push: compare against the default branch's merge base.
  git fetch --no-tags --depth=50 origin main >/dev/null 2>&1 || true
  base=$(git merge-base origin/main "$head" 2>/dev/null || true)
fi
if [ -z "$base" ]; then
  echo true
  exit 0
fi

changed=$(git diff --name-only "$base" "$head")
if [ -z "$changed" ]; then
  echo false
  exit 0
fi

# Everything except documentation needs the Android build. .md files anywhere
# are documentation; docs/ and .scratch/ are never compiled.
if printf '%s\n' "$changed" | grep -Evq '^(docs/|\.scratch/|.*\.md$|LICENSE$|NOTICE$)'; then
  echo true
else
  echo false
fi

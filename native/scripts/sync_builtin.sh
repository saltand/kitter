#!/usr/bin/env bash
# Syncs the embedded copy of the built-in Kitter skill with the canonical
# source at the repository root (resources/skills/kitter). Go's embed
# cannot reach outside the module, so the files live under
# core/library/builtin/ and builtin_sync_test.go keeps them identical.
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
src="$repo_root/resources/skills/kitter"
dst="$repo_root/native/core/library/builtin/kitter"

rm -rf "$dst"
mkdir -p "$dst"
cp -R "$src/." "$dst/"
echo "synced $src -> $dst"

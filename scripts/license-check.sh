#!/usr/bin/env sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"

echo "Review direct dependency manifests:"
echo "- $ROOT/server/go.mod"
echo "- $ROOT/mobile/pubspec.yaml"
echo "- $ROOT/crypto/rust/Cargo.toml"
echo
echo "Update THIRD_PARTY_NOTICES.md before release with exact resolved versions and transitive licenses."


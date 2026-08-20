#!/usr/bin/env bash
# Recomputes flake.nix's vendorHash after a go.mod/go.sum change.
#
# vendorHash locks the hash of this package's vendored Go module dependencies
# for buildGoModule (see the comment above it in flake.nix). There's no
# `nix hash` subcommand for this — the reliable way is nixpkgs' own trick:
# temporarily set the hash to the well-known all-zero placeholder
# (lib.fakeHash), let `nix build` fail with a hash mismatch, and read the
# correct hash out of that failure's "got: sha256-..." line.
#
# Usage: ./scripts/update-vendor-hash.sh   (run from the project root, or
# anywhere — it cd's to its own repo root first)
set -euo pipefail

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

FLAKE_FILE="flake.nix"
FAKE_HASH="sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

current_hash=$(grep -oE 'vendorHash = "[^"]*"' "$FLAKE_FILE" | sed -E 's/vendorHash = "(.*)"/\1/')
if [ -z "$current_hash" ]; then
	echo "error: couldn't find a 'vendorHash = \"...\"' line in $FLAKE_FILE" >&2
	exit 1
fi

restore_original() {
	sed -i.bak "s|vendorHash = \"$FAKE_HASH\"|vendorHash = \"$current_hash\"|" "$FLAKE_FILE"
	rm -f "$FLAKE_FILE.bak"
}
trap restore_original ERR

echo "Building with a fake vendorHash to force a hash-mismatch error..."
sed -i.bak "s|vendorHash = \"$current_hash\"|vendorHash = \"$FAKE_HASH\"|" "$FLAKE_FILE"
rm -f "$FLAKE_FILE.bak"

build_output="$(nix build .#default 2>&1 || true)"
# `|| true` guards the whole pipeline: under `pipefail`, grep finding no match
# (e.g. the build failed for a reason unrelated to the hash, like a genuinely
# broken go.sum) would otherwise make this assignment itself a failing
# command, tripping the ERR trap and reverting silently before the explicit
# "couldn't parse a new hash" diagnostic below ever runs.
new_hash="$(printf '%s\n' "$build_output" | grep -oE 'got:\s+sha256-[A-Za-z0-9+/=]+' | awk '{print $2}' | head -1 || true)"

if [ -z "$new_hash" ]; then
	echo "error: couldn't parse a new hash out of the build output:" >&2
	printf '%s\n' "$build_output" >&2
	restore_original
	exit 1
fi

sed -i.bak "s|vendorHash = \"$FAKE_HASH\"|vendorHash = \"$new_hash\"|" "$FLAKE_FILE"
rm -f "$FLAKE_FILE.bak"
trap - ERR # new_hash is now written; a verification failure below shouldn't revert it

echo "vendorHash: $current_hash -> $new_hash"
echo "Verifying with a real build..."
nix build .#default
echo "OK: vendorHash updated and verified."

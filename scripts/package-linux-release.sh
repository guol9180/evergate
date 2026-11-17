#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: package-linux-release.sh <version> <binary> <output-dir>}"
binary="${2:?usage: package-linux-release.sh <version> <binary> <output-dir>}"
output_dir="${3:?usage: package-linux-release.sh <version> <binary> <output-dir>}"
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

version="${version#v}"
asset="evergate_${version}_linux_amd64.tar.gz"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT

install -m 0755 "$binary" "$stage/evergate"
install -m 0644 "$repo/LICENSE" "$stage/LICENSE"
install -m 0644 "$repo/README.md" "$stage/README.md"
mkdir -p "$output_dir"
tar -C "$stage" -czf "$output_dir/$asset" evergate LICENSE README.md
echo "$output_dir/$asset"

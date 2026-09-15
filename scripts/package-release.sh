#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
RUNTIME="${1:?Usage: package-release.sh RUNTIME_DIRECTORY [OUTPUT_DIRECTORY]}"
OUT="${2:-$ROOT/release}"
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT
mkdir -p "$WORK/x-ui/bin"
# Deliberate allowlist: never publish a database, certificate, or config.json.
cp "$RUNTIME/x-ui" "$WORK/x-ui/x-ui"
for file in xray-linux-amd64 LICENSE README.md geoip.dat geosite.dat geoip_IR.dat geosite_IR.dat geoip_RU.dat geosite_RU.dat; do
  cp "$RUNTIME/bin/$file" "$WORK/x-ui/bin/$file"
done
cp "$ROOT/LICENSE" "$WORK/x-ui/LICENSE"
cp "$ROOT/PROVENANCE.md" "$WORK/x-ui/PROVENANCE.md"
cp "$ROOT/scripts/install-selfhosted.sh" "$WORK/x-ui/install-selfhosted.sh"
chmod 755 "$WORK/x-ui/x-ui" "$WORK/x-ui/bin/xray-linux-amd64" "$WORK/x-ui/install-selfhosted.sh"
export COPYFILE_DISABLE=1
tar --no-xattrs -czf "$OUT/x-ui-linux-amd64.tar.gz" -C "$WORK" x-ui
cd "$OUT"
if command -v sha256sum >/dev/null; then
  sha256sum x-ui-linux-amd64.tar.gz > SHA256SUMS
else
  shasum -a 256 x-ui-linux-amd64.tar.gz > SHA256SUMS
fi
cat SHA256SUMS

#!/usr/bin/env bash
set -euo pipefail
script=$(mktemp)
trap 'rm -f "$script"' EXIT
curl --proto '=https' --tlsv1.2 -fLsS --retry 3 --connect-timeout 20 \
  https://raw.githubusercontent.com/jbhd135/4x-ui/main/scripts/install-selfhosted.sh -o "$script"
bash "$script" "$@"

#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
PESQUISARR="${PESQUISARR:-$ROOT/../../../pesquisarr}"
if [[ ! -d "$PESQUISARR/dist/server" ]]; then
  echo "missing $PESQUISARR/dist/server — build pesquisarr (astro build) first" >&2
  exit 1
fi
# Keep package metadata; refresh payload from dist.
find "$ROOT" -mindepth 1 -maxdepth 1 \
  ! -name 'orvalho.cue' ! -name 'README.md' ! -name 'assemble.sh' ! -name '.gitignore' \
  -exec rm -rf {} +
cp -a "$PESQUISARR/dist/server/." "$ROOT/"
mkdir -p "$ROOT/assets"
if [[ -d "$PESQUISARR/dist/client" ]]; then
  cp -a "$PESQUISARR/dist/client/." "$ROOT/assets/"
fi
echo "assembled pesquisarr package in $ROOT"

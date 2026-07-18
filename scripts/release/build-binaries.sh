#!/usr/bin/env bash
#
# Cross-build the bramble CLI for one or more target platforms and emit
# release-named binaries into an output directory.
#
# Usage:
#   scripts/release/build-binaries.sh <version> <outdir> <goos/goarch> [<goos/goarch>...]
#
# Each target is a "goos/goarch" pair (e.g. linux/amd64, darwin/arm64).
# Output binaries are named:  bramble_<version>_<goos>_<goarch>[.exe]
#
# The version is embedded via -ldflags -X so `bramble --version` reports the
# real release version instead of the default "dev".
#
# CGO: linux (BlueZ/D-Bus) and windows (WinRT) BLE backends are pure Go and
# build with CGO_ENABLED=0. The darwin BLE backend (CoreBluetooth) requires
# cgo, so darwin targets are built with CGO_ENABLED=1 and MUST run on a
# native macOS host (a linux host cannot compile the darwin/cgo target).
set -euo pipefail

if [ "$#" -lt 3 ]; then
  echo "usage: $0 <version> <outdir> <goos/goarch> [<goos/goarch>...]" >&2
  exit 2
fi

version="$1"
outdir="$2"
shift 2

pkg="./cmd/bramble"
ldflags="-s -w -X github.com/justinlindh/bramble-cli/internal/commands.version=${version}"

mkdir -p "$outdir"

for target in "$@"; do
  goos="${target%%/*}"
  goarch="${target##*/}"
  if [ -z "$goos" ] || [ -z "$goarch" ] || [ "$goos" = "$target" ]; then
    echo "invalid target (want goos/goarch): $target" >&2
    exit 2
  fi

  ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi

  cgo="0"
  if [ "$goos" = "darwin" ]; then
    cgo="1"
  fi

  out="${outdir}/bramble_${version}_${goos}_${goarch}${ext}"
  echo "building ${goos}/${goarch} (CGO_ENABLED=${cgo}) -> ${out}"
  CGO_ENABLED="$cgo" GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$ldflags" -o "$out" "$pkg"
done

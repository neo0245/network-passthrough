#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version-tag>" >&2
  exit 1
fi

VERSION="$1"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist/$VERSION"
GO="/usr/local/go/bin/go"

rm -rf "$DIST"
mkdir -p "$DIST"

build_target() {
  local goos="$1"
  local goarch="$2"
  local label="$3"
  local archive="$4"
  local ext=""
  local stage="$DIST/$label"

  mkdir -p "$stage"
  if [[ "$goos" == "windows" ]]; then
    ext=".exe"
  fi

  GOOS="$goos" GOARCH="$goarch" "$GO" build -trimpath -ldflags="-s -w -X main.version=${VERSION#v}" -o "$stage/slppc$ext" "$ROOT/cmd/slppc"
  GOOS="$goos" GOARCH="$goarch" "$GO" build -trimpath -ldflags="-s -w -X main.version=${VERSION#v}" -o "$stage/slppd$ext" "$ROOT/cmd/slppd"

  cp "$ROOT/docs/USER_MANUAL.md" "$stage/"
  cp "$ROOT/docs/RELEASE_NOTES_v0.1.0.md" "$stage/"
  cp "$ROOT/README.md" "$stage/"
  if [[ "$goos" == "linux" ]]; then
    mkdir -p "$stage/deploy/systemd"
    cp "$ROOT/deploy/systemd/slppd.service" "$stage/deploy/systemd/"
  fi

  if [[ "$archive" == *.zip ]]; then
    (cd "$DIST" && zip -rq "$archive" "$label")
  else
    (cd "$DIST" && tar -czf "$archive" "$label")
  fi
}

build_target windows amd64 windows-x86_64 "slpp-${VERSION}-windows-x86_64.zip"
build_target windows arm64 windows-arm64 "slpp-${VERSION}-windows-arm64.zip"
build_target linux arm64 linux-arm64 "slpp-${VERSION}-linux-arm64.tar.gz"
build_target darwin arm64 macos-arm64 "slpp-${VERSION}-macos-arm64.tar.gz"

echo "release artifacts created in $DIST"

#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != "arm64" ]]; then
  echo "error: builds are supported on arm64 macOS only" >&2
  exit 1
fi

WAILS="${WAILS:-$(command -v wails || true)}"
if [[ -z "$WAILS" ]]; then
  WAILS="$(go env GOPATH)/bin/wails"
fi
if [[ ! -x "$WAILS" ]]; then
  echo "error: wails CLI not found; install with:" >&2
  echo "  go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1" >&2
  exit 1
fi

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
OUT_DIR="${OUT_DIR:-dist}"
APP="build/bin/chattu.app"
ZIP="$OUT_DIR/chattu-${VERSION}-darwin-arm64.zip"

"$WAILS" build -platform darwin/arm64 -clean

mkdir -p "$OUT_DIR"
rm -f "$ZIP"
ditto -c -k --keepParent "$APP" "$ZIP"
echo "built $ZIP"
